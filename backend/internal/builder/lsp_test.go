// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

// fakeLanguageServerEnv switches the test binary into a stand-in language
// server: rust-analyzer is not installed on every machine that runs the
// tests, and the bridge's behaviour does not depend on it, so the tests drive
// the real sandbox, framing and policy against this process instead.
const fakeLanguageServerEnv = "PLUX_FAKE_LANGUAGE_SERVER"

// TestMain turns the test binary into the stand-in language server when the
// language-server tests start it as a child; otherwise it runs the tests.
func TestMain(m *testing.M) {
	if os.Getenv(fakeLanguageServerEnv) == "1" {
		runFakeLanguageServer(os.Stdin, os.Stdout)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// runFakeLanguageServer speaks just enough LSP: it echoes the handshake and
// hover parameters, asks for configuration after the handshake, reports a
// diagnostic with its real workspace path, and ends or misbehaves on request.
func runFakeLanguageServer(in io.Reader, out io.Writer) {
	cwd, _ := os.Getwd()
	send := func(v any) {
		body, _ := json.Marshal(v)
		fmt.Fprintf(out, "Content-Length: %d\r\n\r\n%s", len(body), body)
	}
	r := bufio.NewReader(in)
	for {
		n := -1
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			line = strings.TrimSpace(line)
			if line == "" {
				break
			}
			if v, ok := strings.CutPrefix(line, "Content-Length: "); ok {
				n, _ = strconv.Atoi(v)
			}
		}
		body := make([]byte, n)
		if _, err := io.ReadFull(r, body); err != nil {
			return
		}
		var m struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
			Result json.RawMessage `json:"result"`
		}
		_ = json.Unmarshal(body, &m)
		switch {
		case m.Method == "" && m.Result != nil:
			send(map[string]any{"jsonrpc": "2.0", "method": "test/configResponse", "params": m.Result})
		case m.Method == "initialize" || m.Method == "textDocument/hover": //nolint:misspell // the LSP method name.
			send(map[string]any{"jsonrpc": "2.0", "id": m.ID, "result": m.Params})
		case m.Method == "initialized": //nolint:misspell // the LSP method name.
			send(map[string]any{"jsonrpc": "2.0", "id": 77, "method": "workspace/configuration", "params": map[string]any{"items": []any{map[string]any{"section": "rust-analyzer"}}}})
			send(map[string]any{"jsonrpc": "2.0", "method": "textDocument/publishDiagnostics", "params": map[string]any{
				"uri": "file://" + cwd + "/functions/loan.schedule/src/lib.rs", "diagnostics": []any{map[string]any{"message": "see " + cwd + "/x"}},
			}})
		case m.Method == "textDocument/didOpen":
			send(map[string]any{"jsonrpc": "2.0", "method": "test/opened", "params": m.Params})
		case m.Method == "textDocument/documentSymbol":
			var found []string
			_ = filepath.WalkDir(cwd, func(p string, d os.DirEntry, _ error) error {
				if d != nil && (d.Name() == "rust-analyzer.toml" || (d.IsDir() && d.Name() == ".cargo")) {
					found = append(found, strings.TrimPrefix(p, cwd))
				}
				return nil
			})
			send(map[string]any{"jsonrpc": "2.0", "id": m.ID, "result": found})
		case m.Method == "textDocument/rename":
			fmt.Fprintf(os.Stderr, "boom in %s/src\n", cwd)
			os.Exit(3)
		case m.Method == "textDocument/references":
			fmt.Fprintf(out, "Content-Length: 99999999\r\n\r\n")
		case m.Method == "exit":
			return
		}
	}
}

// languageFixture opens a session against the stand-in.
func languageFixture(t *testing.T, sb Sandbox, src FunctionSource, mut func(*LanguageOptions)) *LanguageSession {
	t.Helper()
	f := newFixture(t)
	f.cfg.Sandbox = sb
	o := LanguageOptions{
		Server: os.Args[0], Env: []string{fakeLanguageServerEnv + "=1"},
		Wall: 30 * time.Second, Memory: 0, MessageBytes: 1 << 16,
	}
	if mut != nil {
		mut(&o)
	}
	s, err := f.builder().OpenLanguage(t.Context(), src, o)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s
}

// next returns the next message or fails after a deadline.
func next(t *testing.T, s *LanguageSession) map[string]json.RawMessage {
	t.Helper()
	select {
	case raw, ok := <-s.Messages():
		if !ok {
			t.Fatalf("the session ended: %+v", s.Wait())
		}
		var m map[string]json.RawMessage
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatalf("message %q: %v", raw, err)
		}
		m["_raw"] = raw
		return m
	case <-time.After(20 * time.Second):
		t.Fatal("no message from the language server")
		return nil
	}
}

