// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

// Command walkthrough runs the scripted regulated-industry audit walkthrough,
// the exit criterion of phase 9 (spec §5.1, plan §5.12), against an installed
// Plux server, and verifies the signed evidence bundle it leaves.
//
//	walkthrough run    -server URL -org ID -app ID -plux plux -plux-load plux-load -out dir [flags]
//	walkthrough run    -dry-run
//	walkthrough verify -dir dir
//
// The administrator's token is read from WALKTHROUGH_ADMIN_TOKEN, never from
// a flag. The cluster, the chart and the hooks that need a browser, kubectl or
// helm are the business of test/walkthrough/walkthrough-kind.sh. Exit codes:
// 0 success, 1 a failed walkthrough or verification, 2 a usage error.
package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/nightCode42/plux3/tools/internal/walkthrough"
)

// Exit codes.
const (
	exitOK     = 0
	exitFailed = 1
	exitUsage  = 2
)

// File names inside the output directory.
const (
	bundleFile    = "evidence-bundle.json"
	keyFile       = "evidence-bundle.pub"
	evidenceDir   = "evidence"
	adminTokenEnv = "WALKTHROUGH_ADMIN_TOKEN"
	// The bootstrapped administrator's sign-in, for the step-ups of the admin
	// actor: the administrator has no account at the test provider.
	adminEmailEnv    = "WALKTHROUGH_ADMIN_EMAIL"
	adminPasswordEnv = "WALKTHROUGH_ADMIN_PASSWORD" //nolint:gosec // the name of a variable, not a credential
)

// main delegates to run so that the logic is testable.
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := run(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}

// run dispatches the subcommands.
func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		_, _ = fmt.Fprintln(stderr, "usage: walkthrough run|verify [flags]")
		return exitUsage
	}
	switch args[0] {
	case "run":
		return runWalkthrough(ctx, args[1:], stdout, stderr)
	case "verify":
		return verifyBundle(args[1:], stdout, stderr)
	default:
		_, _ = fmt.Fprintf(stderr, "walkthrough: unknown command %q\n", args[0])
		return exitUsage
	}
}

// options are the flags of `walkthrough run`.
type options struct {
	server, org, app, fleetEnv, out, project string
	fnSDK                                    string
	plux, pluxLoad, caFile                   string
	listen, advertise                        string
	idpPort, sinkPort                        string
	idpCert, idpKey, idpCA                   string
	devices                                  int
	stage, propagation, upgradeWindow        time.Duration
	tokenCmd, stepUpCmd, auditKeyCmd         string
	upgradeCmd, viewerCmd                    string
	with                                     string
	dryRun                                   bool
}

