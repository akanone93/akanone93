// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package renderworker

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/renderpb/renderpbconnect"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
)

// MaxRequestBytes is the largest request the worker reads: both projects at
// the default expanded size of a project, the function catalogue
// (CatalogueAllowance) and room for the framing. An
// organisation that raised archive.expandedBytes above the default has its
// render refused with a clear error rather than held in this process's memory.
func MaxRequestBytes() int {
	def, _ := limits.Lookup(limits.ArchiveExpandedBytes)
	return int(min(2*def.Default+1<<20+CatalogueAllowance, 1<<31-1))
}

// drainTimeout is how long a stopping worker lets a running render finish.
const drainTimeout = 10 * time.Minute

// Serve answers on addr with the mutual-TLS configuration of the internal
// roles (httpx.InternalServerTLS: a client certificate signed by the internal
// CA is verified before any request is read, SEC-043) until ctx ends, then
// drains the renders that run.
func Serve(ctx context.Context, addr string, tlsConfig *tls.Config, h *Handler, log *slog.Logger) error {
	mux := http.NewServeMux()
	path, handler := renderpbconnect.NewRenderWorkerServiceHandler(h, connect.WithReadMaxBytes(MaxRequestBytes()))
	mux.Handle(path, handler)
	mux.HandleFunc("/livez", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	srv := &http.Server{
		Handler: mux, TLSConfig: tlsConfig, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 2 * time.Minute,
		BaseContext: func(net.Listener) context.Context { return context.WithoutCancel(ctx) }, // a stop drains the renders that run
	}
	lis, err := (&net.ListenConfig{}).Listen(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("renderworker: listen: %w", err)
	}
	failed := make(chan error, 1)
	go func() { failed <- srv.ServeTLS(lis, "", "") }()
	log.InfoContext(ctx, "the render worker answers", slog.String("address", lis.Addr().String()))
	select {
	case err := <-failed:
		return fmt.Errorf("renderworker: serve: %w", err)
	case <-ctx.Done():
	}
	drain, cancel := context.WithTimeout(context.WithoutCancel(ctx), drainTimeout)
	defer cancel()
	if err := srv.Shutdown(drain); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("renderworker: stop: %w", err)
	}
	return nil
}
