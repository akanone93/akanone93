// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

package governance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/nightCode42/plux3/backend/edition"
	"github.com/nightCode42/plux3/backend/ext/governance"
)

// Reports is the enterprise compliance report writer (GOV-026). It holds no
// state; the zero value is ready to use.
type Reports struct{}

var _ governance.ReportRenderer = Reports{}

// The columns of the table of changes: their left edges and widths in
// points. They add up to the width of the text area.
var columns = [...]struct {
	title string
	width float64
}{{"When (UTC)", 84}, {"Change", 184}, {"Made by", 82}, {"Decisions", 145}}

// Type sizes of the document.
const (
	bodySize  = 8.0
	leading   = 10.0
	monoSize  = 7.0
	monoLead  = 9.0
	cellInset = 3.0
)

// PDF implements governance.ReportRenderer. It needs the licence's
// FeatureComplianceReports for reading. The document states the period, lists
// every change with its hash, decisions and evidence, and ends with the page
// of signatures that ties it to the DSSE envelope an auditor verifies.
func (Reports) PDF(ctx context.Context, r governance.Report, e governance.Envelope) ([]byte, error) {
	if err := edition.RequireRead(ctx, edition.FeatureComplianceReports); err != nil {
		return nil, fmt.Errorf("governance: %w", err)
	}
	statement, err := e.Statement()
	if err != nil {
		return nil, fmt.Errorf("governance: %w", err)
	}
	w := newPDFWriter("Plux compliance report")
	summarise(w, r)
	listChanges(w, r.Changes)
	sign(w, e, statement)
	return w.render(func(n, total int) string {
		return fmt.Sprintf("Plux compliance report, organisation %s - page %d of %d", r.OrganizationID, n, total)
	}), nil
}

// stamp renders an instant the way the document always does.
func stamp(t time.Time) string { return t.UTC().Format("2006-01-02 15:04:05") }

// summarise writes the title block: what the report covers and who asked.
func summarise(w *pdfWriter, r governance.Report) {
	w.line(fontBold, 18, margin, 26, "Compliance report")
	for _, l := range []string{
		"Organisation: " + r.OrganizationID,
		fmt.Sprintf("Period: %s to %s (UTC, end exclusive)", stamp(r.From), stamp(r.To)),
		fmt.Sprintf("Requested: %s by %s", stamp(r.GeneratedAt), r.GeneratedBy),
		fmt.Sprintf("Production changes: %d", len(r.Changes)),
	} {
		w.line(fontRegular, 10, margin, 14, l)
	}
	w.line(fontRegular, bodySize, margin, 14, "Every production change of the period with its author, approvers, timestamps, content hash and evidence (GOV-026).")
	w.rule(10)
}

// listChanges writes the table of changes, or says there are none.
func listChanges(w *pdfWriter, changes []governance.Change) {
	w.line(fontBold, 12, margin, 18, "Production changes")
	if len(changes) == 0 {
		w.line(fontRegular, 10, margin, 14, "No production change was made in this period.")
		return
	}
	header(w)
	for _, c := range changes {
		row(w, c)
	}
}

// header writes the heading row of the table.
func header(w *pdfWriter) {
	w.room(2 * leading)
	x := margin
	for _, col := range columns {
		w.at(fontBold, bodySize, x+cellInset, w.y-bodySize, col.title)
		x += col.width
	}
	w.y -= leading
	w.rule(4)
}

// row writes one change: its cells, then its hash, evidence and break-glass
// lines beneath them.
func row(w *pdfWriter, c governance.Change) {
	cells := [...][]string{
		{stamp(c.ChangedAt)},
		wrap(fontRegular, bodySize, columns[1].width-2*cellInset, fmt.Sprintf("%s %s: %s (%s)", c.Resource, c.Action, c.Subject, c.Environment)),
		wrap(fontRegular, bodySize, columns[2].width-2*cellInset, byline(c)),
		decisionLines(c),
	}
	height := 0
	for _, lines := range cells {
		height = max(height, len(lines))
	}
	w.room(float64(height)*leading + monoLead)
	x := margin
	for i, lines := range cells {
		for j, text := range lines {
			w.at(fontRegular, bodySize, x+cellInset, w.y-bodySize-float64(j)*leading, text)
		}
		x += columns[i].width
	}
	w.y -= float64(height) * leading
	detail(w, c)
	w.rule(6)
}

