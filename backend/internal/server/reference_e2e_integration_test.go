// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package server

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/internal/gateway"
	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/tenancy"
)

// referenceApp names a reference host app, its fixture and where the
// stack of its test listens.
type referenceApp struct {
	fixture string
	dir     string
	addr    string
	// id is the Android application ID, iosID the iOS bundle identifier, and
	// a11yTarget the Dart entry point the accessibility-tree test's build
	// runs (QA-011).
	id         string
	iosID      string
	a11yTarget string
}

// Verifies: DX-004, QA-006, SEC-026, SEC-027, SEC-030.
// Plux Bank's exit flow (apps/plux_bank/integration_test/bank_flows.dart)
// against this server and the reference backend, under the maximum profile:
// every source through the gateway, the sign-in token exchanged for the user
// the test identity provider vouches for, the transfer confirmed with the
// device's SCA key and accepted by a backend that trusts only the gateway's
// assertion, and the notification streamed through the gateway to the
// dashboard. See runReferenceApp.
func TestPluxBankAgainstTheServer(t *testing.T) {
	runReferenceApp(t, referenceApp{
		fixture: "plux_bank", dir: "plux_bank", addr: deviceAddr("127.0.0.1:18097"),
		id: "dev.plux.plux_bank", iosID: "dev.plux.pluxBank", a11yTarget: "integration_test/a11y_main.dart",
	})
}

// Verifies: DX-004, QA-006, SEC-030.
// Plux Express's flows (apps/plux_express/integration_test/express_flows.dart)
// against this server and the reference backend, under the standard profile
// with a direct source (the catalogue) beside gateway sources: the
// paginated catalogue, the persisted cart, an order tracked over a WebSocket
// through the gateway to its delivery, and a courier's confirmation queued
// offline and replayed once. See runReferenceApp.
func TestPluxExpressAgainstTheServer(t *testing.T) {
	runReferenceApp(t, referenceApp{
		fixture: "plux_express", dir: "plux_express", addr: deviceAddr("127.0.0.1:18098"),
		id: "dev.plux.plux_express", iosID: "dev.plux.pluxExpress", a11yTarget: "lib/main.dart",
	})
}

// referenceRun is a reference app's stack, published and promoted: what
// the flows of the app run against.
type referenceRun struct {
	app     referenceApp
	st      *stack
	ref     *referenceAPI
	project string
	// release is the sequence of the release the publish made.
	release int64
	// roots are the environment's root keys as keyID:publicKey.
	roots []string
	// functions are the versions deployed before the publish, by name.
	functions map[string]*pluxv1.FunctionVersion
}

// startReferenceApp builds and starts the reference backend (test/refapi),
// starts the stack with the roles that run functions, deploys the app's
// functions with the artefacts of sdk/fn/examples (the worker's builder is
// replaced by them; see exampleFunctions), and publishes the app's fixture
// with its environments pointed at that backend.
//
// The fixtures' environments name refapi.invalid, which no device resolves:
// the copy of the project the test publishes names the backend's loopback
// address instead (the apps trust only its test CA, which the flows get as
// the PLUX_REFAPI_CA define). PLUX_E2E_REFAPI_ADDR fixes the address the
// backend listens on, for runs where a device reaches it through a port
// forward.
func startReferenceApp(t *testing.T, app referenceApp) *referenceRun {
	t.Helper()
	// The reference backend verifies the gateway's assertions with the keys
	// this server publishes, so it needs to know where the server will be
	// before either starts. The environment is not a production one, so
	// its assertions are signed with the development class of key.
	addr := app.addr
	if strings.HasSuffix(addr, ":0") {
		addr = freeAddr(t)
	}
	server := "http://" + addr
	ref := startReferenceAPI(t, referenceAssertions{jwks: server + JWKSPath + "development", issuer: server, audience: referenceAudience})
	// The gateway trusts the reference backend's test CA, as a customer's
	// private CA is trusted (gateway.upstreamCAFile), and so does its read
	// of the identity provider's keys.
	examples := loadExampleFunctions(t)
	st := startStackWith(t, addr, stackOptions{
		extraYAML:    "gateway:\n  upstreamCAFile: \"" + ref.caFile + "\"\n",
		identityHTTP: ref.client(t),
		functions:    &stackFunctions{Builder: examples, Linker: examples},
	})
	project := st.project(t, app.fixture)
	pointAtReferenceAPI(t, project, ref.url)
	// What goes through the gateway has its upstream registered, at the
	// base URL the environment gives the source (SEC-031), and the
	// environment's users sign in at the reference backend's identity
	// provider (SEC-026), which vouches for the tier of the customer
	// besides the subject: the claim a function prices a transfer by is
	// forwarded to what the user token carries.
	st.registerUpstreams(t, project, "staging", referenceAudience)
	st.setIdentityProvider(t, "staging", gateway.IdentityProvider{
		Issuer: ref.url + "/idp", JWKSURL: ref.url + "/idp/jwks.json", Audiences: []string{"plux"},
		ForwardClaims: []string{"tier"},
	})
	// The functions the app's plugins grant exist before the release does
	// (PLX-7012).
	deployed := st.deployExampleFunctions(t, project, "staging")

	var pub struct {
		OK      bool
		Release int64
	}
	decode(t, st.run(t, 0, "publish", "-C", project, "--env", "staging", "--promote", "staging", "--json"), &pub)
	if !pub.OK || pub.Release != 1 {
		t.Fatalf("publish: %+v", pub)
	}
	var keys struct {
		Keys []struct{ KeyID, PublicKey string }
	}
	decode(t, st.run(t, 0, "keys", "-C", project, "--env", "staging", "--json"), &keys)
	var roots []string
	for _, k := range keys.Keys {
		roots = append(roots, k.KeyID+":"+k.PublicKey)
	}
	return &referenceRun{app: app, st: st, ref: ref, project: project, release: pub.Release, roots: roots, functions: deployed}
}

