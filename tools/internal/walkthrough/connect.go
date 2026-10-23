// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package walkthrough

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// maxReply bounds what the client reads of one answer.
const maxReply = 4 << 20

// Caller makes unary ConnectRPC calls with JSON messages: the few calls the
// walkthrough makes that the CLI has no command for (identity providers, SCIM
// tokens, audit export destinations, the sign-in).
type Caller interface {
	// Call posts req as the JSON message of the procedure, such as
	// "/plux.v1.AuditService/CreateExportDestination", and decodes the
	// answer into resp. A Connect error is returned with its code and
	// message.
	Call(ctx context.Context, procedure string, req, resp any) error
}

// ConnectClient is a [Caller] over HTTP, speaking the Connect protocol's
// unary JSON form.
type ConnectClient struct {
	BaseURL string
	// Token is the bearer credential; empty for the unauthenticated
	// sign-in calls.
	Token string
	HTTP  *http.Client
	// PublicURL is the server's public base URL, which DPoP proofs name
	// when BaseURL is a port-forward; empty means BaseURL.
	PublicURL string
	// Actor and StepUp complete a step-up the server asks for (PLX-6200):
	// StepUp confirms the ceremony at a URL as Actor. Without StepUp a
	// step-up refusal is returned as it is.
	Actor  string
	StepUp StepUpFunc

	stepUp stepUpper
}

// proofBase is the base URL DPoP proofs name.
func (c *ConnectClient) proofBase() string {
	return strings.TrimSuffix(firstNonEmpty(c.PublicURL, c.BaseURL), "/")
}

// client is the HTTP client calls use.
func (c *ConnectClient) client() *http.Client {
	if c.HTTP == nil {
		return http.DefaultClient
	}
	return c.HTTP
}

// Call implements [Caller]. A call the server refuses for want of a step-up
// is repeated once after the step-up, when StepUp is set.
func (c *ConnectClient) Call(ctx context.Context, procedure string, req, resp any) error {
	err := c.call(ctx, procedure, req, resp)
	if c.StepUp == nil || !needsStepUp(err) {
		return err
	}
	if err := c.stepUp.obtain(ctx, c); err != nil {
		return fmt.Errorf("walkthrough.ConnectClient.Call %s: %w", procedure, err)
	}
	return c.call(ctx, procedure, req, resp)
}

// call makes one call, with the step-up grant when one is held.
func (c *ConnectClient) call(ctx context.Context, procedure string, req, resp any) error {
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("walkthrough.ConnectClient.Call: %w", err)
	}
	hr, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimSuffix(c.BaseURL, "/")+procedure, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("walkthrough.ConnectClient.Call: %w", err)
	}
	hr.Header.Set("Content-Type", "application/json")
	hr.Header.Set("Connect-Protocol-Version", "1")
	if c.Token != "" {
		hr.Header.Set("Authorization", "Bearer "+c.Token)
		if err := c.stepUp.attach(hr.Header, c.Token, c.proofBase()+procedure); err != nil {
			return fmt.Errorf("walkthrough.ConnectClient.Call: %w", err)
		}
	}
	res, err := c.client().Do(hr)
	if err != nil {
		return fmt.Errorf("walkthrough.ConnectClient.Call %s: %w", procedure, err)
	}
	defer func() { _ = res.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(res.Body, maxReply))
	if err != nil {
		return fmt.Errorf("walkthrough.ConnectClient.Call %s: %w", procedure, err)
	}
	if res.StatusCode != http.StatusOK {
		return connectError(procedure, res.StatusCode, data)
	}
	if resp == nil {
		return nil
	}
	if err := json.Unmarshal(data, resp); err != nil {
		return fmt.Errorf("walkthrough.ConnectClient.Call %s: %w", procedure, err)
	}
	return nil
}

// connectError renders a failed call from the Connect error body, which has a
// code and a message.
func connectError(procedure string, status int, data []byte) error {
	var e struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if json.Unmarshal(data, &e) != nil || e.Code == "" {
		return fmt.Errorf("walkthrough.ConnectClient.Call %s: HTTP %d", procedure, status)
	}
	return fmt.Errorf("walkthrough.ConnectClient.Call %s: %s: %s", procedure, e.Code, e.Message)
}

// HTTPSCIMClient is a [SCIMClient] over HTTP.
type HTTPSCIMClient struct {
	BaseURL string
	HTTP    *http.Client
}

// Post implements [SCIMClient] with the SCIM media type of RFC 7644.
func (c *HTTPSCIMClient) Post(ctx context.Context, token, path string, body any) (map[string]any, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("walkthrough.HTTPSCIMClient.Post: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimSuffix(c.BaseURL, "/")+path, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("walkthrough.HTTPSCIMClient.Post: %w", err)
	}
	req.Header.Set("Content-Type", "application/scim+json")
	req.Header.Set("Authorization", "Bearer "+token)
	client := c.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("walkthrough.HTTPSCIMClient.Post %s: %w", path, err)
	}
	defer func() { _ = res.Body.Close() }()
	reply, err := io.ReadAll(io.LimitReader(res.Body, maxReply))
	if err != nil {
		return nil, fmt.Errorf("walkthrough.HTTPSCIMClient.Post %s: %w", path, err)
	}
	if res.StatusCode != http.StatusCreated && res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("walkthrough.HTTPSCIMClient.Post %s: HTTP %d", path, res.StatusCode)
	}
	out := map[string]any{}
	if err := json.Unmarshal(reply, &out); err != nil {
		return nil, fmt.Errorf("walkthrough.HTTPSCIMClient.Post %s: %w", path, err)
	}
	return out, nil
}
