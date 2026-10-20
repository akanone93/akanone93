// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package scheduler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/nightCode42/plux3/backend/internal/storage"
)

// DefaultInterval is how often the leader runs a pass.
const DefaultInterval = 15 * time.Second

// LockKey is the advisory lock the replicas elect a leader with: "SCHD".
const LockKey int64 = 0x53434844

// Job is periodic work for one organisation. It must be idempotent: a pass
// may repeat after a failure.
type Job interface {
	// Name identifies the job in logs.
	Name() string
	// Run does the job for the organisation.
	Run(ctx context.Context, organizationID string) error
}

// JobFunc adapts a function to a Job.
type JobFunc struct {
	Label string
	F     func(ctx context.Context, organizationID string) error
}

// Name returns the label.
func (j JobFunc) Name() string { return j.Label }

// Run calls the function.
func (j JobFunc) Run(ctx context.Context, organizationID string) error {
	return j.F(ctx, organizationID)
}

// Locker takes the leader lock; *storage.DB is one.
type Locker interface {
	TryAdvisoryLock(ctx context.Context, key int64) (*storage.AdvisoryLock, error)
}

// Options configure a Scheduler.
type Options struct {
	// Locker elects the leader.
	Locker Locker
	// Organizations calls f for every organisation, as
	// tenancy.Service.ForEachOrganization does.
	Organizations func(ctx context.Context, f func(organizationID string) error) error
	// Jobs run, in order, for every organisation on every pass.
	Jobs []Job
	// LockKey names the advisory lock; zero is LockKey. Replicas that are to
	// elect one leader must share it.
	LockKey int64
	// Interval is the time between passes; zero is DefaultInterval.
	Interval time.Duration
	// Log receives the failures of jobs; nil discards them.
	Log *slog.Logger
}

// Scheduler runs its jobs while it is the leader.
type Scheduler struct {
	o Options

	mu     sync.Mutex
	lock   *storage.AdvisoryLock
	cancel context.CancelFunc
	done   chan struct{}
}

// New returns a scheduler that has not started.
func New(o Options) (*Scheduler, error) {
	if o.Locker == nil || o.Organizations == nil {
		return nil, errors.New("scheduler: a scheduler needs a locker and a way to list organisations")
	}
	if o.LockKey == 0 {
		o.LockKey = LockKey
	}
	if o.Interval <= 0 {
		o.Interval = DefaultInterval
	}
	return &Scheduler{o: o}, nil
}

// Name identifies the scheduler as a server component.
func (*Scheduler) Name() string { return "scheduler" }

// Leader reports whether this scheduler holds the leader lock.
func (s *Scheduler) Leader() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lock != nil
}

// Step runs one pass if this scheduler is, or can become, the leader, and
// reports whether it ran one. A lock whose connection has died is dropped
// first, so the scheduler stops acting as the leader the moment it can
// tell. The failures of the jobs are returned joined; the pass still ran.
func (s *Scheduler) Step(ctx context.Context) (bool, error) {
	if !s.elect(ctx) {
		return false, nil
	}
	return true, s.pass(ctx)
}

// elect keeps or takes the leader lock.
func (s *Scheduler) elect(ctx context.Context) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lock != nil {
		if s.lock.Held(ctx) {
			return true
		}
		s.lock.Release(ctx)
		s.lock = nil
	}
	lock, err := s.o.Locker.TryAdvisoryLock(ctx, s.o.LockKey)
	if err != nil {
		s.warn(ctx, "leader election failed", slog.Any("error", err))
		return false
	}
	s.lock = lock
	return lock != nil
}

// pass runs every job for every organisation.
func (s *Scheduler) pass(ctx context.Context) error {
	var errs []error
	err := s.o.Organizations(ctx, func(org string) error {
		for _, j := range s.o.Jobs {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if err := j.Run(ctx, org); err != nil {
				errs = append(errs, fmt.Errorf("scheduler: %s: %w", j.Name(), err))
			}
		}
		return nil
	})
	if err != nil {
		errs = append(errs, fmt.Errorf("scheduler: %w", err))
	}
	return errors.Join(errs...)
}

// Start begins the loop. It returns at once; Stop ends it.
func (s *Scheduler) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		return errors.New("scheduler: already started")
	}
	ctx, cancel := context.WithCancel(ctx)
	s.cancel, s.done = cancel, make(chan struct{})
	go s.loop(ctx, s.done)
	return nil
}

// loop runs a pass every interval until ctx ends.
func (s *Scheduler) loop(ctx context.Context, done chan<- struct{}) {
	defer close(done)
	ticker := time.NewTicker(s.o.Interval)
	defer ticker.Stop()
	for {
		if _, err := s.Step(ctx); err != nil && ctx.Err() == nil {
			s.warn(ctx, "a pass failed", slog.Any("error", err))
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// Stop ends the loop, waits for the pass in flight to finish, and releases
// the leader lock so another replica takes over at once.
func (s *Scheduler) Stop(ctx context.Context) error {
	s.mu.Lock()
	cancel, done := s.cancel, s.done
	s.cancel = nil
	s.mu.Unlock()
	var err error
	if cancel != nil {
		cancel()
		select {
		case <-done:
		case <-ctx.Done():
			err = fmt.Errorf("scheduler: stop: %w", ctx.Err())
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lock != nil {
		s.lock.Release(context.WithoutCancel(ctx))
		s.lock = nil
	}
	return err
}

func (s *Scheduler) warn(ctx context.Context, msg string, args ...any) {
	if s.o.Log != nil {
		s.o.Log.WarnContext(ctx, "scheduler: "+msg, args...)
	}
}