// runReferenceApp starts the app's stack (startReferenceApp) and runs the
// app's flows against it. It runs only when PLUX_E2E_FLUTTER names the
// flutter executable (make e2e-starter); PLUX_E2E_DEVICE names an emulator
// or simulator, as for the starter.
func runReferenceApp(t *testing.T, app referenceApp) {
	flutter := os.Getenv("PLUX_E2E_FLUTTER")
	if flutter == "" {
		t.Skip("set PLUX_E2E_FLUTTER to the flutter executable (make e2e-starter)")
	}
	dir, err := filepath.Abs(filepath.Join("..", "..", "..", "apps", app.dir))
	if err != nil {
		t.Fatal(err)
	}
	run := startReferenceApp(t, app)
	st, ref, roots := run.st, run.ref, run.roots

	defines := []string{
		"--dart-define=PLUX_ENDPOINT=" + st.server,
		"--dart-define=PLUX_APP_ID=" + st.app.ID,
		"--dart-define=PLUX_ENVIRONMENT=staging",
		"--dart-define=PLUX_HOST_BUILD=e2e",
		"--dart-define=PLUX_ROOT_KEYS=" + strings.Join(roots, ","),
		"--dart-define=PLUX_REFAPI_CA=" + ref.ca,
		"--dart-define=PLUX_REFAPI_URL=" + ref.url,
	}
	// The expanded reporter everywhere: on GitHub Actions flutter test
	// picks another, whose summary line differs.
	steps := [][]string{append([]string{flutter, "test", "--reporter=expanded", "test/e2e_test.dart"}, defines...)}
	succeeded := "All tests passed!"
	ctx := st.ctx
	if device := os.Getenv("PLUX_E2E_DEVICE"); device != "" {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, deviceTimeout(t))
		defer cancel()
		// One result bundle per app: xcodebuild refuses a path that exists.
		if bundle := os.Getenv("PLUX_E2E_XCRESULT"); bundle != "" {
			t.Setenv("PLUX_E2E_XCRESULT", strings.TrimSuffix(bundle, ".xcresult")+"-"+app.dir+".xcresult")
		}
		steps, succeeded = deviceSteps(flutter, device, defines)
		if os.Getenv("PLUX_E2E_XCTEST") != "" {
			// The scheme runs the UI tests too; they need the accessibility
			// pass's build, so the flows' run leaves them out.
			last := len(steps) - 1
			steps[last] = append(steps[last], "-skip-testing:RunnerUITests")
		}
	}
	var out bytes.Buffer
	for _, step := range steps {
		cmd := exec.CommandContext(ctx, step[0], step[1:]...) //nolint:gosec // G204: the flutter, Xcode and device the developer named.
		cmd.Dir = dir
		cmd.Stdout = &out
		cmd.Stderr = cmd.Stdout
		if err = cmd.Run(); err != nil {
			break
		}
	}
	// Only a failure's output is printed, and only its end: on a device the
	// build's output runs to tens of thousands of lines, and CI keeps the
	// last few thousand of a job's log.
	if err != nil || !strings.Contains(out.String(), succeeded) {
		t.Logf("the flows (%s), last %d lines:\n%s", dir, flowsTail, tail(out.String(), flowsTail))
		t.Logf("the reference backend said:\n%s", ref.log())
		t.Fatalf("%s's flows failed: %v", app.dir, err)
	}

	// On an emulator or simulator the app's accessibility tree is walked too
	// (QA-011): the platform's own view of the pages the flows drove.
	if device := os.Getenv("PLUX_E2E_DEVICE"); device != "" {
		if os.Getenv("PLUX_E2E_XCTEST") != "" {
			accessibilityTreePassIOS(ctx, t, app, dir, device, defines)
		} else {
			accessibilityTreePass(ctx, t, app, dir, device, defines)
		}
	}

	// The device synced from this server: it reported what the runtime
	// sends without consent.
	events, err := deviceEvents(st)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, e := range events {
		seen[e.Name] = true
	}
	missing := false
	for _, want := range []string{"session_start", "sync_result"} {
		if !seen[want] {
			missing = true
			t.Errorf("no %s event reached the server: %v", want, seen)
		}
	}
	if missing {
		t.Logf("events of every environment: %v", eventsByEnvironment(t, st))
		t.Logf("the flows (%s), last %d lines:\n%s", dir, flowsTail, tail(out.String(), flowsTail))
	}
}

