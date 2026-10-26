// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	"bytes"
	"encoding/json"
	"net/url"
	"strings"
	"sync"
)

// JSON-RPC error codes the policy answers with.
const (
	codeMethodNotFound = -32601
	codeInvalidRequest = -32600
)

// clientMethods are the requests and notifications a client may send: what an
// editor needs for diagnostics, completion, navigation, formatting and
// semantic highlighting (FN-033). Everything else is refused, notably
// `workspace/executeCommand` and the `rust-analyzer/*` extensions that run
// cargo, tests or macros, and `workspace/didChangeConfiguration`, which could
// switch build scripts or procedural macros on.
var clientMethods = map[string]bool{
	"initialize": true, "initialized": true, //nolint:misspell // LSP method names.
	"shutdown": true, "exit": true, "$/cancelRequest": true,
	"textDocument/didOpen": true, "textDocument/didChange": true, "textDocument/didClose": true, "textDocument/didSave": true,
	"textDocument/completion": true, "completionItem/resolve": true, "textDocument/hover": true,
	"textDocument/signatureHelp": true, "textDocument/definition": true, "textDocument/declaration": true,
	"textDocument/typeDefinition": true, "textDocument/implementation": true, "textDocument/references": true,
	"textDocument/documentHighlight": true, "textDocument/documentSymbol": true, "workspace/symbol": true,
	"textDocument/codeAction": true, "codeAction/resolve": true, "textDocument/formatting": true,
	"textDocument/rangeFormatting": true, "textDocument/onTypeFormatting": true, "textDocument/rename": true,
	"textDocument/prepareRename": true, "textDocument/foldingRange": true, "textDocument/selectionRange": true,
	"textDocument/semanticTokens/full": true, "textDocument/semanticTokens/range": true,
	"textDocument/inlayHint": true, "inlayHint/resolve": true,
}

// uriKeys are the JSON keys whose string values name a document or a root.
var uriKeys = map[string]bool{"uri": true, "targetUri": true, "rootUri": true, "oldUri": true, "newUri": true}

// forcedSettings are the language server's settings, whatever the client asks
// for: build scripts and procedural macros off (they run user code), no
// check-on-save (cargo check runs build scripts), no network for cargo, no
// code lenses or runnables that would start tests (FN-033, ADR-0099).
const forcedSettings = `{
  "cargo": {"buildScripts": {"enable": false}, "extraEnv": {"CARGO_NET_OFFLINE": "true"}, "autoreload": false},
  "procMacro": {"enable": false, "attributes": {"enable": false}},
  "checkOnSave": false,
  "cachePriming": {"enable": false},
  "lens": {"enable": false},
  "files": {"watcher": "client"},
  "diagnostics": {"enable": true}
}`

// languagePolicy filters the messages between a client and a language server
// and rewrites paths between the virtual and the real workspace. It is safe
// for concurrent use.
type languagePolicy struct {
	realRoot string // the sandbox's workspace directory
	realURI  string // file://<realRoot>/
	cropRoot string // file:///workspace/functions/<name>/
	allowed  string // file:///workspace/

	mu      sync.Mutex      // guards pending
	pending map[string]bool // ids of server requests the client may answer
}

// newLanguagePolicy returns the policy of a session whose workspace is at
// realRoot and whose crate is at the virtual cropRoot.
func newLanguagePolicy(realRoot, cropRoot string) *languagePolicy {
	return &languagePolicy{
		realRoot: realRoot, realURI: "file://" + realRoot + "/", cropRoot: cropRoot,
		allowed: "file://" + virtualRoot + "/", pending: map[string]bool{},
	}
}

// message is the part of a JSON-RPC message the policy reads.
type message struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

// fromClient decides what to do with one client message: forward is the
// message for the language server (nil to drop), reply an answer for the
// client (nil for none).
func (p *languagePolicy) fromClient(raw []byte) (forward, reply []byte) {
	var m message
	if err := json.Unmarshal(raw, &m); err != nil || len(bytes.TrimSpace(raw)) == 0 || raw[0] != '{' {
		return nil, errorReply(nil, codeInvalidRequest, "the message is not a JSON-RPC object")
	}
	if m.Method == "" {
		// A response is allowed only to a request the server made.
		if len(m.ID) > 0 && p.takePending(string(m.ID)) {
			return raw, nil
		}
		return nil, nil
	}
	if !clientMethods[m.Method] {
		return nil, refuse(m.ID, codeMethodNotFound, "the method "+m.Method+" is not available in the Rust editor")
	}
	params, ok := p.rewriteParams(m.Method, m.Params)
	if !ok {
		return nil, refuse(m.ID, codeInvalidRequest, "a document is outside the workspace")
	}
	out := map[string]json.RawMessage{"jsonrpc": json.RawMessage(`"2.0"`), "method": mustJSON(m.Method)}
	if len(m.ID) > 0 {
		out["id"] = m.ID
	}
	if params != nil {
		out["params"] = params
	}
	enc, err := json.Marshal(out)
	if err != nil {
		return nil, refuse(m.ID, codeInvalidRequest, "the message cannot be encoded")
	}
	return enc, nil
}

