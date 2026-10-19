// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package gitsync

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"time"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-billy/v5/util"
	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/storage/memory"

	"github.com/nightCode42/plux3/backend/internal/document"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// push is one commit to make on a remote branch and push.
type push struct {
	remote, branch string
	username       string
	token          string
	files          []document.File
	name, email    string
	when           time.Time
	message        string
	proxy          transport.ProxyOptions
	caBundle       []byte
}

// commitAndPush clones the branch, replaces its tree with the files,
// commits when anything changed, pushes, and returns the head commit. All
// of it happens in memory.
func commitAndPush(ctx context.Context, p push) (string, error) {
	repo, err := p.clone(ctx)
	if err != nil {
		return "", err
	}
	wt, err := repo.Worktree()
	if err != nil {
		return "", fmt.Errorf("gitsync: worktree: %w", err)
	}
	if err := replaceTree(wt.Filesystem, p.files); err != nil {
		return "", err
	}
	if err := wt.AddWithOptions(&git.AddOptions{All: true}); err != nil {
		return "", fmt.Errorf("gitsync: stage: %w", err)
	}
	status, err := wt.Status()
	if err != nil {
		return "", fmt.Errorf("gitsync: status: %w", err)
	}
	head, headErr := repo.Head()
	if status.IsClean() && headErr == nil {
		return head.Hash().String(), nil // the remote already holds this tree
	}
	sig := &object.Signature{Name: p.name, Email: p.email, When: p.when}
	hash, err := wt.Commit(p.message, &git.CommitOptions{Author: sig, Committer: sig})
	if err != nil {
		return "", fmt.Errorf("gitsync: commit: %w", err)
	}
	ref := plumbing.NewBranchReferenceName(p.branch)
	err = repo.PushContext(ctx, &git.PushOptions{
		RemoteName: "origin",
		RefSpecs:   []config.RefSpec{config.RefSpec(ref + ":" + ref)},
		Auth:       p.auth(), ProxyOptions: p.proxy, CABundle: p.caBundle,
	})
	if err != nil && !errors.Is(err, git.NoErrAlreadyUpToDate) {
		return "", refusal(err)
	}
	return hash.String(), nil
}

// auth is the credential the transport sends.
func (p push) auth() *githttp.BasicAuth {
	return &githttp.BasicAuth{Username: p.username, Password: p.token}
}

// clone fetches the newest commit of the branch, or starts the branch in an
// empty repository when the remote has none to fetch.
func (p push) clone(ctx context.Context) (*git.Repository, error) {
	ref := plumbing.NewBranchReferenceName(p.branch)
	store, tree := memory.NewStorage(), memfs.New()
	repo, err := git.CloneContext(ctx, store, tree, &git.CloneOptions{
		URL: p.remote, RemoteName: "origin", Auth: p.auth(), ReferenceName: ref, SingleBranch: true, Depth: 1,
		ProxyOptions: p.proxy, CABundle: p.caBundle,
	})
	switch {
	case err == nil:
		return repo, nil
	case errors.Is(err, transport.ErrEmptyRemoteRepository), errors.Is(err, plumbing.ErrReferenceNotFound):
		return p.initialise(ref)
	}
	return nil, refusal(err)
}

// initialise creates a repository whose branch is unborn, with the remote.
func (p push) initialise(ref plumbing.ReferenceName) (*git.Repository, error) {
	repo, err := git.InitWithOptions(memory.NewStorage(), memfs.New(), git.InitOptions{DefaultBranch: ref})
	if err != nil {
		return nil, fmt.Errorf("gitsync: init: %w", err)
	}
	if _, err := repo.CreateRemote(&config.RemoteConfig{Name: "origin", URLs: []string{p.remote}}); err != nil {
		return nil, fmt.Errorf("gitsync: add the remote: %w", err)
	}
	return repo, nil
}

// replaceTree makes the worktree hold exactly the files.
func replaceTree(tree billy.Filesystem, files []document.File) error {
	entries, err := tree.ReadDir("/")
	if err != nil {
		return fmt.Errorf("gitsync: read the worktree: %w", err)
	}
	for _, e := range entries {
		if err := util.RemoveAll(tree, e.Name()); err != nil {
			return fmt.Errorf("gitsync: clear the worktree: %w", err)
		}
	}
	for _, f := range files {
		if !fs.ValidPath(f.Path) || f.Path == "." {
			return plxerr.New(plxerr.InvalidProjectLayout, "%q is not a path of the project layout", f.Path)
		}
		if err := util.WriteFile(tree, f.Path, f.Content, 0o644); err != nil {
			return fmt.Errorf("gitsync: write %s: %w", f.Path, err)
		}
	}
	return nil
}

// refusal turns a transport failure into PLX-8503 with a reason that never
// repeats what the remote said, which could echo a credential.
func refusal(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("gitsync: %w", err)
	}
	reason := "the remote could not be reached or refused the push"
	switch {
	case errors.Is(err, transport.ErrAuthenticationRequired), errors.Is(err, transport.ErrAuthorizationFailed):
		reason = "the remote did not accept the credential"
	case errors.Is(err, transport.ErrRepositoryNotFound):
		reason = "the remote has no such repository, or the credential cannot see it"
	case errors.Is(err, git.ErrNonFastForwardUpdate):
		reason = "the branch moved on the remote and is not a fast-forward"
	}
	return plxerr.Wrap(plxerr.GitRemoteRefused, err, "%s", reason)
}
