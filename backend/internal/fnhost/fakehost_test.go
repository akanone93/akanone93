// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package fnhost_test

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/nightCode42/plux3/backend/internal/fnhost"
)

// fakeHost is the Host of the conformance vectors. It is safe for
// concurrent use.
type fakeHost struct {
	mu           sync.Mutex
	kv           map[string][]byte
	secrets      map[string]string
	http         map[string]fnhost.HTTPResponse
	invoke       map[string]fnhost.Outcome
	fail         map[string]error
	calls        int
	invokeDepths []int
}

func newFakeHost() *fakeHost {
	return &fakeHost{
		kv: map[string][]byte{}, secrets: map[string]string{},
		http: map[string]fnhost.HTTPResponse{}, invoke: map[string]fnhost.Outcome{},
		fail: map[string]error{},
	}
}

// enter counts a call and returns the failure configured for the capability.
// Callers hold no lock.
func (h *fakeHost) enter(capability string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls++
	return h.fail[capability]
}

func (h *fakeHost) KVGet(_ context.Context, key string) ([]byte, error) {
	if err := h.enter("kvGet"); err != nil {
		return nil, err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	v, ok := h.kv[key]
	if !ok {
		return nil, fnhost.ErrNotFound
	}
	return v, nil
}

func (h *fakeHost) KVPut(_ context.Context, key string, value []byte) error {
	if err := h.enter("kvPut"); err != nil {
		return err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.kv[key] = value
	return nil
}

func (h *fakeHost) KVDelete(_ context.Context, key string) error {
	if err := h.enter("kvDelete"); err != nil {
		return err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.kv[key]; !ok {
		return fnhost.ErrNotFound
	}
	delete(h.kv, key)
	return nil
}

func (h *fakeHost) HTTP(_ context.Context, req fnhost.HTTPRequest) (fnhost.HTTPResponse, error) {
	if err := h.enter("http"); err != nil {
		return fnhost.HTTPResponse{}, err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	resp, ok := h.http[req.URL]
	if !ok {
		return fnhost.HTTPResponse{}, fnhost.ErrUpstream
	}
	return resp, nil
}

func (h *fakeHost) Secret(_ context.Context, name string) ([]byte, error) {
	if err := h.enter("secret"); err != nil {
		return nil, err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	v, ok := h.secrets[name]
	if !ok {
		return nil, fnhost.ErrNotFound
	}
	return []byte(v), nil
}

func (h *fakeHost) Invoke(_ context.Context, name string, _ []byte, depth int) (fnhost.Outcome, error) {
	if err := h.enter("invoke"); err != nil {
		return fnhost.Outcome{}, err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.invokeDepths = append(h.invokeDepths, depth)
	out, ok := h.invoke[name]
	if !ok {
		return fnhost.Outcome{}, fnhost.ErrNotFound
	}
	return out, nil
}

// failure names of the vectors, mapped to the sentinels.
func failure(name string) error {
	switch name {
	case "notFound":
		return fnhost.ErrNotFound
	case "limit":
		return fnhost.ErrLimit
	case "egressRefused":
		return fnhost.ErrEgressRefused
	case "upstream":
		return fnhost.ErrUpstream
	default:
		return fnhost.ErrUnavailable
	}
}

// outcomeSpec is the JSON form of a nested result in the vectors.
type outcomeSpec struct {
	Output   json.RawMessage       `json:"output"`
	Business *fnhost.BusinessError `json:"business"`
}
