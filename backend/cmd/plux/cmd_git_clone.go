// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"

	"github.com/go-git/go-git/v5"
)

// cloneRepository clones the default branch of a repository into dir, at
// its newest commit only. It is the clone of plux git import, which reads
// the working tree and nothing else.
func cloneRepository(ctx context.Context, address, dir string) error {
	if _, err := git.PlainCloneContext(ctx, dir, false, &git.CloneOptions{URL: address, Depth: 1}); err != nil {
		return fmt.Errorf("git import: clone: %w", err)
	}
	return nil
}