type rpc = map[string]any

// send sends the next sequenced message.
func send(t *testing.T, s *LanguageSession, seq *uint64, v any) {
	t.Helper()
	body, _ := json.Marshal(v)
	if err := s.Send(*seq, body); err != nil {
		t.Fatal(err)
	}
	*seq++
}

func loanSource() FunctionSource { return fnSource("loan.schedule", "loan", PlacementServer) }

// Verifies: FN-033.
func TestLanguageSessionForcesTheSettings_FN_033(t *testing.T) {
	s := languageFixture(t, NoSandbox{}, loanSource(), nil)
	seq := uint64(1)
	// The client asks for everything on, with its own roots.
	send(t, s, &seq, rpc{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": rpc{ //nolint:misspell // the LSP method name.
		"processId": 4242, "rootUri": "file:///etc", "rootPath": "/etc",
		"workspaceFolders": []any{rpc{"uri": "file:///etc", "name": "x"}},
		"initializationOptions": rpc{
			"cargo":     rpc{"buildScripts": rpc{"enable": true}},
			"procMacro": rpc{"enable": true}, "checkOnSave": true,
		},
	}})
	m := next(t, s)
	var got struct {
		Result struct {
			ProcessID *int `json:"processId"`
			RootURI   string
			Init      struct {
				Cargo struct {
					BuildScripts struct{ Enable bool }
				}
				ProcMacro struct{ Enable bool }
				Check     bool `json:"checkOnSave"`
			} `json:"initializationOptions"`
			Folders []struct{ URI string } `json:"workspaceFolders"`
		}
	}
	got.Result.RootURI = ""
	raw, _ := json.Marshal(map[string]json.RawMessage{"result": m["result"]})
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.Result.Init.Cargo.BuildScripts.Enable || got.Result.Init.ProcMacro.Enable || got.Result.Init.Check {
		t.Errorf("the client turned build scripts, procedural macros or check-on-save on: %s", m["result"])
	}
	if got.Result.ProcessID != nil {
		t.Error("the client's process id reached the server")
	}
	if bytes.Contains(m["result"], []byte("/etc")) {
		t.Errorf("the client's root reached the server: %s", m["result"])
	}
	// The server saw the sandbox's directory; the echo shows it as the
	// virtual workspace, so no host path reaches the client.
	if len(got.Result.Folders) != 1 || got.Result.Folders[0].URI != "file:///workspace" {
		t.Errorf("workspaceFolders = %+v; want the workspace", got.Result.Folders)
	}
}

// Verifies: FN-033.
func TestLanguageSessionAnswersConfigurationItself_FN_033(t *testing.T) {
	s := languageFixture(t, NoSandbox{}, loanSource(), nil)
	seq := uint64(1)
	send(t, s, &seq, rpc{"jsonrpc": "2.0", "method": "initialized", "params": rpc{}}) //nolint:misspell // the LSP method name.
	var sawDiagnostic, sawConfig bool
	for range 2 {
		m := next(t, s)
		switch string(m["method"]) {
		case `"textDocument/publishDiagnostics"`:
			sawDiagnostic = true
			if bytes.Contains(m["_raw"], []byte("plux-build-")) {
				t.Errorf("a host path reached the client: %s", m["_raw"])
			}
			if !bytes.Contains(m["_raw"], []byte("file:///workspace/functions/loan.schedule/src/lib.rs")) {
				t.Errorf("the document is not under the virtual workspace: %s", m["_raw"])
			}
		case `"test/configResponse"`:
			sawConfig = true
			if !bytes.Contains(m["params"], []byte(`"enable":false`)) {
				t.Errorf("the configuration answer = %s; want the forced settings", m["params"])
			}
		default:
			t.Errorf("unexpected message %s", m["_raw"])
		}
	}
	if !sawDiagnostic || !sawConfig {
		t.Errorf("diagnostic = %v, configuration answered = %v", sawDiagnostic, sawConfig)
	}
}

