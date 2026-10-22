// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

// Command webauthn-driver is the software authenticator and browser the
// audit walkthrough uses for the two things that need a person: signing in
// with a second factor, and confirming a step-up with a security key
// (SEC-103, GOV-030). It implements the two hooks of
// test/walkthrough/walkthrough-kind.sh:
//
//	webauthn-driver token    prints an access token for $WALKTHROUGH_ACTOR
//	                         (WALKTHROUGH_TOKEN_CMD);
//	webauthn-driver stepup   completes the step-up at $WALKTHROUGH_STEPUP_URL
//	                         for $WALKTHROUGH_ACTOR (WALKTHROUGH_STEPUP_CMD).
//
// It is configured through the environment the walkthrough gives its hooks:
// WALKTHROUGH_SERVER, WALKTHROUGH_ORG, WALKTHROUGH_ACTOR, WALKTHROUGH_EMAIL,
// WALKTHROUGH_STEPUP_URL and WALKTHROUGH_IDP_CA_PEM (the CA of the test
// provider), plus, from the script, WALKTHROUGH_DRIVER_DIR (a directory only
// the run's user can read, where the keys live), and optionally
// WALKTHROUGH_ORG_KEY (default "acme", the key `plux-server seed` gives the
// organisation), WALKTHROUGH_IDP_KEY (default
// "walkthrough-idp"), WALKTHROUGH_CA_FILE (the CA of the server's
// certificate) and WALKTHROUGH_WEBAUTHN_ORIGIN. Keys are never printed. Exit
// codes: 0 success, 1 a failed ceremony, 2 a usage or configuration error.
package main

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/nightCode42/plux3/tools/internal/webauthndriver"
)

// Exit codes.
const (
	exitOK     = 0
	exitFailed = 1
	exitUsage  = 2
)

// ceremonyTimeout bounds one invocation: a sign-in and a ceremony or two.
const ceremonyTimeout = 2 * time.Minute

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code := run(ctx, os.Args[1:], os.Getenv, os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}

// run dispatches the subcommands.
func run(ctx context.Context, args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	if len(args) != 1 || (args[0] != "token" && args[0] != "stepup") {
		_, _ = fmt.Fprintln(stderr, "usage: webauthn-driver token|stepup")
		return exitUsage
	}
	driver, err := newDriver(getenv)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "webauthn-driver: %v\n", err)
		return exitUsage
	}
	ctx, cancel := context.WithTimeout(ctx, ceremonyTimeout)
	defer cancel()
	if args[0] == "token" {
		return printToken(ctx, driver, stdout, stderr)
	}
	return completeStepUp(ctx, driver, getenv("WALKTHROUGH_STEPUP_URL"), stderr)
}

// printToken writes the access token and nothing else to standard output.
func printToken(ctx context.Context, d *webauthndriver.Driver, stdout, stderr io.Writer) int {
	token, err := d.Token(ctx)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "webauthn-driver: token: %v\n", err)
		return exitFailed
	}
	_, _ = fmt.Fprintln(stdout, token)
	return exitOK
}

// completeStepUp completes the step-up request at the URL.
func completeStepUp(ctx context.Context, d *webauthndriver.Driver, url string, stderr io.Writer) int {
	if err := d.StepUp(ctx, url); err != nil {
		_, _ = fmt.Fprintf(stderr, "webauthn-driver: stepup: %v\n", err)
		return exitFailed
	}
	return exitOK
}

// newDriver reads the configuration from the environment.
func newDriver(getenv func(string) string) (*webauthndriver.Driver, error) {
	for _, name := range []string{"WALKTHROUGH_SERVER", "WALKTHROUGH_ORG", "WALKTHROUGH_ACTOR", "WALKTHROUGH_EMAIL", "WALKTHROUGH_DRIVER_DIR", "WALKTHROUGH_IDP_CA_PEM"} {
		if getenv(name) == "" {
			return nil, fmt.Errorf("%s is not set", name)
		}
	}
	idp, err := httpClient([]byte(getenv("WALKTHROUGH_IDP_CA_PEM")), false)
	if err != nil {
		return nil, fmt.Errorf("WALKTHROUGH_IDP_CA_PEM: %w", err)
	}
	server, err := serverClient(getenv("WALKTHROUGH_CA_FILE"))
	if err != nil {
		return nil, err
	}
	d, err := webauthndriver.New(webauthndriver.Config{
		Server: getenv("WALKTHROUGH_SERVER"), Origin: getenv("WALKTHROUGH_WEBAUTHN_ORIGIN"),
		OrgID: getenv("WALKTHROUGH_ORG"), OrgKey: orDefault(getenv("WALKTHROUGH_ORG_KEY"), "acme"),
		ProviderKey: orDefault(getenv("WALKTHROUGH_IDP_KEY"), "walkthrough-idp"),
		Actor:       getenv("WALKTHROUGH_ACTOR"), Email: getenv("WALKTHROUGH_EMAIL"),
		Store:      webauthndriver.FileStore{Dir: getenv("WALKTHROUGH_DRIVER_DIR")},
		ServerHTTP: server, IDPHTTP: idp, Random: rand.Reader,
	})
	if err != nil {
		return nil, fmt.Errorf("webauthn-driver: %w", err)
	}
	return d, nil
}

// orDefault returns v, or def when v is empty.
func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// serverClient returns the client for the installation: the system roots and,
// when given, the CA file.
func serverClient(caFile string) (*http.Client, error) {
	if caFile == "" {
		return &http.Client{Timeout: time.Minute}, nil
	}
	pemData, err := os.ReadFile(caFile) //nolint:gosec // an operator-chosen path
	if err != nil {
		return nil, fmt.Errorf("WALKTHROUGH_CA_FILE: %w", err)
	}
	return httpClient(pemData, true)
}

// httpClient returns a client that trusts the CA in pemData, and the system
// roots too when withSystem is set. It never follows a redirect: the driver
// reads where the provider sends the browser, and a server's redirect is a
// failure it should report.
func httpClient(pemData []byte, withSystem bool) (*http.Client, error) {
	pool := x509.NewCertPool()
	if withSystem {
		var err error
		if pool, err = x509.SystemCertPool(); err != nil {
			return nil, fmt.Errorf("the system certificates: %w", err)
		}
	}
	if !pool.AppendCertsFromPEM(pemData) {
		return nil, errors.New("holds no certificate")
	}
	return &http.Client{
		Timeout:       time.Minute,
		Transport:     &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}, nil
}