// byline names who made a change and who wrote it.
func byline(c governance.Change) string {
	if c.Author != "" && c.Author != c.ChangedBy {
		return c.ChangedBy + ", written by " + c.Author
	}
	return c.ChangedBy
}

// decisionLines lists the decisions on the change, fitted to their column.
func decisionLines(c governance.Change) []string {
	switch {
	case c.BreakGlass != nil:
		return []string{"break-glass, reviewed after"}
	case len(c.Approvals) == 0:
		return []string{"none asked for"}
	}
	var lines []string
	for _, a := range c.Approvals {
		text := fmt.Sprintf("%s: %s, %s", a.Decision, a.DeciderID, stamp(a.DecidedAt))
		lines = append(lines, wrap(fontRegular, bodySize, columns[3].width-2*cellInset, text)...)
	}
	return lines
}

// detail writes what sits under a change's row in small type: the content
// hash, the evidence and, for an emergency, the justification and review.
func detail(w *pdfWriter, c governance.Change) {
	mono := func(text string) {
		for _, l := range wrap(fontMono, monoSize, pageWidth-2*margin-2*cellInset, text) {
			w.line(fontMono, monoSize, margin+cellInset, monoLead, l)
		}
	}
	mono("sha256 " + c.ContentHash)
	if len(c.Evidence) > 0 {
		refs := make([]string, len(c.Evidence))
		for i, e := range c.Evidence {
			refs[i] = e.Kind + ":" + e.Ref
		}
		mono("evidence " + strings.Join(refs, " "))
	}
	if c.BreakGlass != nil {
		b := c.BreakGlass
		text := fmt.Sprintf("Break-glass: %s (review %s, due %s)", b.Justification, b.ReviewStatus, stamp(b.ReviewDueAt))
		for _, l := range wrap(fontRegular, bodySize, pageWidth-2*margin-2*cellInset, text) {
			w.line(fontRegular, bodySize, margin+cellInset, leading, l)
		}
	}
}

// sign writes the page of signatures: the identity of the signed statement and
// each signature over it, so that the document can be checked against the JSON
// envelope, and how to check that.
func sign(w *pdfWriter, e governance.Envelope, statement []byte) {
	w.newPage()
	w.line(fontBold, 14, margin, 22, "Signatures")
	sum := sha256.Sum256(statement)
	for _, l := range wrap(fontRegular, 10, pageWidth-2*margin,
		"This document renders a statement that the audit key signed as a DSSE envelope. The statement and the signatures below identify that envelope; verify the JSON file, not this document.") {
		w.line(fontRegular, 10, margin, 14, l)
	}
	w.y -= 8
	w.line(fontRegular, 10, margin, 14, "Payload type: "+e.PayloadType)
	w.line(fontRegular, 10, margin, 14, "Statement SHA-256:")
	w.line(fontMono, 8, margin, 12, hex.EncodeToString(sum[:]))
	for i, s := range e.Signatures {
		w.y -= 6
		w.line(fontBold, 10, margin, 14, fmt.Sprintf("Signature %d, key %s", i+1, s.KeyID))
		for _, l := range wrap(fontMono, 8, pageWidth-2*margin, s.Sig) {
			w.line(fontMono, 8, margin, 12, l)
		}
	}
	w.y -= 10
	w.line(fontRegular, 9, margin, 13, "Verify with: plux compliance verify <report.json> --key <key id>=<base64 public key>")
}