// Verifies: FN-033.
func TestLanguageSessionRefusesWhatTheEditorDoesNotNeed_FN_033(t *testing.T) {
	s := languageFixture(t, NoSandbox{}, loanSource(), nil)
	seq := uint64(1)
	cases := []struct {
		name string
		msg  rpc
	}{
		{"execute command", rpc{"jsonrpc": "2.0", "id": 5, "method": "workspace/executeCommand", "params": rpc{"command": "rust-analyzer.runSingle"}}},
		{"change configuration", rpc{"jsonrpc": "2.0", "id": 6, "method": "workspace/didChangeConfiguration", "params": rpc{}}},
		{"flycheck", rpc{"jsonrpc": "2.0", "id": 7, "method": "rust-analyzer/runFlycheck", "params": rpc{}}},
		{"outside the workspace", rpc{"jsonrpc": "2.0", "id": 8, "method": "textDocument/didOpen", "params": rpc{"textDocument": rpc{"uri": "file:///etc/passwd", "text": ""}}}},
		{"climbing out", rpc{"jsonrpc": "2.0", "id": 9, "method": "textDocument/hover", "params": rpc{"textDocument": rpc{"uri": "file:///workspace/../etc/passwd"}}}},
		{"encoded climb", rpc{"jsonrpc": "2.0", "id": 10, "method": "textDocument/hover", "params": rpc{"textDocument": rpc{"uri": "file:///workspace/%2e%2e/etc/passwd"}}}},
		{"encoded slash", rpc{"jsonrpc": "2.0", "id": 11, "method": "textDocument/hover", "params": rpc{"textDocument": rpc{"uri": "file:///workspace/a%2f..%2f..%2fb"}}}},
		{"an answer to nothing", rpc{"jsonrpc": "2.0", "id": 12, "result": rpc{}}},
	}
	for _, c := range cases {
		send(t, s, &seq, c.msg)
	}
	// Only the five requests with an id that the policy refuses are answered with
	// an error; the change of configuration was refused as a method, and the
	// unmatched answer is dropped silently. Then a legal echo proves the session
	// lives and nothing else reached the server.
	send(t, s, &seq, rpc{"jsonrpc": "2.0", "id": 99, "method": "textDocument/hover", "params": rpc{"ok": true}})
	errs := map[string]bool{}
	for {
		m := next(t, s)
		if string(m["id"]) == "99" {
			break
		}
		if m["error"] == nil {
			t.Fatalf("a refused message produced %s", m["_raw"])
		}
		errs[string(m["id"])] = true
	}
	for _, id := range []string{"5", "6", "7", "8", "9", "10", "11"} {
		if !errs[id] {
			t.Errorf("request %s was not answered with an error", id)
		}
	}
}

// Verifies: FN-033.
func TestLanguageSessionMapsDocumentsAndKeepsOrder_FN_033(t *testing.T) {
	s := languageFixture(t, NoSandbox{}, loanSource(), nil)
	seq := uint64(1)
	uri := s.RootURI() + "src/lib.rs"
	send(t, s, &seq, rpc{"jsonrpc": "2.0", "method": "textDocument/didOpen", "params": rpc{
		"textDocument": rpc{"uri": uri, "languageId": "rust", "version": 1, "text": "// file:///workspace/ stays text\n"},
	}})
	m := next(t, s)
	if string(m["method"]) != `"test/opened"` {
		t.Fatalf("got %s", m["_raw"])
	}
	if !bytes.Contains(m["_raw"], []byte(uri)) || !bytes.Contains(m["_raw"], []byte("file:///workspace/ stays text")) {
		t.Errorf("the document or its text changed on the way back: %s", m["_raw"])
	}
	if err := s.Send(5, []byte(`{"jsonrpc":"2.0","method":"textDocument/hover"}`)); err == nil {
		t.Error("a message with a gap in its sequence was accepted")
	}
	if err := s.Send(1, []byte(`{"jsonrpc":"2.0","method":"textDocument/hover"}`)); err == nil {
		t.Error("a repeated sequence number was accepted")
	}
	if err := s.Send(seq, bytes.Repeat([]byte("x"), 1<<17)); err == nil {
		t.Error("a message over lsp.messageBytes was accepted")
	}
}

// Verifies: FN-033.
func TestLanguageSessionScrubsSettingsFilesOfTheSource_FN_033(t *testing.T) {
	src := loanSource()
	fsys := src.Files.(fstest.MapFS)
	fsys["rust-analyzer.toml"] = &fstest.MapFile{Data: []byte("[cargo.buildScripts]\nenable = true\n")}
	fsys[".cargo/config.toml"] = &fstest.MapFile{Data: []byte("[build]\nrustc-wrapper = \"/bin/evil\"\n")}
	s := languageFixture(t, NoSandbox{}, src, nil)
	seq := uint64(1)
	send(t, s, &seq, rpc{"jsonrpc": "2.0", "id": 3, "method": "textDocument/documentSymbol"})
	m := next(t, s)
	var found []string
	if err := json.Unmarshal(m["result"], &found); err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || found[0] != "/.cargo" {
		t.Errorf("settings files left in the workspace = %v; want only the builder's own /.cargo", found)
	}
}

