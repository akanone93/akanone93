// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

package auditexport

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
)

// maxResponseBytes bounds how much of a receiver's answer is read. The
// status is what matters; a receiver cannot make the sink read more.
const maxResponseBytes = 64 << 10

// postJSON posts a JSON body with the given extra headers and returns an
// error unless the receiver answers with a success status. The response
// body is drained and closed.
func postJSON(ctx context.Context, client *http.Client, endpoint string, headers http.Header, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("auditexport: build the request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for name, values := range headers {
		req.Header[name] = values
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("auditexport: post to the destination: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBytes))
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("auditexport: the destination answered %d", resp.StatusCode)
	}
	return nil
}