// rewriteParams checks every document URI of the parameters and maps it to the
// real workspace; for the handshake request it also replaces the roots and the settings
// with the policy's own. ok is false when a URI is not below the workspace.
func (p *languagePolicy) rewriteParams(method string, raw json.RawMessage) (json.RawMessage, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, true
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, false
	}
	if method == "initialize" { //nolint:misspell // the LSP method name.
		obj, isObj := v.(map[string]any)
		if !isObj {
			return nil, false
		}
		var settings any
		_ = json.Unmarshal([]byte(forcedSettings), &settings)
		obj["initializationOptions"] = settings
		obj["rootUri"] = strings.TrimSuffix(p.realURI, "/")
		obj["rootPath"] = nil
		obj["processId"] = nil
		obj["workspaceFolders"] = []any{map[string]any{"uri": strings.TrimSuffix(p.realURI, "/"), "name": "workspace"}}
		v = obj
	}
	ok := true
	if method != "initialize" { //nolint:misspell // the LSP method name.
		v = p.mapURIs(v, &ok)
	}
	if !ok {
		return nil, false
	}
	out, err := json.Marshal(v)
	if err != nil {
		return nil, false
	}
	return out, true
}

// mapURIs walks a decoded value and maps the string values of URI keys from the
// virtual to the real workspace. A URI outside the workspace clears ok.
func (p *languagePolicy) mapURIs(v any, ok *bool) any {
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			if s, isStr := child.(string); isStr && uriKeys[k] {
				mapped, good := p.toReal(s)
				if !good {
					*ok = false
					return t
				}
				t[k] = mapped
				continue
			}
			t[k] = p.mapURIs(child, ok)
		}
		return t
	case []any:
		for i, child := range t {
			t[i] = p.mapURIs(child, ok)
		}
		return t
	default:
		return v
	}
}

// toReal maps a virtual document URI to the real one. It refuses a URI that is
// not below the virtual workspace, that climbs out of it or hides a climb in a
// percent-encoding.
func (p *languagePolicy) toReal(uri string) (string, bool) {
	rest, found := strings.CutPrefix(uri, p.allowed)
	if !found {
		return "", false
	}
	decoded, err := url.PathUnescape(rest)
	if err != nil || strings.ContainsAny(decoded, "\x00\\") {
		return "", false
	}
	for seg := range strings.SplitSeq(decoded, "/") {
		if seg == ".." || seg == "." {
			return "", false
		}
	}
	if strings.Contains(rest, "%2f") || strings.Contains(rest, "%2F") {
		return "", false
	}
	return p.realURI + rest, true
}

// fromServer decides what to do with one message of the language server:
// forward is the message for the client with host paths replaced (nil to
// drop), reply an answer for the language server itself. The server's
// `workspace/configuration` requests are answered here with the forced
// settings, so a client never supplies them.
func (p *languagePolicy) fromServer(raw []byte) (forward, reply []byte) {
	var m message
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, nil
	}
	if m.Method == "workspace/configuration" && len(m.ID) > 0 {
		var params struct {
			Items []json.RawMessage `json:"items"`
		}
		_ = json.Unmarshal(m.Params, &params)
		results := make([]json.RawMessage, max(len(params.Items), 1))
		for i := range results {
			results[i] = json.RawMessage(forcedSettings)
		}
		body, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": m.ID, "result": results})
		if err != nil {
			return nil, nil
		}
		return nil, body
	}
	if m.Method != "" && len(m.ID) > 0 {
		p.mu.Lock()
		p.pending[string(m.ID)] = true
		p.mu.Unlock()
	}
	return bytes.ReplaceAll(raw, []byte(p.realRoot), []byte(virtualRoot)), nil
}

// takePending reports whether id names a server request still unanswered, and
// marks it answered.
func (p *languagePolicy) takePending(id string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.pending[id] {
		return false
	}
	delete(p.pending, id)
	return true
}

// refuse answers a refused request; a notification, which has no id, is
// dropped without an answer, as the protocol asks.
func refuse(id json.RawMessage, code int, text string) []byte {
	if len(id) == 0 {
		return nil
	}
	return errorReply(id, code, text)
}

// errorReply is a JSON-RPC error response; a nil id answers a message that had
// none, with a null id as the protocol asks.
func errorReply(id json.RawMessage, code int, text string) []byte {
	if len(id) == 0 {
		id = json.RawMessage("null")
	}
	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": id, "error": map[string]any{"code": code, "message": text},
	})
	if err != nil {
		return nil
	}
	return body
}

// mustJSON encodes a string.
func mustJSON(s string) json.RawMessage {
	b, _ := json.Marshal(s)
	return b
}
