// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"text/tabwriter"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/l10n/xchange"
	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
)

const l10nUsage = "Usage: plux l10n pull|push|export|import|status [flags]\n"

// localeFile matches the translations document of a locale in a project:
// translations/<locale>.json (the locale of common.schema.json).
var localeFile = regexp.MustCompile(`^translations/([a-z]{2,3}(?:-[A-Z][a-z]{3})?(?:-(?:[A-Z]{2}|[0-9]{3}))?)\.json$`)

// l10n runs plux l10n pull|push|export|import|status (CLI-003, I18N-009).
func (e env) l10n(args []string) int {
	if len(args) < 1 {
		_, _ = fmt.Fprint(e.stderr, l10nUsage)
		return exitUsage
	}
	switch args[0] {
	case "pull":
		return e.l10nPull(args[1:])
	case "push":
		return e.l10nPush(args[1:])
	case "export":
		return e.l10nExport(args[1:])
	case "import":
		return e.l10nImport(args[1:])
	case "status":
		return e.l10nStatus(args[1:])
	}
	_, _ = fmt.Fprint(e.stderr, l10nUsage)
	return exitUsage
}

// l10nTarget is the app a l10n command acts on, with its clients.
type l10nTarget struct {
	c      common
	cl     *clients
	client pluxv1connect.LocalisationServiceClient
}

// l10nConnect resolves the common flags and connects.
func (e env) l10nConnect(c common) (l10nTarget, error) {
	if err := c.resolve(); err != nil {
		return l10nTarget{}, err
	}
	if err := needApp(c); err != nil {
		return l10nTarget{}, err
	}
	cl, err := e.connect(c, true)
	if err != nil {
		return l10nTarget{}, err
	}
	client := pluxv1connect.NewLocalisationServiceClient(e.httpClient, c.server, connect.WithInterceptors(headers{token: cl.token, org: c.org}))
	return l10nTarget{c: c, cl: cl, client: client}, nil
}