// eventsByEnvironment counts the app's events per environment key and
// name, for a failure's report.
func eventsByEnvironment(t *testing.T, st *stack) map[string]map[string]int {
	t.Helper()
	envs, err := st.svc.Tenancy.ListEnvironments(st.ctx, st.owner, st.app.ID, tenancy.Page{Size: 10})
	if err != nil {
		t.Logf("list environments: %v", err)
		return nil
	}
	out := map[string]map[string]int{}
	for _, e := range envs {
		events, err := st.svc.Events.List(st.ctx, st.owner, st.app.ID, e.ID, "", time.Time{}, storage.Cursor{}, 500)
		if err != nil {
			t.Logf("list events of %s: %v", e.Key, err)
			continue
		}
		out[e.Key] = map[string]int{}
		for _, ev := range events {
			out[e.Key][ev.Name]++
		}
	}
	return out
}

// referenceAudience is the audience the reference upstreams are registered
// with and the reference backend expects in an assertion.
const referenceAudience = "refapi"

// referenceAssertions says which gateway the reference backend trusts.
type referenceAssertions struct{ jwks, issuer, audience string }

// referenceAPI is a running reference backend.
type referenceAPI struct {
	// url is its base URL, https://127.0.0.1:PORT.
	url string
	// ca is the PEM of the certificate authority it generated, in base64.
	ca string
	// caFile is where the PEM of that authority is written.
	caFile string
	stderr *bytes.Buffer
}

// log is what the backend wrote to its standard error.
func (r *referenceAPI) log() string { return r.stderr.String() }

// client is an HTTP client that trusts the backend's test authority, as the
// gateway does for a customer's identity provider on a private authority.
func (r *referenceAPI) client(t *testing.T) *http.Client {
	t.Helper()
	pem, err := os.ReadFile(r.caFile)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		t.Fatal("the reference backend's authority is no PEM certificate")
	}
	return &http.Client{
		Timeout:   10 * time.Second,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}},
	}
}