// parseOptions reads the flags of `walkthrough run`.
func parseOptions(args []string, stderr io.Writer) (options, error) {
	var o options
	fs := flag.NewFlagSet("walkthrough run", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&o.server, "server", "", "public base URL of the installation")
	fs.StringVar(&o.org, "org", "", "organisation ID")
	fs.StringVar(&o.app, "app", "", "app ID")
	fs.StringVar(&o.fleetEnv, "fleet-env", "staging", "non-production environment the synthetic devices belong to")
	fs.StringVar(&o.out, "out", "walkthrough-out", "directory for the evidence and the signed bundle")
	fs.StringVar(&o.fnSDK, "fn-sdk", "sdk/fn", "the function SDK: plux-fn and the example crates the project's functions are built from")
	fs.StringVar(&o.project, "project", "schema/testdata/documents/plux_bank", "the Plux Bank project the steps publish (copied; never changed in place)")
	fs.StringVar(&o.plux, "plux", "plux", "path of the plux CLI")
	fs.StringVar(&o.pluxLoad, "plux-load", "plux-load", "path of plux-load")
	fs.StringVar(&o.caFile, "ca-file", "", "PEM file of the CA that signed the server's certificate")
	fs.StringVar(&o.listen, "listen", "0.0.0.0", "address the OIDC test provider and the syslog sink listen on")
	fs.StringVar(&o.advertise, "advertise", "", "address the installation reaches them on (the kind network's gateway)")
	fs.StringVar(&o.idpPort, "idp-port", "18443", "port of the OIDC test provider")
	fs.StringVar(&o.idpCert, "idp-cert", "", "PEM certificate the OIDC test provider serves, issued for -advertise by -idp-ca; empty makes one")
	fs.StringVar(&o.idpKey, "idp-key", "", "PEM key of -idp-cert")
	fs.StringVar(&o.idpCA, "idp-ca", "", "PEM authority that issued -idp-cert, which the installation trusts")
	fs.StringVar(&o.sinkPort, "sink-port", "16514", "port of the syslog sink")
	fs.IntVar(&o.devices, "devices", 300, "devices in a synthetic fleet")
	fs.DurationVar(&o.stage, "stage-duration", time.Minute, "duration of each rollout stage")
	fs.DurationVar(&o.propagation, "propagation", 60*time.Second, "bound on a control document reaching the fleet (NFR-024)")
	fs.DurationVar(&o.upgradeWindow, "upgrade-window", 8*time.Minute, "how long the fleet runs while the chart is upgraded")
	fs.StringVar(&o.tokenCmd, "token-cmd", "", "shell command printing an access token for $WALKTHROUGH_ACTOR ($WALKTHROUGH_EMAIL)")
	fs.StringVar(&o.stepUpCmd, "stepup-cmd", "", "shell command completing the step-up at $WALKTHROUGH_STEPUP_URL for $WALKTHROUGH_ACTOR")
	fs.StringVar(&o.auditKeyCmd, "audit-key-cmd", "", "shell command printing the audit public key as id=base64")
	fs.StringVar(&o.upgradeCmd, "upgrade-cmd", "", "shell command that rolls the chart upgrade")
	fs.StringVar(&o.viewerCmd, "viewer-cmd", "", "shell command opening $WALKTHROUGH_RECONSTRUCTION for $WALKTHROUGH_DEVICE in the viewer on an emulator")
	fs.StringVar(&o.with, "with", "", "comma-separated optional steps to run, such as 07b-viewer-on-emulator")
	fs.BoolVar(&o.dryRun, "dry-run", false, "print the plan and exit; starts and changes nothing")
	if err := fs.Parse(args); err != nil {
		return o, fmt.Errorf("parse the flags: %w", err)
	}
	return o, o.validate()
}

// validate checks the flags a real run needs.
func (o options) validate() error {
	if o.dryRun {
		return nil
	}
	for name, v := range map[string]string{
		"-server": o.server, "-org": o.org, "-app": o.app, "-advertise": o.advertise, "-token-cmd": o.tokenCmd,
		"-audit-key-cmd": o.auditKeyCmd, "-upgrade-cmd": o.upgradeCmd,
	} {
		if v == "" {
			return fmt.Errorf("%s is required", name)
		}
	}
	if os.Getenv(adminTokenEnv) == "" {
		return fmt.Errorf("%s is not set", adminTokenEnv)
	}
	return nil
}

// runWalkthrough is `walkthrough run`.
func runWalkthrough(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	o, err := parseOptions(args, stderr)
	if errors.Is(err, flag.ErrHelp) {
		return exitOK
	}
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "walkthrough:", err)
		return exitUsage
	}
	if o.dryRun {
		(&walkthrough.Runner{Steps: walkthrough.Steps(&walkthrough.Env{})}).Plan(stdout)
		return exitOK
	}
	if err := execute(ctx, o, stdout); err != nil {
		_, _ = fmt.Fprintln(stderr, "walkthrough:", err)
		return exitFailed
	}
	return exitOK
}

// execute prepares the environment, runs the steps and seals the evidence.
func execute(ctx context.Context, o options, stdout io.Writer) error {
	store, err := walkthrough.NewStore(filepath.Join(o.out, evidenceDir))
	if err != nil {
		return fmt.Errorf("evidence store: %w", err)
	}
	env, closeAll, err := buildEnv(o)
	if err != nil {
		return err
	}
	defer closeAll()
	runner := &walkthrough.Runner{Steps: walkthrough.Steps(env), Store: store, Out: stdout, Now: time.Now, Enabled: enabledSteps(o.with)}
	st, runErr := runner.Run(ctx)
	if err := seal(o.out, st); err != nil {
		return errors.Join(runErr, err)
	}
	if runErr != nil {
		return fmt.Errorf("the walkthrough failed: %w", runErr)
	}
	return nil
}

