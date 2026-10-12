// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Package statuslist keeps Google's Android attestation status list
// current for the Key Attestation verifier (SEC-002, ADR-0012).
//
// Two sources exist. By default the worker fetches the list from Google
// through the SSRF-guarded client (SEC-104), honouring the HTTP cache
// headers, and publishes it to the shared cache; every api replica reads
// it from there. An installation that cannot reach Google (DEP-040) names
// a file instead, which each replica reads itself and re-reads when it
// changes. A list that cannot be had at all is not an empty list: the
// verifier then fails with the provider's outage, which the
// attestationOutageGrace setting governs (SEC-009).
package statuslist

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nightCode42/plux3/backend/internal/attest/keyattest"
	"github.com/nightCode42/plux3/backend/internal/cache"
	"github.com/nightCode42/plux3/backend/internal/httpx"
)

const (
	// DefaultURL is where Google publishes the list.
	DefaultURL = "https://android.googleapis.com/attestation/status"

	// cacheKey holds the list as Google served it, for as long as it is
	// fresh.
	cacheKey = "attest:android:status-list"

	// memoTTL is how long a replica reuses the list it parsed, so that a
	// verification does not read the shared cache or the file.
	memoTTL = 30 * time.Second

	// defaultLifetime applies where the response says nothing about
	// caching; minLifetime and maxLifetime clamp what it says, so that a
	// server cannot make the list stale at once or keep it for ever.
	defaultLifetime = time.Hour
	minLifetime     = 5 * time.Minute
	maxLifetime     = 24 * time.Hour
)

// Options configures a List.
type Options struct {
	// URL is where the list is fetched; empty means DefaultURL. It is
	// ignored when File is set.
	URL string
	// File, when set, is a list the operator maintains: nothing is
	// fetched, and the file is re-read when it changes.
	File string
	// Client fetches the list; it must be the guarded client of package
	// httpx. Required without File.
	Client *http.Client
	// Cache is the shared cache the fetched list is published to.
	// Required without File.
	Cache cache.Cache
	// MaxBytes bounds the list, attest.statusListBytes (LIM-001).
	MaxBytes int64
	// Now is the clock; nil means time.Now.
	Now func() time.Time
}

// List supplies the current status list to keyattest.Verifier and, in the
// worker, refreshes it. It is safe for concurrent use.
type List struct {
	o Options

	mu    sync.Mutex
	list  *keyattest.Revocations
	until time.Time // the parsed list is reused until then
	stamp fileStamp // the file the parsed list came from

	refreshMu sync.Mutex
	etag      string
	raw       []byte    // the body etag belongs to
	refreshAt time.Time // the next time Refresh fetches, unless the cache lost the list
}

// fileStamp identifies a version of the operator's file.
type fileStamp struct {
	modified time.Time
	size     int64
}