// startReferenceAPI builds test/refapi, starts it as the e2e flows do
// (-ca-out, a short -track-step and the gateway it trusts) and waits for the
// line that announces where it listens. It stops with the test.
func startReferenceAPI(t *testing.T, trust referenceAssertions) *referenceAPI {
	t.Helper()
	module := filepath.Join("..", "..", "..", "test", "refapi")
	dir := t.TempDir()
	bin := filepath.Join(dir, "refapi")
	build := exec.CommandContext(t.Context(), "go", "build", "-o", bin, ".") //nolint:gosec // G204: a path the test chose.
	build.Dir = module
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build test/refapi: %v\n%s", err, out)
	}

	caFile := filepath.Join(dir, "ca.pem")
	addr := os.Getenv("PLUX_E2E_REFAPI_ADDR")
	if addr == "" {
		addr = "127.0.0.1:0"
	}
	ctx, cancel := context.WithCancel(context.Background())
	args := []string{
		"-addr", addr, "-ca-out", caFile, "-track-step", "200ms",
		"-assertion-jwks", trust.jwks, "-assertion-issuer", trust.issuer, "-assertion-audience", trust.audience,
	}
	cmd := exec.CommandContext(ctx, bin, args...) //nolint:gosec // G204: the binary the test built.
	cmd.Cancel = func() error { return cmd.Process.Signal(os.Interrupt) }
	cmd.WaitDelay = 5 * time.Second
	stderr := &bytes.Buffer{}
	cmd.Stderr = stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() { cancel(); _ = cmd.Wait() })

	lines := make(chan string, 1)
	go func() {
		line, _ := bufio.NewReader(stdout).ReadString('\n')
		lines <- line
	}()
	var line string
	select {
	case line = <-lines:
	case <-time.After(30 * time.Second):
		t.Fatal("the reference backend did not announce where it listens")
	}
	const prefix = "refapi listening on "
	if !strings.HasPrefix(line, prefix) {
		t.Fatalf("the reference backend said %q, want %q…\n%s", line, prefix, stderr)
	}
	pem, err := os.ReadFile(caFile)
	if err != nil {
		t.Fatal(err)
	}
	return &referenceAPI{
		url:    strings.TrimSpace(strings.TrimPrefix(line, prefix)),
		ca:     base64.StdEncoding.EncodeToString(pem),
		caFile: caFile,
		stderr: stderr,
	}
}

// pointAtReferenceAPI rewrites the project's documents so that its
// environments and its declared network domain name the reference backend
// at url rather than refapi.invalid: the URL's origin in every value, the
// host in every domain list. It fails if the project names refapi.invalid
// nowhere.
func pointAtReferenceAPI(t *testing.T, project, url string) {
	t.Helper()
	const placeholder = "refapi.invalid"
	host := strings.TrimPrefix(url, "https://")
	host = host[:strings.LastIndex(host, ":")]
	rewrote := 0
	err := filepath.WalkDir(project, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) != ".json" {
			return err
		}
		data, err := os.ReadFile(path) //nolint:gosec // G304: inside the project the test copied.
		if err != nil {
			return err
		}
		out := strings.ReplaceAll(string(data), "https://"+placeholder, url)
		out = strings.ReplaceAll(out, `"`+placeholder+`"`, `"`+host+`"`)
		if out == string(data) {
			return nil
		}
		rewrote++
		return os.WriteFile(path, []byte(out), 0o600)
	})
	if err != nil {
		t.Fatal(err)
	}
	if rewrote == 0 {
		t.Fatalf("%s names %s nowhere", project, placeholder)
	}
}

// accessibilityTreePass builds the app for the emulator with the flows'
// defines and its UiAutomator tests (AccessibilityTreeTest, in the app's
// androidTest sources), installs both and runs the tests: they walk the
// pages through Android's accessibility tree and assert labels, reading
// order and target sizes, once in English and once with the app in Arabic.
// The app starts from a clean install, so it signs in anew.
func accessibilityTreePass(ctx context.Context, t *testing.T, app referenceApp, dir, device string, defines []string) {
	t.Helper()
	run := func(name string, args ...string) string {
		t.Helper()
		cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // G204: flutter, the SDK's tools and the device the developer named.
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s %s: %v\n%s", filepath.Base(name), strings.Join(args, " "), err, tail(string(out), 150))
		}
		return string(out)
	}
	adb := "adb"
	for _, v := range []string{"ANDROID_HOME", "ANDROID_SDK_ROOT"} {
		if sdk := os.Getenv(v); sdk != "" {
			adb = filepath.Join(sdk, "platform-tools", "adb")
			break
		}
	}
	// Only the emulator's ABI is built, as flutter test builds it for the
	// flows: the WebAssembly runtime's native build has no 32-bit ARM target.
	abi := strings.TrimSpace(run(adb, "-s", device, "shell", "getprop", "ro.product.cpu.abi"))
	platform, ok := map[string]string{"x86_64": "android-x64", "arm64-v8a": "android-arm64"}[abi]
	if !ok {
		t.Fatalf("%s's accessibility-tree pass: no Flutter target platform for the emulator's ABI %q", app.dir, abi)
	}
	run(os.Getenv("PLUX_E2E_FLUTTER"), append([]string{"build", "apk", "--debug", "--target-platform=" + platform, "--target=" + app.a11yTarget}, defines...)...)
	run(filepath.Join(dir, "android", "gradlew"), "-p", filepath.Join(dir, "android"), "--no-daemon", "--console=plain", ":app:assembleDebugAndroidTest")
	apks := filepath.Join(dir, "build", "app", "outputs", "apk")
	run(adb, "-s", device, "install", "-r", "-t", filepath.Join(dir, "build", "app", "outputs", "flutter-apk", "app-debug.apk"))
	run(adb, "-s", device, "install", "-r", "-t", filepath.Join(apks, "androidTest", "debug", "app-debug-androidTest.apk"))
	run(adb, "-s", device, "shell", "pm", "clear", app.id)
	out := run(adb, "-s", device, "shell", "am", "instrument", "-w",
		"-e", "class", app.id+".AccessibilityTreeTest", app.id+".test/androidx.test.runner.AndroidJUnitRunner")
	if !strings.Contains(out, "OK (") {
		t.Fatalf("%s's accessibility-tree pass on %s:\n%s", app.dir, device, tail(out, 150))
	}
	t.Logf("%s's accessibility-tree pass on %s:\n%s", app.dir, device, tail(out, 10))
}

