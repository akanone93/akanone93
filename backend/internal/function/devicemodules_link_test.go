// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package function

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// busyLinker answers ErrLinkerBusy busy times, then links; it counts calls.
type busyLinker struct {
	busy  int32
	calls atomic.Int32
	gate  chan struct{}
}

func (b *busyLinker) Link(ctx context.Context, _ LinkInput) (LinkOutput, error) {
	n := b.calls.Add(1)
	if n <= b.busy {
		return LinkOutput{}, ErrLinkerBusy
	}
	if b.gate != nil {
		select {
		case <-b.gate:
		case <-ctx.Done():
			return LinkOutput{}, ctx.Err()
		}
	}
	return LinkOutput{Module: []byte("module")}, nil
}

// Verifies: FN-051.
// A device-module link the builder refuses as busy (it has every build slot
// in use, FN-016) is asked again after a wait, so a publish whose jobs need
// modules at once does not fail (build log #185); a builder busy for the
// whole budget is an error that says so.
func TestLinkWaitsForABusyBuilder_FN_051(t *testing.T) {
	t.Parallel()
	l := &busyLinker{busy: 2}
	out, err := linkPatiently(context.Background(), l, LinkInput{Timeout: time.Second}, time.Millisecond)
	if err != nil || string(out.Module) != "module" || l.calls.Load() != 3 {
		t.Fatalf("out %q, err %v, calls %d", out.Module, err, l.calls.Load())
	}
	always := &busyLinker{busy: 1 << 30}
	_, err = linkPatiently(context.Background(), always, LinkInput{Timeout: 20 * time.Millisecond}, time.Millisecond)
	if !errors.Is(err, ErrLinkerBusy) {
		t.Fatalf("a builder busy for the whole budget: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := linkPatiently(ctx, &busyLinker{busy: 1 << 30}, LinkInput{Timeout: 3 * time.Hour}, time.Hour); !errors.Is(err, context.Canceled) {
		t.Fatalf("an ended context: %v", err)
	}
}

// Verifies: FN-051.
// Compilations that need the same module at once share one link.
func TestConcurrentLinksOfOneSetShareTheBuild_FN_051(t *testing.T) {
	t.Parallel()
	l := &busyLinker{gate: make(chan struct{})}
	var flights linkFlights
	var wg sync.WaitGroup
	results := make([]string, 5)
	for i := range results {
		wg.Go(func() {
			out, err := flights.do(context.Background(), "key", func() (LinkOutput, error) {
				return linkPatiently(context.Background(), l, LinkInput{Timeout: time.Second}, time.Millisecond)
			})
			if err == nil {
				results[i] = string(out.Module)
			}
		})
	}
	for l.calls.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	time.Sleep(20 * time.Millisecond) // let the others join the link in progress
	close(l.gate)
	wg.Wait()
	if l.calls.Load() != 1 {
		t.Errorf("links = %d, want 1", l.calls.Load())
	}
	for i, r := range results {
		if r != "module" {
			t.Errorf("caller %d got %q", i, r)
		}
	}
}
