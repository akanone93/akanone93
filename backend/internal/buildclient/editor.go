// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package buildclient

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/fnpb"
	"github.com/nightCode42/plux3/backend/internal/function"
)

var _ function.Editor = (*Client)(nil)

// source reads the stored archive of an editor input and checks its hash.
func (c *Client) source(ctx context.Context, in function.EditorInput) ([]byte, error) {
	source, _, err := c.objects.Get(ctx, in.SourceObjectKey)
	if err != nil {
		return nil, fmt.Errorf("buildclient: read the source: %w", err)
	}
	if sum := sha256.Sum256(source); !strings.EqualFold(hex.EncodeToString(sum[:]), in.SourceSHA256) {
		return nil, errors.New("buildclient: the stored source does not match its recorded SHA-256")
	}
	return source, nil
}

// OpenLanguage implements function.Editor: it opens the builder's server
// stream and passes its events on until the session ends (FN-033).
func (c *Client) OpenLanguage(ctx context.Context, in function.EditorInput, send func(function.EditorEvent) error) error {
	source, err := c.source(ctx, in)
	if err != nil {
		return err
	}
	stream, err := c.stream.LanguageOpen(ctx, connect.NewRequest(&fnpb.LanguageOpenRequest{
		Source: source, SourceSha256: in.SourceSHA256, Name: in.Name,
		SessionMinutes: in.SessionMinutes, MemoryBytes: uint64(max(in.MemoryBytes, 0)), //nolint:gosec // G115: non-negative.
	}))
	if err != nil {
		return fmt.Errorf("buildclient.OpenLanguage: %w", err)
	}
	defer func() { _ = stream.Close() }()
	for stream.Receive() {
		var ev function.EditorEvent
		switch e := stream.Msg().GetEvent().(type) {
		case *fnpb.LanguageEvent_Opened:
			ev.Opened = &function.EditorOpened{SessionID: e.Opened.GetSessionId(), RootURI: e.Opened.GetRootUri(), Seconds: in.SessionMinutes * 60}
		case *fnpb.LanguageEvent_Message:
			ev.Message = e.Message
		case *fnpb.LanguageEvent_Closed:
			ev.Closed = &function.EditorClosed{Reason: e.Closed.GetReason(), Detail: e.Closed.GetDetail()}
		default:
			continue
		}
		if err := send(ev); err != nil {
			return err
		}
	}
	if err := stream.Err(); err != nil && ctx.Err() == nil {
		return fmt.Errorf("buildclient.OpenLanguage: %w", err)
	}
	return nil
}

// SendLanguage implements function.Editor.
func (c *Client) SendLanguage(ctx context.Context, sessionID string, sequence uint64, message []byte) error {
	if _, err := c.rpc.LanguageSend(ctx, connect.NewRequest(&fnpb.LanguageSendRequest{SessionId: sessionID, Sequence: sequence, Message: message})); err != nil {
		return fmt.Errorf("buildclient.SendLanguage: %w", err)
	}
	return nil
}

// Test implements function.Editor (FN-034).
func (c *Client) Test(ctx context.Context, in function.EditorInput) (function.EditorTests, error) {
	source, err := c.source(ctx, in)
	if err != nil {
		return function.EditorTests{}, err
	}
	if in.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, in.Timeout*BuildSteps+transferSlack)
		defer cancel()
	}
	resp, err := c.rpc.Test(ctx, connect.NewRequest(&fnpb.TestRequest{Build: &fnpb.BuildRequest{
		BuildId: "editor-test", Source: source, SourceSha256: in.SourceSHA256, Name: in.Name,
		Limits: &fnpb.BuildLimits{TimeoutMs: uint32(min(in.Timeout.Milliseconds(), 1<<32-1)), MemoryBytes: uint64(max(in.MemoryBytes, 0))}, //nolint:gosec // G115: bounded and non-negative.
	}}))
	if err != nil {
		return function.EditorTests{}, fmt.Errorf("buildclient.Test: %w", err)
	}
	var out function.EditorTests
	if raw := resp.Msg.GetDiagnosticsJson(); len(raw) > 0 {
		if err := json.Unmarshal(raw, &out.Diagnostics); err != nil {
			return function.EditorTests{}, fmt.Errorf("buildclient.Test: read the diagnostics: %w", err)
		}
	}
	for _, t := range resp.Msg.GetTestResults() {
		out.Results = append(out.Results, function.TestResult{Name: t.GetName(), Passed: t.GetPassed(), Message: t.GetMessage()})
	}
	return out, nil
}