// enabledSteps turns the -with list into the runner's set.
func enabledSteps(with string) map[string]bool {
	set := map[string]bool{}
	for _, id := range strings.Split(with, ",") {
		if id = strings.TrimSpace(id); id != "" {
			set[id] = true
		}
	}
	return set
}

// seal signs the statement with a key made for the run and writes the bundle
// and the public half of the key. The private key is dropped with the process.
func seal(out string, st walkthrough.Statement) error {
	pub, priv, err := walkthrough.GenerateKey(rand.Reader)
	if err != nil {
		return fmt.Errorf("signing key: %w", err)
	}
	b, err := walkthrough.Seal(st, priv)
	if err != nil {
		return fmt.Errorf("seal the statement: %w", err)
	}
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return fmt.Errorf("encode the bundle: %w", err)
	}
	if err := os.WriteFile(filepath.Join(out, bundleFile), append(data, '\n'), 0o640); err != nil { //nolint:gosec // evidence is for auditors to read
		return fmt.Errorf("write the bundle: %w", err)
	}
	enc := base64.StdEncoding.EncodeToString(pub) + "\n"
	if err := os.WriteFile(filepath.Join(out, keyFile), []byte(enc), 0o640); err != nil { //nolint:gosec // a public key
		return fmt.Errorf("write the public key: %w", err)
	}
	return nil
}

// verifyBundle is `walkthrough verify`: the signature first, then the files.
func verifyBundle(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("walkthrough verify", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dir := fs.String("dir", "walkthrough-out", "the output directory of a run")
	pubFile := fs.String("key", "", "file with the base64 public key the bundle must be signed with (default: the key beside the bundle)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}
	st, err := verifyDir(*dir, *pubFile)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "walkthrough:", err)
		return exitFailed
	}
	_, _ = fmt.Fprintf(stdout, "evidence bundle verified: %d steps, passed=%v, transcript sha256 %s\n", len(st.Steps), st.Passed, st.TranscriptSHA256)
	if !st.Passed {
		return exitFailed
	}
	return exitOK
}

// verifyDir checks the bundle of a run directory against a public key and
// re-hashes the evidence files.
func verifyDir(dir, pubFile string) (walkthrough.Statement, error) {
	if pubFile == "" {
		pubFile = filepath.Join(dir, keyFile)
	}
	raw, err := os.ReadFile(pubFile) //nolint:gosec // an operator-chosen path
	if err != nil {
		return walkthrough.Statement{}, fmt.Errorf("read the public key: %w", err)
	}
	pub, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return walkthrough.Statement{}, errors.New("the public key is not a base64 Ed25519 key")
	}
	data, err := os.ReadFile(filepath.Join(dir, bundleFile)) //nolint:gosec // an operator-chosen path
	if err != nil {
		return walkthrough.Statement{}, fmt.Errorf("read the bundle: %w", err)
	}
	var b walkthrough.Bundle
	if err := json.Unmarshal(data, &b); err != nil {
		return walkthrough.Statement{}, fmt.Errorf("parse the bundle envelope: %w", err)
	}
	st, err := walkthrough.Verify(b, pub)
	if err != nil {
		return st, fmt.Errorf("verify the bundle: %w", err)
	}
	if err := walkthrough.CheckFiles(st, filepath.Join(dir, evidenceDir)); err != nil {
		return st, fmt.Errorf("check the evidence files: %w", err)
	}
	return st, nil
}