// New returns a List.
func New(o Options) (*List, error) {
	if o.MaxBytes <= 0 {
		return nil, errors.New("statuslist: a size bound is required")
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.File == "" {
		if o.Client == nil || o.Cache == nil {
			return nil, errors.New("statuslist: fetching needs an HTTP client and the shared cache")
		}
		if o.URL == "" {
			o.URL = DefaultURL
		}
		if err := checkURL(o.URL); err != nil {
			return nil, err
		}
	}
	return &List{o: o}, nil
}

// FromFile reports whether the list comes from an operator's file, in
// which case there is nothing to refresh in the background.
func (l *List) FromFile() bool { return l.o.File != "" }

// Revocations returns the current list, or an error when there is none.
func (l *List) Revocations(ctx context.Context) (*keyattest.Revocations, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.o.Now()
	if l.list != nil && now.Before(l.until) {
		return l.list, nil
	}
	var err error
	if l.o.File != "" {
		err = l.reloadFile(now)
	} else {
		err = l.loadShared(ctx, now)
	}
	if err != nil {
		return nil, err
	}
	return l.list, nil
}

// reloadFile reads the operator's file when it changed. A file that has
// become unreadable or invalid keeps the last good list: the operator
// replaced it by hand, and a half-written file must not open a gap.
func (l *List) reloadFile(now time.Time) error {
	list, stamp, err := readFile(l.o.File, l.o.MaxBytes, l.stamp, l.list)
	if err != nil {
		if l.list != nil {
			l.until = now.Add(memoTTL)
			return nil
		}
		return err
	}
	l.list, l.stamp, l.until = list, stamp, now.Add(memoTTL)
	return nil
}

// readFile returns the list in the file, or have when the file is still
// the version stamp names.
func readFile(path string, limit int64, last fileStamp, have *keyattest.Revocations) (*keyattest.Revocations, fileStamp, error) {
	f, err := os.Open(path) //nolint:gosec // G304: the operator names the file in the configuration
	if err != nil {
		return nil, last, fmt.Errorf("statuslist: %w", err)
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return nil, last, fmt.Errorf("statuslist: %w", err)
	}
	stamp := fileStamp{modified: info.ModTime(), size: info.Size()}
	if have != nil && stamp == last {
		return have, stamp, nil
	}
	raw, err := httpx.ReadAll(f, limit)
	if err != nil {
		return nil, last, fmt.Errorf("statuslist: %w", err)
	}
	list, err := keyattest.ParseRevocations(raw)
	if err != nil {
		return nil, last, fmt.Errorf("statuslist: the file is not a status list: %w", err)
	}
	return list, stamp, nil
}

// loadShared reads the list the worker published.
func (l *List) loadShared(ctx context.Context, now time.Time) error {
	raw, ok, err := l.o.Cache.Get(ctx, cacheKey)
	if err != nil {
		return fmt.Errorf("statuslist: the shared cache: %w", err)
	}
	if !ok {
		return errors.New("statuslist: no fresh list has been fetched")
	}
	list, err := keyattest.ParseRevocations(raw)
	if err != nil {
		return fmt.Errorf("statuslist: the cached list is not valid: %w", err)
	}
	l.list, l.until = list, now.Add(memoTTL)
	return nil
}

// Refresh fetches the list and publishes it to the shared cache, for the
// worker's periodic job. It fetches again only when the list is half way
// through its freshness or the cache lost it, and asks the server whether
// it changed. It does nothing for a file.
func (l *List) Refresh(ctx context.Context) error {
	if l.o.File != "" {
		return nil
	}
	l.refreshMu.Lock()
	defer l.refreshMu.Unlock()
	now := l.o.Now()
	if now.Before(l.refreshAt) {
		if _, ok, err := l.o.Cache.Get(ctx, cacheKey); err == nil && ok {
			return nil
		}
	}
	lifetime, err := l.fetch(ctx)
	if err != nil {
		return err
	}
	if err := l.o.Cache.Set(ctx, cacheKey, l.raw, lifetime); err != nil {
		return fmt.Errorf("statuslist: publish: %w", err)
	}
	l.refreshAt = now.Add(lifetime / 2)
	return nil
}

// fetch asks for the list and leaves its body in l.raw. It returns how
// long the body stays fresh.
func (l *List) fetch(ctx context.Context) (time.Duration, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, l.o.URL, nil)
	if err != nil {
		return 0, fmt.Errorf("statuslist: %w", err)
	}
	if err := httpx.CheckURL(req.URL.Scheme, req.URL.Hostname()); err != nil {
		return 0, fmt.Errorf("statuslist: %w", err)
	}
	if l.etag != "" && l.raw != nil {
		req.Header.Set("If-None-Match", l.etag)
	}
	resp, err := l.o.Client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("statuslist: fetch: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusOK:
		raw, err := httpx.ReadAll(resp.Body, l.o.MaxBytes)
		if err != nil {
			return 0, fmt.Errorf("statuslist: fetch: %w", err)
		}
		if _, err := keyattest.ParseRevocations(raw); err != nil {
			return 0, fmt.Errorf("statuslist: the response is not a status list: %w", err)
		}
		l.raw, l.etag = raw, resp.Header.Get("ETag")
	case http.StatusNotModified:
		if l.raw == nil {
			return 0, errors.New("statuslist: fetch: unexpected 304 without a cached list")
		}
	default:
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return 0, fmt.Errorf("statuslist: fetch: the server answered %d", resp.StatusCode)
	}
	return freshFor(resp.Header, l.o.Now()), nil
}

// freshFor is how long a response may be reused: its Cache-Control
// max-age, else its Expires, else an hour, within the clamp.
func freshFor(h http.Header, now time.Time) time.Duration {
	d := defaultLifetime
	if age, ok := maxAge(h.Get("Cache-Control")); ok {
		d = age
	} else if exp, err := http.ParseTime(h.Get("Expires")); err == nil {
		d = exp.Sub(now)
	}
	return min(max(d, minLifetime), maxLifetime)
}

// maxAge reads the max-age directive of a Cache-Control header.
func maxAge(cacheControl string) (time.Duration, bool) {
	for _, directive := range strings.Split(cacheControl, ",") {
		name, value, _ := strings.Cut(strings.TrimSpace(directive), "=")
		if strings.EqualFold(name, "max-age") {
			n, err := strconv.ParseInt(strings.Trim(value, `"`), 10, 32)
			if err != nil || n < 0 {
				return 0, false
			}
			return time.Duration(n) * time.Second, true
		}
	}
	return 0, false
}

// checkURL refuses a source that is not an https URL.
func checkURL(raw string) error {
	if !strings.HasPrefix(strings.ToLower(raw), "https://") {
		return errors.New("statuslist: the status list URL must use https")
	}
	return nil
}