// accessibilityTreePassIOS is accessibilityTreePass for a simulator: it builds
// the app for the simulator with the flows' defines and its a11yTarget, then
// runs its XCUITests (AccessibilityTreeTests, in ios/RunnerUITests) through
// the Runner scheme. The app starts from a clean install, so it signs in anew.
func accessibilityTreePassIOS(ctx context.Context, t *testing.T, app referenceApp, dir, device string, defines []string) {
	t.Helper()
	build := exec.CommandContext(ctx, os.Getenv("PLUX_E2E_FLUTTER"), append([]string{"build", "ios", "--simulator", "--debug", "--target=" + app.a11yTarget}, defines...)...) //nolint:gosec // G204: the flutter the developer named.
	build.Dir = dir
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("flutter build ios for %s's accessibility-tree pass: %v\n%s", app.dir, err, tail(string(out), 150))
	}
	_ = exec.CommandContext(ctx, "xcrun", "simctl", "uninstall", device, app.iosID).Run() //nolint:gosec // G204: the simulator the developer named.
	args := []string{
		"test", "-workspace", "ios/Runner.xcworkspace", "-scheme", "Runner", "-configuration", "Debug",
		"-destination", "platform=iOS Simulator,id=" + device, "-parallel-testing-enabled", "NO",
		"-only-testing:RunnerUITests/AccessibilityTreeTests",
	}
	if bundle := os.Getenv("PLUX_E2E_XCRESULT"); bundle != "" {
		args = append(args, "-resultBundlePath", strings.TrimSuffix(bundle, ".xcresult")+"-a11y.xcresult")
	}
	xcodebuild := exec.CommandContext(ctx, "xcodebuild", args...) //nolint:gosec // G204: Xcode and the simulator the developer named.
	xcodebuild.Dir = dir
	out, err := xcodebuild.CombinedOutput()
	if err != nil || !strings.Contains(string(out), "** TEST SUCCEEDED **") {
		t.Fatalf("%s's accessibility-tree pass on %s: %v\n%s\n%s", app.dir, device, err, xctestFailures(string(out)), tail(string(out), 150))
	}
	t.Logf("%s's accessibility-tree pass on %s:\n%s", app.dir, device, tail(string(out), 10))
}

// xctestFailures is the assertions xcodebuild reported failing, the lines
// "<file>:<line>: error: -[<class> <test>] : <message>": XCTest's query log
// runs to thousands of lines, so a failure's tail rarely holds them.
func xctestFailures(out string) string {
	var failed []string
	for line := range strings.Lines(out) {
		if strings.Contains(line, ": error: -[") {
			failed = append(failed, strings.TrimRight(line, "\n"))
		}
	}
	if len(failed) == 0 {
		return "(no failed assertion reported)"
	}
	return "failed assertions:\n" + strings.Join(failed, "\n")
}

// flowsTail is how many lines of a failed run's output the driver prints.
const flowsTail = 300
