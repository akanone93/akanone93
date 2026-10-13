// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package fnrunner

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"testing"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/fnpb"
)

// Verifies: FN-009.
// A module that fails any check is refused with PLX-7019, and nothing is
// compiled or cached.
func TestRunnerRefusesUnverifiedModules_FN_009(t *testing.T) {
	t.Parallel()
	good := probe(t, "probe.wasm")
	want := digest(good)
	other := probe(t, "probe-device.wasm")
	stranger := newSigner(t, "k1") // same key ID, another key
	unknown := newSigner(t, "unlisted")

	tampered := append(append([]byte(nil), good...), 0)
	badSignature := newSigner(t, "k1").artifact(good)
	cases := []struct {
		name string
		make func(h *harness) *fnpb.GetModuleResponse
	}{
		{"module tampered after signing", func(h *harness) *fnpb.GetModuleResponse {
			r := h.signer.artifact(good)
			r.Module = tampered
			return r
		}},
		{"another module under the requested digest", func(h *harness) *fnpb.GetModuleResponse {
			return h.signer.artifact(other)
		}},
		{"signed by a different key under a trusted ID", func(*harness) *fnpb.GetModuleResponse { return stranger.artifact(good) }},
		{"key ID the runner does not trust", func(*harness) *fnpb.GetModuleResponse { return unknown.artifact(good) }},
		{"no key ID", func(h *harness) *fnpb.GetModuleResponse {
			r := h.signer.artifact(good)
			r.KeyId = ""
			return r
		}},
		{"signature of another statement", func(*harness) *fnpb.GetModuleResponse { return badSignature }},
		{"truncated signature", func(h *harness) *fnpb.GetModuleResponse {
			r := h.signer.artifact(good)
			r.Signature = r.Signature[:10]
			return r
		}},
		{"no signature", func(h *harness) *fnpb.GetModuleResponse {
			r := h.signer.artifact(good)
			r.Signature = nil
			return r
		}},
		{"statement names another module", func(h *harness) *fnpb.GetModuleResponse {
			return h.signer.sign(good, []byte(fmt.Sprintf(`{"moduleSha256":%q}`, digest(other))))
		}},
		{"statement is not JSON", func(h *harness) *fnpb.GetModuleResponse { return h.signer.sign(good, []byte("not json")) }},
		{"statement is not an object", func(h *harness) *fnpb.GetModuleResponse { return h.signer.sign(good, []byte(`["x"]`)) }},
		{"statement does not name a module", func(h *harness) *fnpb.GetModuleResponse {
			return h.signer.sign(good, []byte(`{"abiVersion":1}`))
		}},
		{"statement field in another case", func(h *harness) *fnpb.GetModuleResponse {
			return h.signer.sign(good, []byte(fmt.Sprintf(`{"modulesha256":%q}`, want)))
		}},
		{"statement digest is upper case", func(h *harness) *fnpb.GetModuleResponse {
			return h.signer.sign(good, []byte(fmt.Sprintf(`{"moduleSha256":%q}`, upper(want))))
		}},
		{"statement digest is not a string", func(h *harness) *fnpb.GetModuleResponse {
			return h.signer.sign(good, []byte(`{"moduleSha256":7}`))
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, harnessOptions{})
			h.api.modules[want] = tc.make(h)

			_, err := h.call(0, h.request("{}"))
			if err == nil {
				t.Fatal("Invoke accepted the module")
			}
			if got := plxCode(t, err); got != "PLX-7019" {
				t.Errorf("code = %s, want PLX-7019 (%v)", got, err)
			}
			if connect.CodeOf(err) != connect.CodeDataLoss {
				t.Errorf("Connect code = %v", connect.CodeOf(err))
			}
			if n, _ := h.runner.cache.len(); n != 0 {
				t.Errorf("%d modules cached after a refusal", n)
			}
			if got := h.reg.counter(t, "plux_function_module_cache_total", cacheRefused); got != 1 {
				t.Errorf("refused counter = %v, want 1", got)
			}
			if got := h.reg.counter(t, "plux_function_invocations_total", "function_artifact_signature_invalid"); got != 1 {
				t.Errorf("invocation counter = %v, want 1", got)
			}
		})
	}
}

func upper(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'a' && c <= 'f' {
			b[i] = c - 32
		}
	}
	return string(b)
}

// Verifies: FN-009.
// The signature is checked before the module is compiled: a verified module
// that is not a function module is refused as invalid (PLX-7018), which
// shows that compilation happened only after verification.
func TestRunnerCompilesOnlyAfterVerification_FN_009(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{})
	garbage := []byte("\x00asm-not-really")
	h.api.modules[digest(garbage)] = h.signer.artifact(garbage)
	msg := h.request("{}")
	msg.ModuleSha256 = digest(garbage)
	_, err := h.call(0, msg)
	if got := plxCode(t, err); got != "PLX-7018" {
		t.Errorf("code = %s, want PLX-7018 (%v)", got, err)
	}

	unsigned := []byte("\x00asm-unsigned")
	h.api.modules[digest(unsigned)] = h.signer.artifact([]byte("something else"))
	msg.ModuleSha256 = digest(unsigned)
	_, err = h.call(0, msg)
	if got := plxCode(t, err); got != "PLX-7019" {
		t.Errorf("code = %s, want PLX-7019 for an unverified garbage module (%v)", got, err)
	}
}

// Verifies: FN-009.
// Keys come from configuration only: a response that brings its own valid
// key cannot make the runner trust it, and New refuses a runner without a
// key.
func TestRunnerTrustsOnlyConfiguredKeys_FN_009(t *testing.T) {
	t.Parallel()
	h := newHarness(t, harnessOptions{})
	_, fresh, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	s := signer{id: "k1", key: fresh}
	good := probe(t, "probe.wasm")
	h.api.modules[digest(good)] = s.artifact(good)
	if _, err := h.call(0, h.request("{}")); plxCode(t, err) != "PLX-7019" {
		t.Errorf("an untrusted signer was accepted: %v", err)
	}

	for name, mutate := range map[string]func(*Options){
		"no keys":       func(o *Options) { o.Keys = nil },
		"short key":     func(o *Options) { o.Keys = map[string]ed25519.PublicKey{"k": {1, 2}} },
		"no host":       func(o *Options) { o.Host = nil },
		"no bounds":     func(o *Options) { o.MaxConcurrent = 0 },
		"no memory":     func(o *Options) { o.MaxMemoryBytes = 0 },
		"no clock":      func(o *Options) { o.Clock = nil },
		"no response":   func(o *Options) { o.HTTPResponseBytes = 0 },
		"no egress":     func(o *Options) { o.Egress = nil },
		"no randomness": func(o *Options) { o.Random = nil },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			opts := Options{
				Keys: map[string]ed25519.PublicKey{"k": make(ed25519.PublicKey, ed25519.PublicKeySize)},
				Host: h.runner.opts.Host, Egress: h.runner.opts.Egress, Clock: h.runner.opts.Clock, Random: zeroReader{},
				MaxMemoryBytes: 1 << 20, MaxModules: 1, MaxModuleBytes: 1, MaxConcurrent: 1, HTTPResponseBytes: 1,
			}
			mutate(&opts)
			if r, err := New(context.Background(), opts); err == nil {
				_ = r.Close(context.Background())
				t.Error("New accepted the options")
			}
		})
	}
}
