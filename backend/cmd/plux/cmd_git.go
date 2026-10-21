// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/pluxv1"
)

const (
	gitUsage         = "Usage: plux git export configure|status|run [flags]\n       plux git import <dir|url> [flags]\n"
	gitExportUsage   = "Usage: plux git export configure|status|run [flags]\n"
	gitConfigureHelp = "Usage: plux git export configure --remote https-url --credential key --environment id [--branch name] [--app id] [--json]\n\n" +
		"Exports the app to a Git repository on every publish (GOV-011). The token is a secret of the\n" +
		"environment named by --environment; the remote holds no credentials."
	gitStatusHelp = "Usage: plux git export status [--app id] [--json]\n\nShows the export settings of the app and its newest exports."
	gitRunHelp    = "Usage: plux git export run [--app id] [--json]\n\n" +
		"Exports the app's newest published version now, or returns the finished export."
	gitImportHelp = "Usage: plux git import <dir|url> [--app id] [--json]\n\n" +
		"Imports a repository in the project layout (SCH-006) as the app's drafts, through the same\n" +
		"validation as plux import. A url must be https://."
)

// parseAppCommand parses the flags of a command on one app: the common
// ones, the command's extra ones and its positional arguments. The app
// defaults to plux.json.
func (e env) parseAppCommand(cmd, usage string, args []string, positional int, extra func(*flag.FlagSet)) (common, []string, int, bool) {
	c := common{dir: "."}
	set := flag.NewFlagSet(cmd, flag.ContinueOnError)
	c.register(set, false)
	set.StringVar(&c.app, "app", "", "the app `id` (default plux.json)")
	if extra != nil {
		extra(set)
	}
	rest, code, ok := parseInterleaved(set, args, e.stderr, usage)
	if !ok {
		return c, nil, code, false
	}
	if len(rest) != positional {
		set.Usage()
		return c, nil, exitUsage, false
	}
	if err := c.resolve(); err != nil {
		return c, nil, e.fail(cmd, err), false
	}
	if err := needApp(c); err != nil {
		return c, nil, e.fail(cmd, err), false
	}
	return c, rest, exitOK, true
}

// git dispatches plux git export and plux git import (GOV-011).
func (e env) git(args []string) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(e.stderr, gitUsage)
		return exitUsage
	}
	switch args[0] {
	case "export":
		return e.gitExport(args[1:])
	case "import":
		return e.gitImport(args[1:])
	default:
		_, _ = fmt.Fprintf(e.stderr, "%s git: unknown subcommand %q\n\n%s", name, args[0], gitUsage)
		return exitUsage
	}
}

// gitExport dispatches the export subcommands.
func (e env) gitExport(args []string) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(e.stderr, gitExportUsage)
		return exitUsage
	}
	switch args[0] {
	case "configure":
		return e.gitConfigure(args[1:])
	case "status":
		return e.gitStatus(args[1:])
	case "run":
		return e.gitRun(args[1:])
	default:
		_, _ = fmt.Fprintf(e.stderr, "%s git export: unknown subcommand %q\n\n%s", name, args[0], gitExportUsage)
		return exitUsage
	}
}

// gitConfigure sets the export of an app and enables it.
func (e env) gitConfigure(args []string) int {
	var remote, branch, credential, environment string
	c, _, code, ok := e.parseAppCommand("git export configure", gitConfigureHelp, args, 0, func(set *flag.FlagSet) {
		set.StringVar(&remote, "remote", "", "the repository's https `address`")
		set.StringVar(&branch, "branch", "", "the `branch` each export commits to (default main)")
		set.StringVar(&credential, "credential", "", "the `name` of the environment secret that holds the token")
		set.StringVar(&environment, "environment", "", "the `id` of the environment that holds the secret")
	})
	if !ok {
		return code
	}
	cl, err := e.connect(c, true)
	if err != nil {
		return e.fail("git export configure", err)
	}
	res, err := cl.gitExp.ConfigureExport(context.Background(), connect.NewRequest(&pluxv1.ConfigureExportRequest{
		Settings: &pluxv1.GitSettings{
			AppId: c.app, Enabled: true, RemoteUrl: remote, Branch: branch,
			CredentialEnvironmentId: environment, CredentialKey: credential,
		},
	}))
	if err != nil {
		return e.fail("git export configure", err)
	}
	s := res.Msg.GetSettings()
	return e.emit(c.json, gitSettingsView(s), gitSettingsLine(s))
}

