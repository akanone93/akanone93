// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
)

const goldenFonts = "../../../schema/goldens/fonts"

// fakeGolden is GoldenService with the baselines it was given and the
// requests it received.
type fakeGolden struct {
	pluxv1connect.UnimplementedGoldenServiceHandler
	mu        sync.Mutex
	baselines map[string][]byte
	uploads   []*pluxv1.UploadRunRequest
	approved  []*pluxv1.ApproveRequest
}

func (f *fakeGolden) GetBaselines(context.Context, *connect.Request[pluxv1.GetBaselinesRequest]) (*connect.Response[pluxv1.GetBaselinesResponse], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return connect.NewResponse(&pluxv1.GetBaselinesResponse{ImageSha256: f.baselines}), nil
}

func (f *fakeGolden) UploadRun(_ context.Context, r *connect.Request[pluxv1.UploadRunRequest]) (*connect.Response[pluxv1.UploadRunResponse], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.uploads = append(f.uploads, r.Msg)
	return connect.NewResponse(&pluxv1.UploadRunResponse{RunId: "run-1"}), nil
}

func (f *fakeGolden) Approve(_ context.Context, r *connect.Request[pluxv1.ApproveRequest]) (*connect.Response[pluxv1.ApproveResponse], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.approved = append(f.approved, r.Msg)
	return connect.NewResponse(&pluxv1.ApproveResponse{ApprovedCaseIds: []string{"a", "b"}}), nil
}

func (*fakeGolden) GetRunStatus(_ context.Context, r *connect.Request[pluxv1.GetRunStatusRequest]) (*connect.Response[pluxv1.GetRunStatusResponse], error) {
	if len(r.Msg.GetBundleSha256()) != sha256.Size || len(r.Msg.GetAppSha256()) != sha256.Size {
		return nil, connect.NewError(connect.CodeInvalidArgument, nil)
	}
	return connect.NewResponse(&pluxv1.GetRunStatusResponse{Found: true, RunId: "run-1", Cases: 10, Differences: 2}), nil
}

// goldenProject is the starter project with a golden matrix narrowed to one
// frame, one locale and one text scale, so a run renders two cases per page.
func goldenProject(t *testing.T) string {
	t.Helper()
	dir := copyProject(t)
	path := filepath.Join(dir, "app.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var app map[string]any
	if err := json.Unmarshal(data, &app); err != nil {
		t.Fatal(err)
	}
	app["goldens"] = map[string]any{"matrix": map[string]any{
		"frames": []string{"small-360x640"}, "themes": []string{"light", "dark"}, "textScales": []float64{1}, "locales": []string{"en"},
	}}
	data, err = json.Marshal(app)
	if err != nil {
		t.Fatal(err)
	}
	putFile(t, path, string(data))
	return dir
}

// fakeFlutterRendering is a flutter executable that "renders" every case the
// generated test names: it writes a file per case to the directory the test
// writes to and reports each as passed.
func fakeFlutterRendering(t *testing.T, content string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the flutter test double is a POSIX shell script, which Windows cannot execute; the comparison logic is OS-independent and runs on Linux and macOS")
	}
	path := filepath.Join(t.TempDir(), "flutter")
	script := `#!/bin/sh
out=$(sed -n "s/^const _out = '\(.*\)';$/\1/p" test/golden_test.dart)
mkdir -p "$out"
i=0
id=2
grep -o "testWidgets('[^']*'" test/golden_test.dart | sed "s/testWidgets('//;s/'$//" | while read -r name; do
  printf '{"test":{"id":%d,"name":"golden %s","suiteID":0,"groupIDs":[1]},"type":"testStart","time":1000}\n' "$id" "$name"
  printf '` + content + `%s' "$name" > "$out/$(printf %04d "$i").png"
  printf '{"testID":%d,"result":"success","skipped":false,"hidden":false,"type":"testDone","time":1500}\n' "$id"
  i=$((i+1))
  id=$((id+1))
done
printf '{"success":true,"type":"done","time":2000}\n'
`
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil { //nolint:gosec // an executable test double
		t.Fatal(err)
	}
	return path
}

