// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package release_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/release"
	"github.com/nightCode42/plux3/backend/internal/seccfg"
)

const calculatorPage = "plugins/loans/pages/calculator.page.json"

// addImage appends an Image without alternative text to the calculator
// page, replacing the one an earlier call added; a waiver, if given, is declared on the node.
func addImage(t *testing.T, f *fixture, waiver string) {
	t.Helper()
	ctx := context.Background()
	page, err := f.docs.GetDocument(ctx, f.owner, f.app, f.loans, calculatorPage)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(page.Content, &doc); err != nil {
		t.Fatal(err)
	}
	node := map[string]any{
		"id": "01f0c450-6c00-7000-8000-0000000a11d1", "type": "Image",
		"props": map[string]any{"source": map[string]any{"asset": map[string]any{"$asset": "01a0c450-6c00-700d-8000-000000019223"}}},
	}
	if waiver != "" {
		node["a11yWaivers"] = map[string]any{"imageAlternative": waiver}
	}
	column := doc["root"].(map[string]any)["slots"].(map[string]any)["body"].(map[string]any)["slots"].(map[string]any)["child"].(map[string]any)
	var kept []any
	for _, c := range column["children"].([]any) {
		if c.(map[string]any)["id"] != node["id"] {
			kept = append(kept, c)
		}
	}
	column["children"] = append(kept, node)
	content, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.docs.PutDocument(ctx, f.owner, f.app, f.loans, "s", calculatorPage, content, page.Revision); err != nil {
		t.Fatal(err)
	}
}

// setProfile sets the development environment's security profile.
func setProfile(t *testing.T, f *fixture, profile string) {
	t.Helper()
	if _, err := f.sec.Set(context.Background(), f.owner, seccfg.Change{
		AppID: f.app, EnvironmentID: f.envs["development"].ID, Profile: profile, Overrides: []byte(`{}`),
	}); err != nil {
		t.Fatalf("Set(%s): %v", profile, err)
	}
}