// l10nPull writes the server's translations/<locale>.json files into the
// project.
func (e env) l10nPull(args []string) int {
	var c common
	set := flag.NewFlagSet("l10n pull", flag.ContinueOnError)
	c.register(set, true)
	if _, code, ok := parse(set, args, e.stderr, "Usage: plux l10n pull [-C dir] [--app id] [--json]\n\nWrites the server's translations/<locale>.json files into the project.", 0); !ok {
		return code
	}
	t, err := e.l10nConnect(c)
	if err != nil {
		return e.fail("l10n pull", err)
	}
	files, err := exportDraft(context.Background(), t.cl, t.c.app)
	if err != nil {
		return e.fail("l10n pull", err)
	}
	var written []string
	for _, p := range slices.Sorted(mapKeys(files)) {
		if !localeFile.MatchString(p) {
			continue
		}
		target := filepath.Join(t.c.dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil { //nolint:gosec // G301: project files.
			return e.fail("l10n pull", err)
		}
		if err := writeFile(target, files[p]); err != nil {
			return e.fail("l10n pull", err)
		}
		written = append(written, p)
	}
	return e.emit(t.c.json, map[string]any{"files": written}, fmt.Sprintf("Wrote %d translation files to %s.", len(written), t.c.dir))
}

// l10nPush uploads the project's translations/<locale>.json files, each
// through ImportTranslations, so the server applies the review rules.
func (e env) l10nPush(args []string) int {
	var c common
	set := flag.NewFlagSet("l10n push", flag.ContinueOnError)
	c.register(set, true)
	dry := set.Bool("dry-run", false, "report what would change and save nothing")
	if _, code, ok := parse(set, args, e.stderr, "Usage: plux l10n push [-C dir] [--app id] [--dry-run] [--json]\n\nUploads the project's translations/<locale>.json files.", 0); !ok {
		return code
	}
	t, err := e.l10nConnect(c)
	if err != nil {
		return e.fail("l10n push", err)
	}
	files, err := readProject(t.c.dir)
	if err != nil {
		return e.fail("l10n push", err)
	}
	var (
		results []map[string]any
		failed  bool
		text    strings.Builder
	)
	for _, p := range slices.Sorted(mapKeys(files)) {
		m := localeFile.FindStringSubmatch(p)
		if m == nil {
			continue
		}
		res, err := t.upload(context.Background(), m[1], "json", files[p], *dry)
		if err != nil {
			return e.fail("l10n push", fmt.Errorf("%s: %w", p, err))
		}
		failed = failed || res.hasErrors()
		results = append(results, res.view(m[1]))
		text.WriteString(res.text(m[1]))
	}
	if len(results) == 0 {
		return e.fail("l10n push", fmt.Errorf("no translations/<locale>.json files in %s", t.c.dir))
	}
	code := e.emit(t.c.json, map[string]any{"locales": results}, text.String())
	if code == exitOK && failed {
		return exitFailed
	}
	return code
}

// importOutcome is the answer to an import.
type importOutcome struct {
	res *pluxv1.ImportTranslationsResponse
}

func (o importOutcome) hasErrors() bool {
	return slices.ContainsFunc(o.res.GetDiagnostics(), func(d *pluxv1.Diagnostic) bool {
		return d.GetSeverity() == pluxv1.Severity_SEVERITY_ERROR
	})
}

func (o importOutcome) view(locale string) map[string]any {
	diags := make([]map[string]any, 0, len(o.res.GetDiagnostics()))
	for _, d := range o.res.GetDiagnostics() {
		diags = append(diags, map[string]any{
			"code": d.GetCode(), "severity": severityName(d.GetSeverity()), "path": d.GetLocation().GetPath(), "message": d.GetMessage(),
		})
	}
	return map[string]any{"locale": locale, "saved": o.res.GetSaved(), "changedKeys": o.res.GetChangedKeys(), "diagnostics": diags}
}

func (o importOutcome) text(locale string) string {
	var b strings.Builder
	for _, d := range o.res.GetDiagnostics() {
		fmt.Fprintf(&b, "%s: %s %s %s: %s\n", locale, severityName(d.GetSeverity()), d.GetCode(), d.GetLocation().GetPath(), d.GetMessage())
	}
	verb := "saved"
	if !o.res.GetSaved() {
		verb = "unchanged or not saved"
	}
	fmt.Fprintf(&b, "%s: %d keys changed, %s.\n", locale, len(o.res.GetChangedKeys()), verb)
	return b.String()
}

func severityName(s pluxv1.Severity) string {
	return strings.ToLower(strings.TrimPrefix(s.String(), "SEVERITY_"))
}

// upload sends one translation file, after checking it against the import
// size limit the server also enforces (translation.importSize).
func (t l10nTarget) upload(ctx context.Context, locale, format string, data []byte, dry bool) (importOutcome, error) {
	if limit := limits.Defaults().Get(limits.TranslationImportSize); int64(len(data)) > limit {
		return importOutcome{}, fmt.Errorf("the file is %d bytes; translation.importSize allows %d", len(data), limit)
	}
	res, err := t.client.ImportTranslations(ctx, connect.NewRequest(&pluxv1.ImportTranslationsRequest{
		AppId: t.c.app, Locale: locale, Format: format, Content: data, DryRun: dry,
	}))
	if err != nil {
		return importOutcome{}, err //nolint:wrapcheck // reported as is
	}
	return importOutcome{res: res.Msg}, nil
}

// l10nExport writes a locale's translations in a format.
func (e env) l10nExport(args []string) int {
	var c common
	set := flag.NewFlagSet("l10n export", flag.ContinueOnError)
	c.register(set, true)
	locale := set.String("locale", "", "the `locale` to export")
	format := set.String("format", "", "the `format`: xliff, arb, json or csv")
	out := set.String("out", "", "write to this `file` (default standard output)")
	if _, code, ok := parse(set, args, e.stderr, "Usage: plux l10n export --locale de --format xliff|arb|json|csv [--out file] [-C dir] [--app id]\n\nExports a locale for a translation tool; the app's default locale is the source.", 0); !ok {
		return code
	}
	f, err := xchange.ParseFormat(*format)
	if err != nil || *locale == "" {
		return e.fail("l10n export", usageError("pass --locale and --format xliff, arb, json or csv"))
	}
	t, err := e.l10nConnect(c)
	if err != nil {
		return e.fail("l10n export", err)
	}
	res, err := t.client.ExportTranslations(context.Background(), connect.NewRequest(&pluxv1.ExportTranslationsRequest{
		AppId: t.c.app, Locale: *locale, Format: string(f),
	}))
	if err != nil {
		return e.fail("l10n export", err)
	}
	if *out == "" {
		if _, err := e.stdout.Write(res.Msg.GetContent()); err != nil {
			return e.fail("l10n export", err)
		}
		return exitOK
	}
	if err := writeFile(*out, res.Msg.GetContent()); err != nil {
		return e.fail("l10n export", err)
	}
	return e.emit(t.c.json, map[string]any{"file": *out, "locale": *locale, "format": string(f), "sourceLocale": res.Msg.GetSourceLocale()},
		fmt.Sprintf("Wrote %s (%s, source %s).", *out, f, res.Msg.GetSourceLocale()))
}

// formatOfFile guesses a format from a file name.
func formatOfFile(name string) (xchange.Format, bool) {
	switch strings.ToLower(path.Ext(filepath.ToSlash(name))) {
	case ".xlf", ".xliff":
		return xchange.XLIFF, true
	case ".arb":
		return xchange.ARB, true
	case ".csv":
		return xchange.CSV, true
	case ".json":
		return xchange.JSON, true
	}
	return "", false
}

// l10nImport applies a translation file to a locale.
func (e env) l10nImport(args []string) int {
	var c common
	set := flag.NewFlagSet("l10n import", flag.ContinueOnError)
	c.register(set, true)
	locale := set.String("locale", "", "the `locale` the file translates")
	format := set.String("format", "", "the `format`: xliff, arb, json or csv (default from the file extension)")
	dry := set.Bool("dry-run", false, "report what would change and save nothing")
	pos, code, ok := parseInterleaved(set, args, e.stderr, "Usage: plux l10n import --locale de [--format xliff|arb|json|csv] [--dry-run] <file> [-C dir] [--app id]\n\nApplies a translation file to a locale; the server checks it and applies the review rules.")
	if !ok {
		return code
	}
	if len(pos) != 1 || *locale == "" {
		return e.fail("l10n import", usageError("pass --locale and one file"))
	}
	f := xchange.Format(*format)
	if *format == "" {
		guess, known := formatOfFile(pos[0])
		if !known {
			return e.fail("l10n import", usageError("cannot tell the format from the file name; pass --format"))
		}
		f = guess
	}
	if _, err := xchange.ParseFormat(string(f)); err != nil {
		return e.fail("l10n import", usageError(err.Error()))
	}
	info, err := os.Stat(pos[0])
	if err != nil {
		return e.fail("l10n import", err)
	}
	if limit := limits.Defaults().Get(limits.TranslationImportSize); info.Size() > limit {
		return e.fail("l10n import", fmt.Errorf("%s is %d bytes; translation.importSize allows %d", pos[0], info.Size(), limit))
	}
	data, err := os.ReadFile(pos[0])
	if err != nil {
		return e.fail("l10n import", err)
	}
	t, err := e.l10nConnect(c)
	if err != nil {
		return e.fail("l10n import", err)
	}
	res, err := t.upload(context.Background(), *locale, string(f), data, *dry)
	if err != nil {
		return e.fail("l10n import", err)
	}
	code = e.emit(t.c.json, res.view(*locale), res.text(*locale))
	if code == exitOK && res.hasErrors() {
		return exitFailed
	}
	return code
}

// l10nStatus prints the progress of every locale.
func (e env) l10nStatus(args []string) int {
	var c common
	set := flag.NewFlagSet("l10n status", flag.ContinueOnError)
	c.register(set, true)
	if _, code, ok := parse(set, args, e.stderr, "Usage: plux l10n status [-C dir] [--app id] [--json]\n\nCounts the translation keys of each locale by review state.", 0); !ok {
		return code
	}
	t, err := e.l10nConnect(c)
	if err != nil {
		return e.fail("l10n status", err)
	}
	res, err := t.client.GetStatus(context.Background(), connect.NewRequest(&pluxv1.GetStatusRequest{AppId: t.c.app}))
	if err != nil {
		return e.fail("l10n status", err)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "source locale %s\n\n", res.Msg.GetSourceLocale())
	w := tabwriter.NewWriter(&b, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "LOCALE\tTOTAL\tNEW\tTRANSLATED\tREVIEWED\tAPPROVED\tMISSING")
	rows := make([]map[string]any, 0, len(res.Msg.GetLocales()))
	for _, l := range res.Msg.GetLocales() {
		_, _ = fmt.Fprintf(w, "%s\t%d\t%d\t%d\t%d\t%d\t%d\n", l.GetLocale(), l.GetTotal(), l.GetNew(), l.GetTranslated(), l.GetReviewed(), l.GetApproved(), l.GetMissing())
		rows = append(rows, map[string]any{
			"locale": l.GetLocale(), "total": l.GetTotal(), "new": l.GetNew(), "translated": l.GetTranslated(),
			"reviewed": l.GetReviewed(), "approved": l.GetApproved(), "missing": l.GetMissing(),
		})
	}
	_ = w.Flush()
	return e.emit(t.c.json, map[string]any{"sourceLocale": res.Msg.GetSourceLocale(), "locales": rows}, b.String())
}
