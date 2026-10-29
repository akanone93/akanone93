// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package builderrole

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/builder"
	"github.com/nightCode42/plux3/backend/internal/fnpb"
	"github.com/nightCode42/plux3/backend/internal/fnpb/fnpbconnect"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
)

// languageFixture serves a role whose language server is `cat`: it hands every
// framed message it reads straight back, which is all the role's relay, the
// sequencing and the session slots need. The policy and the framing are tested
// against a protocol-aware stand-in in package builder.
func languageFixture(t *testing.T, server string, sessions int) (fnpbconnect.BuilderServiceClient, *fnpb.LanguageOpenRequest) {
	t.Helper()
	sdk := t.TempDir()
	for name, body := range map[string]string{"Cargo.toml": "[package]\nname = \"plux-fn\"\n", "src/lib.rs": "// sdk\n"} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(sdk, name)), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(sdk, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	s, err := New(t.Context(), Options{
		Listen: "127.0.0.1:0", TLS: &tls.Config{ClientAuth: tls.RequireAndVerifyClientCert},
		Tools: builder.Config{
			Sandbox: builder.NoSandbox{}, Clock: time.Now, Identity: "builder-test", SDKDir: sdk,
			Cargo: "true", Rustc: "true", WasmOpt: []string{"true"},
		},
		Unsandboxed: true, Limits: limits.Defaults(), Concurrency: 1,
		Clock: time.Now, Random: rand.Reader,
		LanguageServer: server, LanguageSessions: sessions,
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(s.http.Handler)
	t.Cleanup(srv.Close)
	t.Cleanup(func() { _ = s.Stop(t.Context()) })
	files := archive(t, map[string]string{"Cargo.toml": "[package]\nname = \"fee\"\nversion = \"0.1.0\"\n", "Cargo.lock": lockFile, "vendor/": "", "src/lib.rs": "", "function.json": declaration})
	sum := sha256.Sum256(files)
	return fnpbconnect.NewBuilderServiceClient(srv.Client(), srv.URL),
		&fnpb.LanguageOpenRequest{Source: files, SourceSha256: hex.EncodeToString(sum[:]), Name: "bank.transferFee"}
}

// Verifies: FN-033.
func TestLanguageSessionRelaysSequencedMessages_FN_033(t *testing.T) {
	client, open := languageFixture(t, "cat", 1)
	ctx := t.Context()
	stream, err := client.LanguageOpen(ctx, connect.NewRequest(open))
	if err != nil {
		t.Fatal(err)
	}
	if !stream.Receive() || stream.Msg().GetOpened().GetSessionId() == "" || !strings.HasPrefix(stream.Msg().GetOpened().GetRootUri(), "file:///workspace/functions/") {
		t.Fatalf("first event = %v, err %v; want Opened with a session and a workspace root", stream.Msg(), stream.Err())
	}
	id := stream.Msg().GetOpened().GetSessionId()
	hover := `{"jsonrpc":"2.0","id":1,"method":"textDocument/hover","params":{}}`
	send := func(seq uint64, msg string) error {
		_, err := client.LanguageSend(ctx, connect.NewRequest(&fnpb.LanguageSendRequest{SessionId: id, Sequence: seq, Message: []byte(msg)}))
		return err
	}
	if err := send(1, hover); err != nil {
		t.Fatal(err)
	}
	if !stream.Receive() || !strings.Contains(string(stream.Msg().GetMessage()), `"method":"textDocument/hover"`) {
		t.Fatalf("message = %v, err %v; want the language server's echo", stream.Msg(), stream.Err())
	}
	if code := connect.CodeOf(send(1, hover)); code != connect.CodeFailedPrecondition {
		t.Errorf("a repeated sequence: code = %v; want failed precondition", code)
	}
	if code := connect.CodeOf(send(9, hover)); code != connect.CodeFailedPrecondition {
		t.Errorf("a gap: code = %v; want failed precondition", code)
	}
	_, err = client.LanguageSend(ctx, connect.NewRequest(&fnpb.LanguageSendRequest{SessionId: "0123", Sequence: 1, Message: []byte(hover)}))
	if connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("an unknown session: code = %v; want not found", connect.CodeOf(err))
	}
	_ = stream.Close()
}

// Verifies: FN-033, LIM-001.
func TestLanguageSessionsAreCappedAndSlotsAreReturned_FN_033(t *testing.T) {
	client, open := languageFixture(t, "cat", 1)
	first, err := client.LanguageOpen(t.Context(), connect.NewRequest(open))
	if err != nil {
		t.Fatal(err)
	}
	if !first.Receive() {
		t.Fatalf("the first session did not open: %v", first.Err())
	}
	second, err := client.LanguageOpen(t.Context(), connect.NewRequest(open))
	if err == nil {
		second.Receive()
		err = second.Err()
	}
	if connect.CodeOf(err) != connect.CodeResourceExhausted {
		t.Fatalf("a second session beyond the cap: error = %v; want resource exhausted", err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(20 * time.Second)
	for {
		again, err := client.LanguageOpen(t.Context(), connect.NewRequest(open))
		if err == nil && again.Receive() {
			_ = again.Close()
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the slot was not returned after the session ended: %v", err)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// Verifies: FN-033.
func TestLanguageOpenRefusesWithoutAServerOrAGoodRequest_FN_033(t *testing.T) {
	client, open := languageFixture(t, "", 0)
	stream, err := client.LanguageOpen(t.Context(), connect.NewRequest(open))
	if err == nil {
		stream.Receive()
		err = stream.Err()
	}
	if connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Errorf("no language server configured: error = %v; want failed precondition", err)
	}
	client, open = languageFixture(t, "cat", 1)
	open.SourceSha256 = strings.Repeat("0", 64)
	stream, err = client.LanguageOpen(t.Context(), connect.NewRequest(open))
	if err == nil {
		stream.Receive()
		err = stream.Err()
	}
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("a source that does not match its hash: error = %v; want invalid argument", err)
	}
}

// Verifies: FN-034.
func TestTestRunsInTheBuildSandboxAndReturnsOnlyResults_FN_034(t *testing.T) {
	files := archive(t, map[string]string{"Cargo.toml": "[package]\nname = \"fee\"\n", "Cargo.lock": lockFile, "vendor/": "", "function.json": declaration})
	s := newService(t, refusingSandbox{}, false, nil, nil)
	if _, err := s.Test(t.Context(), connect.NewRequest(&fnpb.TestRequest{Build: request(files)})); codeOf(err) != connect.CodeFailedPrecondition {
		t.Errorf("a refused sandbox: error = %v; want a failed precondition, as for a build", err)
	}
	if _, err := s.Test(t.Context(), connect.NewRequest(&fnpb.TestRequest{})); codeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("an empty request: error = %v; want invalid argument", err)
	}
	broken := archive(t, map[string]string{"Cargo.toml": "[package]"})
	resp, err := s.Test(t.Context(), connect.NewRequest(&fnpb.TestRequest{Build: request(broken)}))
	if err != nil || len(resp.Msg.GetDiagnosticsJson()) == 0 || len(resp.Msg.GetTestResults()) != 0 {
		t.Errorf("a source that does not build: %v, %v; want diagnostics and no results", resp, err)
	}
}