// buildEnv starts the services of the run and wires the environment of the
// steps. The returned function stops them.
func buildEnv(o options) (*walkthrough.Env, func(), error) {
	httpClient, err := clientFor(o.caFile)
	if err != nil {
		return nil, nil, err
	}
	work := filepath.Join(o.out, "work")
	project := filepath.Join(work, "project")
	fnSDK, err := filepath.Abs(o.fnSDK)
	if err != nil {
		return nil, nil, fmt.Errorf("-fn-sdk: %w", err)
	}
	if err := os.CopyFS(project, os.DirFS(o.project)); err != nil {
		return nil, nil, fmt.Errorf("copy the project: %w", err)
	}
	commander := &walkthrough.ExecCommander{
		Binary: o.plux, Server: o.server, Org: o.org, CAFile: o.caFile, Home: filepath.Join(work, "home"),
		Initial: map[string]string{"admin": os.Getenv(adminTokenEnv)},
	}
	if err := os.MkdirAll(commander.Home, 0o700); err != nil {
		return nil, nil, fmt.Errorf("create the CLI home: %w", err)
	}
	idp, sink, err := startServices(o)
	if err != nil {
		return nil, nil, err
	}
	shared := hookEnvironment(o, idp)
	commander.StepUp = shellStepUp(o.stepUpCmd, adminSignIn(), shared...)
	env := &walkthrough.Env{
		Plux: commander, Creds: commander,
		Admin: &walkthrough.ConnectClient{
			BaseURL: o.server, Token: os.Getenv(adminTokenEnv), HTTP: httpClient,
			PublicURL: os.Getenv("PLUX_PUBLIC_URL"), Actor: "admin", StepUp: commander.StepUp,
		},
		Public: &walkthrough.ConnectClient{BaseURL: o.server, HTTP: httpClient},
		SCIM:   &walkthrough.HTTPSCIMClient{BaseURL: o.server, HTTP: httpClient},
		Fleet:  &walkthrough.ExecFleet{Binary: o.pluxLoad, Server: o.server, App: o.app, CAFile: o.caFile, Dir: work, Seed: 1},
		Hooks:  hooksFor(o, shared...),
		IDP:    idp, Sink: sink, SinkAddress: net.JoinHostPort(o.advertise, o.sinkPort),
		Server: o.server, Org: o.org, App: o.app, Project: project, FnSDK: fnSDK, TempDir: work,
		ProdEnv: "production", FleetEnv: o.fleetEnv, Devices: o.devices,
		StageDuration: o.stage, Propagation: o.propagation, UpgradeWindow: o.upgradeWindow,
		Now: time.Now, Sleep: walkthrough.SleepContext,
	}
	return env, func() { _ = idp.Close(); _ = sink.Close() }, nil
}

// startServices starts the OIDC test provider and the syslog sink.
func startServices(o options) (*walkthrough.IDP, *walkthrough.Sink, error) {
	hosts := []string{o.advertise}
	idp, err := startIDP(o, hosts)
	if err != nil {
		return nil, nil, fmt.Errorf("start the OIDC test provider: %w", err)
	}
	sink, err := walkthrough.NewSink(net.JoinHostPort(o.listen, o.sinkPort), hosts, time.Now, rand.Reader)
	if err != nil {
		_ = idp.Close()
		return nil, nil, fmt.Errorf("start the syslog sink: %w", err)
	}
	return idp, sink, nil
}

// startIDP starts the OIDC test provider, with the certificate of the flags
// when they name one.
func startIDP(o options, hosts []string) (*walkthrough.IDP, error) {
	listen, advertise := net.JoinHostPort(o.listen, o.idpPort), net.JoinHostPort(o.advertise, o.idpPort)
	if o.idpCert == "" {
		return walkthrough.NewIDP(listen, advertise, hosts, "plux-walkthrough", randomSecret(), time.Now, rand.Reader) //nolint:wrapcheck // the caller wraps
	}
	cert, err := tls.LoadX509KeyPair(o.idpCert, o.idpKey)
	if err != nil {
		return nil, fmt.Errorf("-idp-cert: %w", err)
	}
	caPEM, err := os.ReadFile(o.idpCA)
	if err != nil {
		return nil, fmt.Errorf("-idp-ca: %w", err)
	}
	return walkthrough.NewIDPWithCertificate(listen, advertise, cert, caPEM, "plux-walkthrough", randomSecret(), time.Now, rand.Reader) //nolint:wrapcheck // the caller wraps
}