// Verifies: FN-033.
func TestLanguageSessionEnds_FN_033(t *testing.T) {
	t.Run("the server exits", func(t *testing.T) {
		s := languageFixture(t, NoSandbox{}, loanSource(), nil)
		seq := uint64(1)
		send(t, s, &seq, rpc{"jsonrpc": "2.0", "method": "exit"})
		if end := s.Wait(); end.Reason != "exited" {
			t.Errorf("end = %+v; want exited", end)
		}
	})
	t.Run("the server fails", func(t *testing.T) {
		s := languageFixture(t, NoSandbox{}, loanSource(), nil)
		seq := uint64(1)
		send(t, s, &seq, rpc{"jsonrpc": "2.0", "method": "textDocument/rename", "id": 1})
		end := s.Wait()
		if end.Reason != "failed" || !strings.Contains(end.Detail, "status 3") || strings.Contains(end.Detail, "plux-build-") {
			t.Errorf("end = %+v; want failed with status 3 and no host path", end)
		}
	})
	t.Run("the client closes", func(t *testing.T) {
		s := languageFixture(t, NoSandbox{}, loanSource(), nil)
		s.Close()
		if end := s.Wait(); end.Reason != "client" {
			t.Errorf("end = %+v; want client", end)
		}
		if err := s.Send(1, []byte(`{"jsonrpc":"2.0","method":"textDocument/hover"}`)); err == nil {
			t.Error("a message was accepted after the end")
		}
	})
	t.Run("the session time passes", func(t *testing.T) {
		s := languageFixture(t, NoSandbox{}, loanSource(), func(o *LanguageOptions) { o.Wall = 500 * time.Millisecond })
		if end := s.Wait(); end.Reason != "time" {
			t.Errorf("end = %+v; want time", end)
		}
	})
	t.Run("the server sends an oversize message", func(t *testing.T) {
		s := languageFixture(t, NoSandbox{}, loanSource(), nil)
		seq := uint64(1)
		send(t, s, &seq, rpc{"jsonrpc": "2.0", "method": "textDocument/references"})
		if end := s.Wait(); end.Reason != "failed" || !strings.Contains(end.Detail, "lsp.messageBytes") {
			t.Errorf("end = %+v; want failed over lsp.messageBytes", end)
		}
	})
	t.Run("the sandbox is refused", func(t *testing.T) {
		s := languageFixture(t, unavailableSandbox{}, loanSource(), nil)
		end := s.Wait()
		if end.Reason != "failed" || !strings.Contains(end.Detail, "sandbox") {
			t.Errorf("end = %+v; want failed for the sandbox", end)
		}
	})
}

// Verifies: FN-033, FN-016.
func TestLanguageSessionRunsInTheNamespaceSandbox_FN_033(t *testing.T) {
	requireNamespaces(t)
	s := languageFixture(t, LinuxSandbox{}, loanSource(), func(o *LanguageOptions) { o.Memory = 4 << 30 })
	seq := uint64(1)
	send(t, s, &seq, rpc{"jsonrpc": "2.0", "id": 2, "method": "textDocument/hover", "params": rpc{"hi": 1}})
	if m := next(t, s); string(m["id"]) != "2" {
		t.Fatalf("got %s", m["_raw"])
	}
	send(t, s, &seq, rpc{"jsonrpc": "2.0", "method": "exit"})
	if end := s.Wait(); end.Reason != "exited" {
		t.Errorf("end = %+v; want exited", end)
	}
}

// Verifies: FN-033.
func TestOpenLanguageRequiresItsLimits_FN_033(t *testing.T) {
	f := newFixture(t)
	for name, o := range map[string]LanguageOptions{
		"no server":        {Wall: time.Second, MessageBytes: 1},
		"no session time":  {Server: "x", MessageBytes: 1},
		"no message limit": {Server: "x", Wall: time.Second},
	} {
		if _, err := f.builder().OpenLanguage(context.Background(), loanSource(), o); err == nil {
			t.Errorf("%s: OpenLanguage accepted the options", name)
		}
	}
	_, err := f.builder().OpenLanguage(context.Background(), loanSource(), LanguageOptions{Server: "/nonexistent/rust-analyzer", Wall: time.Second, MessageBytes: 1})
	if err == nil || errors.Is(err, ErrSandboxUnavailable) {
		t.Errorf("a missing language server gave %v", err)
	}
}