// a11ySchema compiles the schema of the accessibility report.
func a11ySchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	f, err := os.Open(filepath.Join("..", "..", "..", "schema", "json", "reports", "a11y-report.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	doc, err := jsonschema.UnmarshalJSON(f)
	if err != nil {
		t.Fatal(err)
	}
	const url = "https://plux.dev/schema/a11y-report.schema.json"
	c := jsonschema.NewCompiler()
	c.DefaultDraft(jsonschema.Draft2020)
	if err := c.AddResource(url, doc); err != nil {
		t.Fatal(err)
	}
	sch, err := c.Compile(url)
	if err != nil {
		t.Fatal(err)
	}
	return sch
}

// Verifies: A11Y-002.
// Under accessibilityBlocksPublish an unwaived accessibility error refuses
// the publish with PLX-3060 beside the finding; a waiver lets it through.
func TestAccessibilityBlocksPublish_A11Y_002(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	setProfile(t, f, "strict")
	addImage(t, f, "")
	job := f.publish(t, f.loans, true)
	if job.State != release.StateFailed || !hasCode(job.Diagnostics, plxerr.AccessibilityBlocksPublish) || !hasCode(job.Diagnostics, plxerr.ImageAlternativeMissing) {
		t.Fatalf("an unwaived error under strict: %+v", job)
	}
	addImage(t, f, "A purely decorative picture.")
	if job := f.publish(t, f.loans, false); job.State != release.StateSucceeded {
		t.Fatalf("a waived finding: %+v", job.Diagnostics)
	}
}

// setAppProfile sets the securityProfile of the app document.
func setAppProfile(t *testing.T, f *fixture, profile string) {
	t.Helper()
	ctx := context.Background()
	app, err := f.docs.GetDocument(ctx, f.owner, f.app, "", "app.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(app.Content, &doc); err != nil {
		t.Fatal(err)
	}
	doc["securityProfile"] = profile
	content, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.docs.PutDocument(ctx, f.owner, f.app, "", "s", "app.json", content, app.Revision); err != nil {
		t.Fatal(err)
	}
}

// Verifies: A11Y-002, SEC-180.
// Under the standard profile the same finding is a warning: it does not
// block, even with accessibilityBlocksPublish on, and an acknowledged
// publish goes through.
func TestAccessibilityWarningsDoNotBlock_A11Y_002(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	setAppProfile(t, f, "standard")
	setProfile(t, f, "strict")
	addImage(t, f, "")
	job := f.publish(t, f.loans, false)
	if job.State != release.StateFailed || hasCode(job.Diagnostics, plxerr.AccessibilityBlocksPublish) || !hasCode(job.Diagnostics, plxerr.WarningsNotAcknowledged) {
		t.Fatalf("a warning: %+v", job)
	}
	if job := f.publish(t, f.loans, true); job.State != release.StateSucceeded {
		t.Fatalf("acknowledged: %+v", job.Diagnostics)
	}
}

// Verifies: A11Y-008.
// A release stores the compiler's report, canonical and valid against its
// schema, serves it as JSON and as an escaped standalone page, and a
// rollback carries it along. A report that does not exist is not found.
func TestReleaseReport_A11Y_008(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newFixture(t)
	setProfile(t, f, "strict")
	const justification = `<script>alert("x")</script> decorative`
	addImage(t, f, justification)
	f.publish(t, "", false)
	if job := f.publish(t, f.loans, false); job.State != release.StateSucceeded {
		t.Fatalf("publish: %+v", job.Diagnostics)
	}
	dev := f.envs["development"].ID
	first, diags, err := f.rel.CreateRelease(ctx, f.owner, f.app, dev, nil, "")
	if err != nil || diags.HasErrors() {
		t.Fatalf("CreateRelease: %v %v", diags, err)
	}
	raw, contentType, err := f.rel.GetReport(ctx, f.viewer, f.app, first.Sequence, release.ReportAccessibility, release.ReportJSON)
	if err != nil || contentType != "application/json" {
		t.Fatalf("GetReport: %v %q", err, contentType)
	}
	inst, err := jsonschema.UnmarshalJSON(strings.NewReader(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	if err := a11ySchema(t).Validate(inst); err != nil {
		t.Errorf("the report does not match its schema: %v\n%s", err, raw)
	}
	if !strings.Contains(string(raw), `"waivers":[{`) {
		t.Errorf("the report lists no waiver: %s", raw)
	}
	page, contentType, err := f.rel.GetReport(ctx, f.viewer, f.app, first.Sequence, release.ReportAccessibility, release.ReportHTML)
	if err != nil || contentType != "text/html; charset=utf-8" {
		t.Fatalf("GetReport html: %v %q", err, contentType)
	}
	if strings.Contains(string(page), "<script>") || !strings.Contains(string(page), "&lt;script&gt;") {
		t.Errorf("the page does not escape the waiver:\n%s", page)
	}
	again, _, err := f.rel.GetReport(ctx, f.viewer, f.app, first.Sequence, release.ReportAccessibility, release.ReportHTML)
	if err != nil || string(again) != string(page) {
		t.Errorf("the page is not deterministic: %v", err)
	}

	if _, err := f.rel.PromoteRelease(ctx, f.owner, f.app, first.Sequence, f.envs["production"].ID, ""); err != nil {
		t.Fatal(err)
	}
	back, err := f.rel.RollbackRelease(ctx, f.owner, f.app, f.envs["production"].ID, "", first.Sequence, "")
	if err != nil {
		t.Fatal(err)
	}
	copied, _, err := f.rel.GetReport(ctx, f.viewer, f.app, back.Sequence, release.ReportAccessibility, release.ReportJSON)
	if err != nil || string(copied) != string(raw) {
		t.Errorf("the rollback's report differs: %v", err)
	}

	// Verifies: I18N-008. The release also stores a localisation report.
	l10n, _, err := f.rel.GetReport(ctx, f.viewer, f.app, first.Sequence, release.ReportLocalisation, release.ReportJSON)
	if err != nil || !strings.Contains(string(l10n), `"defaultLocale"`) {
		t.Errorf("the localisation report: %v\n%s", err, l10n)
	}
	if _, _, err := f.rel.GetReport(ctx, f.viewer, f.app, first.Sequence, release.ReportKind("nonexistent"), release.ReportJSON); code(err) != plxerr.ResourceNotFound {
		t.Errorf("a report that does not exist: %v", err)
	}
	if _, _, err := f.rel.GetReport(ctx, f.viewer, f.app, 99, release.ReportAccessibility, release.ReportJSON); code(err) != plxerr.ResourceNotFound {
		t.Errorf("a missing release: %v", err)
	}
}