func goldenServer(t *testing.T, f *fakeGolden) string {
	t.Helper()
	mux := http.NewServeMux()
	mux.Handle(pluxv1connect.NewGoldenServiceHandler(f))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL
}

// Verifies: TST-003.
// plux golden run compares with the server's baselines, writes the images
// that differ, uploads the run with the bundle hashes and the images of the
// differing cases only, and exits 1 on differences.
func TestGoldenRunComparesAndUploads(t *testing.T) { //nolint:paralleltest // the environment is process-wide
	t.Setenv(tokenEnv, "plux_pat_golden")
	t.Setenv(serverEnv, "")
	t.Setenv(orgEnv, "")
	dir := goldenProject(t)
	flutter := fakeFlutterRendering(t, "image:")
	f := &fakeGolden{}
	server := goldenServer(t, f)
	args := []string{"golden", "run", "--server", server, "--app", "a1", "-C", dir, "--flutter", flutter, "--fonts", goldenFonts, "--upload", "--json"}

	// No baselines: every case is new.
	code, out, stderr := cli(t, t.TempDir(), args...)
	if code != exitFailed {
		t.Fatalf("new cases: exit %d\n%s%s", code, out, stderr)
	}
	var view struct {
		Cases       int
		Differences int
		Runs        map[string]string
	}
	if err := json.Unmarshal([]byte(out), &view); err != nil {
		t.Fatalf("output %q: %v", out, err)
	}
	if view.Cases != 10 || view.Differences != 10 || view.Runs["welcome"] != "run-1" {
		t.Errorf("view = %+v", view)
	}
	if len(f.uploads) != 1 {
		t.Fatalf("uploads = %d", len(f.uploads))
	}
	up := f.uploads[0]
	if up.GetPluginKey() != "welcome" || len(up.GetBundleSha256()) != sha256.Size || len(up.GetAppSha256()) != sha256.Size || len(up.GetCases()) != 10 {
		t.Errorf("upload = plugin %q, %d cases", up.GetPluginKey(), len(up.GetCases()))
	}
	for _, c := range up.GetCases() {
		sum := sha256.Sum256(c.GetImage())
		if len(c.GetImage()) == 0 || !bytes.Equal(sum[:], c.GetImageSha256()) {
			t.Errorf("case %s: image does not match its hash", c.GetCaseId())
		}
	}
	if _, err := os.Stat(filepath.Join(dir, ".plux", "golden", "welcome")); err != nil {
		t.Errorf("the differing images were not written: %v", err)
	}

	// With the baselines approved, a repeated run matches and sends no image.
	f.baselines = map[string][]byte{}
	for _, c := range up.GetCases() {
		f.baselines[c.GetCaseId()] = c.GetImageSha256()
	}
	if code, out, stderr = cli(t, t.TempDir(), args...); code != exitOK {
		t.Fatalf("matching run: exit %d\n%s%s", code, out, stderr)
	}
	for _, c := range f.uploads[1].GetCases() {
		if len(c.GetImage()) != 0 {
			t.Errorf("case %s matched its baseline and still carries an image", c.GetCaseId())
		}
	}

	// One image changes: only that case is a difference and carries its image.
	f.baselines[up.GetCases()[0].GetCaseId()] = bytes.Repeat([]byte{1}, sha256.Size)
	if code, _, _ = cli(t, t.TempDir(), args...); code != exitFailed {
		t.Errorf("changed run: exit %d", code)
	}
	changed := 0
	for _, c := range f.uploads[2].GetCases() {
		if len(c.GetImage()) != 0 {
			changed++
		}
	}
	if changed != 1 {
		t.Errorf("changed images sent = %d, want 1", changed)
	}
}