// randomSecret returns a client secret for the test provider, made for the run.
func randomSecret() string {
	raw := make([]byte, 24)
	if _, err := io.ReadFull(rand.Reader, raw); err != nil {
		return "unavailable"
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}

// clientFor returns an HTTP client that trusts the CA file, if given.
func clientFor(caFile string) (*http.Client, error) {
	client := &http.Client{Timeout: 60 * time.Second}
	if caFile == "" {
		return client, nil
	}
	pemData, err := os.ReadFile(caFile) //nolint:gosec // an operator-chosen path
	if err != nil {
		return nil, fmt.Errorf("read the CA file: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pemData) {
		return nil, errors.New("the CA file holds no certificate")
	}
	client.Transport = &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}}
	return client, nil
}

// hookEnvironment is what the person-facing hooks (the token and the step-up)
// are told about the run besides the person: where the installation is, which
// organisation the run acts in and how to reach the OIDC test provider, whose
// CA exists only inside this process. None of it is secret.
func hookEnvironment(o options, idp *walkthrough.IDP) []string {
	env := []string{
		"WALKTHROUGH_SERVER=" + o.server, "WALKTHROUGH_ORG=" + o.org,
		"WALKTHROUGH_IDP_ISSUER=" + idp.Issuer(), "WALKTHROUGH_IDP_CA_PEM=" + idp.CAPEM(),
	}
	if o.caFile != "" {
		env = append(env, "WALKTHROUGH_CA_FILE="+o.caFile)
	}
	return env
}

// adminSignIn is what the step-up hook needs to sign the administrator in:
// their email and password, from the variables the script exports.
func adminSignIn() []string {
	return []string{"WALKTHROUGH_EMAIL=" + os.Getenv(adminEmailEnv), "WALKTHROUGH_PASSWORD=" + os.Getenv(adminPasswordEnv)}
}

// shellStepUp returns the step-up function that runs the operator's command
// with the shared environment, or nil when none was given. The person who
// steps up is named by their email: a person step 1 provisions by theirs, the
// administrator by the bootstrapped email and password.
func shellStepUp(command string, admin []string, shared ...string) walkthrough.StepUpFunc {
	if command == "" {
		return nil
	}
	return func(ctx context.Context, actor, url string) error {
		env := append(slices.Clone(shared), "WALKTHROUGH_ACTOR="+actor, "WALKTHROUGH_STEPUP_URL="+url)
		if email, ok := walkthrough.EmailOf(actor); ok {
			env = append(env, "WALKTHROUGH_EMAIL="+email)
		} else if actor == "admin" {
			env = append(env, admin...)
		}
		if _, err := walkthrough.ShellHook(ctx, command, env...); err != nil {
			return fmt.Errorf("step-up hook: %w", err)
		}
		return nil
	}
}

// hooksFor wires the shell commands of the flags into the steps' hooks; the
// token hook also gets the shared environment.
func hooksFor(o options, shared ...string) walkthrough.Hooks {
	h := walkthrough.Hooks{
		IssueToken: func(ctx context.Context, actor, email string) (string, error) {
			who := append(slices.Clone(shared), "WALKTHROUGH_ACTOR="+actor, "WALKTHROUGH_EMAIL="+email)
			out, err := walkthrough.ShellHook(ctx, o.tokenCmd, who...)
			if err != nil {
				return "", fmt.Errorf("token hook: %w", err)
			}
			return strings.TrimSpace(out), nil
		},
		AuditKey: func(ctx context.Context) (string, error) {
			out, err := walkthrough.ShellHook(ctx, o.auditKeyCmd)
			if err != nil {
				return "", fmt.Errorf("audit key hook: %w", err)
			}
			return out, nil
		},
		Upgrade: func(ctx context.Context) error {
			if _, err := walkthrough.ShellHook(ctx, o.upgradeCmd); err != nil {
				return fmt.Errorf("upgrade hook: %w", err)
			}
			return nil
		},
	}
	if o.viewerCmd != "" {
		h.Viewer = func(ctx context.Context, device, reconstruction string) error {
			if _, err := walkthrough.ShellHook(ctx, o.viewerCmd, "WALKTHROUGH_DEVICE="+device, "WALKTHROUGH_RECONSTRUCTION="+reconstruction); err != nil {
				return fmt.Errorf("viewer hook: %w", err)
			}
			return nil
		}
	}
	return h
}