// gitStatus prints the export settings of an app and its newest exports.
func (e env) gitStatus(args []string) int {
	c, _, code, ok := e.parseAppCommand("git export status", gitStatusHelp, args, 0, nil)
	if !ok {
		return code
	}
	cl, err := e.connect(c, true)
	if err != nil {
		return e.fail("git export status", err)
	}
	ctx := context.Background()
	settings, err := cl.gitExp.GetSettings(ctx, connect.NewRequest(&pluxv1.GetSettingsRequest{AppId: c.app}))
	if err != nil {
		return e.fail("git export status", err)
	}
	exports, err := cl.gitExp.ListExports(ctx, connect.NewRequest(&pluxv1.ListExportsRequest{AppId: c.app}))
	if err != nil {
		return e.fail("git export status", err)
	}
	return e.printGitStatus(c.json, settings.Msg.GetSettings(), exports.Msg.GetExports())
}

// printGitStatus prints the settings and the exports as JSON or text.
func (e env) printGitStatus(jsonOut bool, s *pluxv1.GitSettings, exports []*pluxv1.GitExport) int {
	views := make([]map[string]any, 0, len(exports))
	text := gitSettingsLine(s)
	for _, x := range exports {
		views = append(views, gitExportView(x))
		text += gitExportLine(x)
	}
	return e.emit(jsonOut, map[string]any{"settings": gitSettingsView(s), "exports": views}, text)
}

// gitRun exports the newest published version of an app now.
func (e env) gitRun(args []string) int {
	c, _, code, ok := e.parseAppCommand("git export run", gitRunHelp, args, 0, nil)
	if !ok {
		return code
	}
	cl, err := e.connect(c, true)
	if err != nil {
		return e.fail("git export run", err)
	}
	res, err := cl.gitExp.RunExport(context.Background(), connect.NewRequest(&pluxv1.RunExportRequest{AppId: c.app}))
	if err != nil {
		return e.fail("git export run", err)
	}
	x := res.Msg.GetExport()
	return e.emit(c.json, gitExportView(x), gitExportLine(x))
}

// gitImport replaces the app's drafts with a repository's project layout,
// through the same validation and drafts as plux import (GOV-011).
func (e env) gitImport(args []string) int {
	c, rest, code, ok := e.parseAppCommand("git import", gitImportHelp, args, 1, nil)
	if !ok {
		return code
	}
	cl, err := e.connect(c, true)
	if err != nil {
		return e.fail("git import", err)
	}
	ctx := context.Background()
	files, err := e.readRepository(ctx, rest[0])
	if err != nil {
		return e.fail("git import", err)
	}
	res, err := importDraft(ctx, cl, c.app, files)
	if err != nil {
		return e.fail("git import", err)
	}
	return e.emit(c.json, map[string]any{"files": len(files), "revision": res.GetRevision()}, fmt.Sprintf("Imported %d files.", len(files)))
}

// readRepository reads the project layout of a local directory, or of an
// https repository cloned into a temporary directory that is removed after
// reading.
func (e env) readRepository(ctx context.Context, src string) (map[string][]byte, error) {
	if !strings.Contains(src, "://") {
		return readProject(src)
	}
	if !strings.HasPrefix(src, "https://") {
		return nil, usageError("the repository must be an https:// address")
	}
	dir, err := os.MkdirTemp("", "plux-git-import-")
	if err != nil {
		return nil, fmt.Errorf("git import: temporary directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	if err := e.clone(ctx, src, dir); err != nil {
		return nil, err
	}
	return readProject(dir)
}

// gitSettingsView is the JSON form of an export setting; it names the
// credential's secret but never holds its value.
func gitSettingsView(s *pluxv1.GitSettings) map[string]any {
	return map[string]any{
		"app_id": s.GetAppId(), "enabled": s.GetEnabled(), "remote_url": s.GetRemoteUrl(),
		"branch": s.GetBranch(), "username": s.GetUsername(),
		"credential_environment_id": s.GetCredentialEnvironmentId(), "credential_key": s.GetCredentialKey(),
	}
}

// gitSettingsLine is the text form of an export setting.
func gitSettingsLine(s *pluxv1.GitSettings) string {
	state := "disabled"
	if s.GetEnabled() {
		state = "enabled"
	}
	return fmt.Sprintf("%s\t%s\t%s\t%s\n", state, s.GetRemoteUrl(), s.GetBranch(), s.GetCredentialKey())
}

// gitExportView is the JSON form of one export.
func gitExportView(x *pluxv1.GitExport) map[string]any {
	return map[string]any{
		"id": x.GetId(), "app_id": x.GetAppId(), "version_id": x.GetVersionId(), "state": x.GetState(),
		"commit": x.GetCommit(), "attempts": x.GetAttempts(), "error_code": x.GetErrorCode(),
	}
}

// gitExportLine is the text form of one export.
func gitExportLine(x *pluxv1.GitExport) string {
	return fmt.Sprintf("%s\t%s\t%s\t%s\n", x.GetId(), x.GetVersionId(), x.GetState(), x.GetCommit())
}