// Verifies: TST-003, A11Y-005.
// The generated test renders the matrix: the frame's size and ratio, the
// theme, the text scale and the locale, with the data sources mocked and
// the vendored fonts loaded.
func TestGoldenRunGeneratesTheMatrix(t *testing.T) { //nolint:paralleltest // the environment is process-wide
	t.Setenv(tokenEnv, "plux_pat_golden")
	t.Setenv(serverEnv, "")
	t.Setenv(orgEnv, "")
	dir := goldenProject(t)
	work := filepath.Join(t.TempDir(), "work")
	server := goldenServer(t, &fakeGolden{})
	code, _, stderr := cli(t, t.TempDir(), "golden", "run", "--server", server, "--app", "a1", "-C", dir, "--flutter", fakeFlutterRendering(t, "x"),
		"--fonts", goldenFonts, "--work", work, "--plugin", "welcome")
	if code != exitFailed {
		t.Fatalf("exit %d %s", code, stderr)
	}
	data, err := os.ReadFile(filepath.Join(work, "test", "golden_test.dart"))
	if err != nil {
		t.Fatal(err)
	}
	test := string(data)
	for _, want := range []string{
		"testWidgets('place/small-360x640/en/dark/x1'", "frame: const Size(360, 640)", "pixelRatio: 2", "themeMode: ThemeMode.dark", "textScale: 1", "locale: 'en'",
		"'NotoNaskhArabic'", "'NotoSansEthiopic'", "dataSources: _dataSources", "FontLoader",
	} {
		if !strings.Contains(test, want) {
			t.Errorf("golden_test.dart lacks %q", want)
		}
	}
	for _, font := range []string{"NotoSans_400Regular.ttf", "NotoNaskhArabic_700Bold.ttf", "NotoSansEthiopic_400Regular.ttf"} {
		if _, err := os.Stat(filepath.Join(work, "fonts", font)); err != nil {
			t.Errorf("font %s was not written: %v", font, err)
		}
	}
}

// Verifies: TST-003.
// plux golden fails without Flutter (PLX-9301), with fonts that do not match
// their pinned hash, and with too many cases for golden.casesPerRun.
func TestGoldenRunRefusals(t *testing.T) { //nolint:paralleltest // the environment is process-wide
	t.Setenv(tokenEnv, "plux_pat_golden")
	t.Setenv(serverEnv, "")
	t.Setenv(orgEnv, "")
	t.Setenv("PATH", t.TempDir())
	dir := goldenProject(t)
	server := goldenServer(t, &fakeGolden{})
	base := []string{"golden", "run", "--server", server, "--app", "a1", "-C", dir, "--fonts", goldenFonts}
	if code, _, stderr := cli(t, t.TempDir(), base...); code != exitFailed || !strings.Contains(stderr, "PLX-9301") {
		t.Errorf("no flutter: %d %s", code, stderr)
	}

	tampered := t.TempDir()
	for _, name := range []string{"VERSION", "NotoSans_400Regular.ttf"} {
		data, err := os.ReadFile(filepath.Join(goldenFonts, name))
		if err != nil {
			t.Fatal(err)
		}
		if name != "VERSION" {
			data = append(data, 0)
		}
		putFile(t, filepath.Join(tampered, name), string(data))
	}
	flutter := fakeFlutterRendering(t, "x")
	args := []string{"golden", "run", "--server", server, "--app", "a1", "-C", dir, "--fonts", tampered, "--flutter", flutter}
	if code, _, stderr := cli(t, t.TempDir(), args...); code != exitFailed || !strings.Contains(stderr, "PLX-9300") {
		t.Errorf("tampered fonts: %d %s", code, stderr)
	}

	// 5 pages x 4 frames x 2 themes x 2 text scales = 20 cases per locale.
	big := copyProject(t)
	app, err := os.ReadFile(filepath.Join(big, "app.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(app, &doc); err != nil {
		t.Fatal(err)
	}
	doc["supportedLocales"] = []string{"en", "de", "ar", "am", "fr", "es", "it", "pt", "nl", "sv", "da", "fi", "no", "pl", "cs", "ru", "tr", "el", "he", "hu", "ro", "bg", "uk", "hr", "sk", "sl", "lt", "lv", "et", "ja", "ko", "zh"}
	doc["goldens"] = map[string]any{"matrix": map[string]any{"locales": doc["supportedLocales"]}}
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	putFile(t, filepath.Join(big, "app.json"), string(data))
	args = []string{"golden", "run", "--server", server, "--app", "a1", "-C", big, "--fonts", goldenFonts, "--flutter", flutter}
	if code, _, stderr := cli(t, t.TempDir(), args...); code != exitFailed || !strings.Contains(stderr, "golden.casesPerRun") {
		t.Errorf("too many cases: %d %s", code, stderr)
	}
}

// Verifies: TST-003.
// plux golden approve sends the run and cases; plux golden status asks for the
// exact bundle hashes and exits 1 while differences are unapproved.
func TestGoldenApproveAndStatus(t *testing.T) { //nolint:paralleltest // the environment is process-wide
	t.Setenv(tokenEnv, "plux_pat_golden")
	t.Setenv(serverEnv, "")
	t.Setenv(orgEnv, "")
	f := &fakeGolden{}
	server := goldenServer(t, f)
	dir := goldenProject(t)
	code, out, stderr := cli(t, t.TempDir(), "golden", "approve", "--server", server, "--app", "a1", "-C", dir, "--run", "run-1", "--case", "a", "--case", "b")
	if code != exitOK || !strings.Contains(out, "Approved 2 case(s).") {
		t.Fatalf("approve: %d\n%s%s", code, out, stderr)
	}
	if got := f.approved[0]; got.GetRunId() != "run-1" || strings.Join(got.GetCaseIds(), ",") != "a,b" {
		t.Errorf("approve request = %v", got)
	}
	if code, _, _ = cli(t, t.TempDir(), "golden", "approve", "--server", server, "--app", "a1", "-C", dir); code != exitUsage {
		t.Errorf("approve without a run: %d, want usage", code)
	}
	code, out, stderr = cli(t, t.TempDir(), "golden", "status", "--server", server, "--app", "a1", "-C", dir)
	if code != exitFailed || !strings.Contains(out, "welcome: 2 of 10 cases unapproved (PLX-3062)") {
		t.Errorf("status: %d\n%s%s", code, out, stderr)
	}
	if code, _, _ = cli(t, t.TempDir(), "golden"); code != exitUsage {
		t.Errorf("no subcommand: %d, want usage", code)
	}
}

// Verifies: TST-003.
// The golden run renders with the real runtime: two runs of the starter
// project render the same bytes, so a baseline approved from one matches
// the other. It runs when the Flutter SDK is installed.
func TestGoldenRunRendersWithFlutter(t *testing.T) { //nolint:paralleltest // the environment is process-wide
	flutter, err := exec.LookPath("flutter")
	if err != nil {
		t.Skip("flutter is not installed: ", err)
	}
	runtime, err := filepath.Abs("../../../packages/plux_flutter")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(tokenEnv, "plux_pat_golden")
	t.Setenv(serverEnv, "")
	t.Setenv(orgEnv, "")
	dir := goldenProject(t)
	f := &fakeGolden{}
	server := goldenServer(t, f)
	args := []string{"golden", "run", "--server", server, "--app", "a1", "-C", dir, "--flutter", flutter, "--runtime", runtime, "--fonts", goldenFonts, "--upload"}
	code, out, stderr := cli(t, t.TempDir(), args...)
	if code != exitFailed || len(f.uploads) != 1 {
		t.Fatalf("first run: exit %d, %d uploads\n%s%s", code, len(f.uploads), out, stderr)
	}
	f.baselines = map[string][]byte{}
	for _, c := range f.uploads[0].GetCases() {
		if !bytes.HasPrefix(c.GetImage(), []byte("\x89PNG")) {
			t.Fatalf("case %s is not a PNG", c.GetCaseId())
		}
		f.baselines[c.GetCaseId()] = c.GetImageSha256()
	}
	if code, out, stderr = cli(t, t.TempDir(), args...); code != exitOK {
		t.Errorf("second run: exit %d (the render is not deterministic)\n%s%s", code, out, stderr)
	}
}
