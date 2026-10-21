// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

package governance

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/edition"
	"github.com/nightCode42/plux3/backend/ext/governance"
)

// sampleReport returns a report of n changes: a promotion with two decisions,
// a break-glass kill switch and a change no policy asked approval for, in
// turn.
func sampleReport(n int) governance.Report {
	r := governance.Report{
		Version: governance.ReportVersion, OrganizationID: "org-1",
		From: utc(2026, 10, 1, 0, 0), To: utc(2026, 11, 1, 0, 0), GeneratedAt: utc(2026, 11, 1, 9, 30), GeneratedBy: "usr-owner",
	}
	hash := strings.Repeat("ab12", 16)
	for i := range n {
		c := governance.Change{
			ID: fmt.Sprintf("chg-%d", i), AppID: "app-1", Environment: "production", Resource: "release", Action: "promote",
			Subject: fmt.Sprintf("release:%d@production/stable", i+1), ContentHash: hash, ChangedBy: "usr-dev", ChangedAt: utc(2026, 10, 2, 8, i%60),
		}
		switch i % 3 {
		case 0:
			c.Author = "usr-author"
			c.Approvals = []governance.Approval{
				{DeciderID: "usr-a", Decider: "Ada", Decision: "approve", DecidedAt: utc(2026, 10, 2, 7, 0), AuditSequence: 41, KeyID: "k1", Signature: "c2ln"},
				{DeciderID: "usr-b", Decider: "Bo", Decision: "approve", DecidedAt: utc(2026, 10, 2, 7, 5), AuditSequence: 42},
			}
			c.Evidence = []governance.Evidence{{Kind: "approvalRequest", Ref: "req-1", Link: "/apps/app-1/approvals/req-1"}}
		case 1:
			c.Resource, c.Action, c.Subject = "breakglass", "killSwitch", "kill switch plugin loans on stable"
			c.BreakGlass = &governance.BreakGlass{Justification: "loans plugin crashes on launch (see INC-7)", ReviewDueAt: utc(2026, 10, 3, 8, 0), ReviewStatus: "open"}
			c.Evidence = []governance.Evidence{{Kind: "breakGlass", Ref: "bg-1"}}
		}
		r.Changes = append(r.Changes, c)
	}
	return r
}

// sampleEnvelope returns an envelope over the statement "{}" with a signature
// the document only displays.
func sampleEnvelope() governance.Envelope {
	return governance.NewEnvelope([]byte("{}"), "audit-key-1", bytes.Repeat([]byte{0xAB}, 64))
}

// pdfInfo is what the structural checks read from a document.
type pdfInfo struct {
	objects map[int]string
	size    int
	pages   int
	text    string
}

var (
	startxrefRE = regexp.MustCompile(`startxref\n(\d+)\n%%EOF\n$`)
	objectRE    = regexp.MustCompile(`(?s)(\d+) 0 obj\n(.*?)\nendobj\n`)
)

// parsePDF checks the structure of a document and returns what it holds: the
// header, the cross-reference table with every offset pointing at its object,
// the trailer, the page tree and each stream's length.
func parsePDF(t *testing.T, doc []byte) pdfInfo {
	t.Helper()
	text := string(doc)
	if !strings.HasPrefix(text, "%PDF-1.7\n") {
		t.Fatalf("header = %q", text[:12])
	}
	m := startxrefRE.FindStringSubmatch(text)
	if m == nil {
		t.Fatal("no startxref and end-of-file marker at the end")
	}
	xref, _ := strconv.Atoi(m[1])
	if !strings.HasPrefix(text[xref:], "xref\n0 ") {
		t.Fatalf("startxref %d does not point at the table: %q", xref, text[xref:xref+12])
	}
	info := pdfInfo{objects: map[int]string{}, text: text}
	for _, o := range objectRE.FindAllStringSubmatch(text, -1) {
		id, _ := strconv.Atoi(o[1])
		info.objects[id] = o[2]
	}
	checkTable(t, text, xref, &info)
	checkPages(t, &info)
	checkStreams(t, &info)
	return info
}

// checkTable verifies the cross-reference table and the trailer.
func checkTable(t *testing.T, text string, xref int, info *pdfInfo) {
	t.Helper()
	var n int
	if _, err := fmt.Sscanf(text[xref:], "xref\n0 %d\n", &n); err != nil {
		t.Fatalf("the table's size: %v", err)
	}
	info.size = n
	lines := strings.SplitAfter(text[xref:], "\n")
	if lines[2] != "0000000000 65535 f \n" {
		t.Errorf("the free entry = %q", lines[2])
	}
	for id := 1; id < n; id++ {
		entry := lines[2+id]
		if len(entry) != 20 || !strings.HasSuffix(entry, " 00000 n \n") {
			t.Fatalf("entry %d = %q is not 20 bytes", id, entry)
		}
		off, _ := strconv.Atoi(entry[:10])
		if !strings.HasPrefix(text[off:], fmt.Sprintf("%d 0 obj\n", id)) {
			t.Errorf("entry %d points at %q", id, text[off:off+12])
		}
	}
	trailer := text[xref+strings.Index(text[xref:], "trailer"):]
	if !strings.Contains(trailer, fmt.Sprintf("/Size %d", n)) || !strings.Contains(trailer, "/Root 1 0 R") {
		t.Errorf("trailer = %q", trailer)
	}
	if len(info.objects) != n-1 {
		t.Errorf("%d objects, but the table lists %d", len(info.objects), n-1)
	}
}

// checkPages verifies the catalogue and that the page tree counts the pages.
func checkPages(t *testing.T, info *pdfInfo) {
	t.Helper()
	if !strings.Contains(info.objects[1], "/Type /Catalog") || !strings.Contains(info.objects[1], "/Pages 2 0 R") {
		t.Errorf("catalogue = %q", info.objects[1])
	}
	m := regexp.MustCompile(`/Count (\d+)`).FindStringSubmatch(info.objects[2])
	if m == nil {
		t.Fatalf("page tree = %q", info.objects[2])
	}
	info.pages, _ = strconv.Atoi(m[1])
	leaves := 0
	for _, body := range info.objects {
		if strings.HasPrefix(body, "<< /Type /Page ") {
			leaves++
			if !strings.Contains(body, "/Parent 2 0 R") || !strings.Contains(body, "/MediaBox [0 0 595 842]") {
				t.Errorf("page = %q", body)
			}
		}
	}
	if leaves != info.pages || strings.Count(info.objects[2], " 0 R") != info.pages {
		t.Errorf("the tree counts %d pages, the file has %d and lists %d", info.pages, leaves, strings.Count(info.objects[2], " 0 R"))
	}
}

// checkStreams verifies that every stream's declared length is its length.
func checkStreams(t *testing.T, info *pdfInfo) {
	t.Helper()
	for id, body := range info.objects {
		start := strings.Index(body, "stream\n")
		if start < 0 {
			continue
		}
		m := regexp.MustCompile(`/Length (\d+)`).FindStringSubmatch(body)
		declared, _ := strconv.Atoi(m[1])
		content := strings.TrimSuffix(body[start+len("stream\n"):], "endstream")
		if len(content) != declared {
			t.Errorf("object %d declares %d bytes and holds %d", id, declared, len(content))
		}
	}
}

// Verifies: GOV-026.
// The PDF is a well-formed PDF 1.7 file: header, objects, a cross-reference
// table whose offsets are right, a trailer and a page tree that counts its
// pages; it lists every change with hash, decisions and evidence and ends with
// a page of signatures.
func TestReports_PDFStructure_GOV_026(t *testing.T) {
	t.Parallel()
	doc, err := Reports{}.PDF(licensed(edition.StateValid), sampleReport(3), sampleEnvelope())
	if err != nil {
		t.Fatal(err)
	}
	info := parsePDF(t, doc)
	if info.pages != 2 {
		t.Errorf("pages = %d, want the changes and the signatures", info.pages)
	}
	for _, want := range []string{
		"Compliance report", "release promote: release:1@production/stable", "\\(production\\)", strings.Repeat("ab12", 16),
		"approve: usr-a", "approve: usr-b", "none asked for", "evidence approvalRequest:req-1", "breakGlass:bg-1",
		"Break-glass: loans plugin crashes on launch \\(see INC-7\\)", "written by",
		"Signatures", "Signature 1, key audit-key-1", "Statement SHA-256", "page 1 of 2", "page 2 of 2",
	} {
		if !strings.Contains(info.text, want) {
			t.Errorf("the document lacks %q", want)
		}
	}
}

// Verifies: GOV-026.
// A long report flows over many pages, each a valid page of the tree, and a
// report of no changes says so.
func TestReports_PDFPagination_GOV_026(t *testing.T) {
	t.Parallel()
	doc, err := Reports{}.PDF(licensed(edition.StateValid), sampleReport(200), sampleEnvelope())
	if err != nil {
		t.Fatal(err)
	}
	if info := parsePDF(t, doc); info.pages < 8 {
		t.Errorf("200 changes took %d pages", info.pages)
	}
	empty, err := Reports{}.PDF(licensed(edition.StateValid), sampleReport(0), sampleEnvelope())
	if err != nil {
		t.Fatal(err)
	}
	if info := parsePDF(t, empty); !strings.Contains(info.text, "No production change was made in this period.") {
		t.Error("an empty report does not say so")
	}
}

// Verifies: GOV-026.
// Text is written as PDF strings: delimiters are escaped, characters outside
// WinAnsiEncoding become question marks and the file stays ASCII apart from
// its binary marker.
func TestReports_PDFText_GOV_026(t *testing.T) {
	t.Parallel()
	r := sampleReport(1)
	r.Changes[0].Subject = `a(b)c\d é 日本 – ok`
	doc, err := Reports{}.PDF(licensed(edition.StateValid), r, sampleEnvelope())
	if err != nil {
		t.Fatal(err)
	}
	parsePDF(t, doc)
	text := string(doc)
	for _, want := range []string{`a\(b\)c\\d \351 ?? \226 ok`} {
		if !strings.Contains(text, want) {
			t.Errorf("the document lacks %q", want)
		}
	}
	for i, b := range doc[len("%PDF-1.7\n%\xE2\xE3\xCF\xD3\n"):] {
		if b > 0x7E || (b < 0x20 && b != '\n') {
			t.Fatalf("byte %d after the header is %#x, not ASCII", i, b)
		}
	}
}

// Verifies: GOV-026.
// The same report and envelope give the same bytes.
func TestReports_PDFIsDeterministic_GOV_026(t *testing.T) {
	t.Parallel()
	a, err := Reports{}.PDF(licensed(edition.StateValid), sampleReport(10), sampleEnvelope())
	if err != nil {
		t.Fatal(err)
	}
	b, err := Reports{}.PDF(licensed(edition.StateValid), sampleReport(10), sampleEnvelope())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a, b) {
		t.Error("two renderings of the same report differ")
	}
}

// Verifies: GOV-026, GOV-030.
// The report writer needs the licence for reading; an expired licence still
// reads, and a missing or invalid one refuses.
func TestReports_PDFLicence_GOV_030(t *testing.T) {
	t.Parallel()
	if _, err := (Reports{}).PDF(context.Background(), sampleReport(1), sampleEnvelope()); err == nil {
		t.Error("PDF without a licence succeeded")
	}
	for _, state := range []edition.State{edition.StateAbsent, edition.StateInvalid} {
		if _, err := (Reports{}).PDF(licensed(state), sampleReport(1), sampleEnvelope()); err == nil {
			t.Errorf("PDF with a %s licence succeeded", state)
		}
	}
	if _, err := (Reports{}).PDF(licensed(edition.StateExpired), sampleReport(1), sampleEnvelope()); err != nil {
		t.Errorf("PDF with an expired licence: %v", err)
	}
	broken := sampleEnvelope()
	broken.Payload = "not base64!"
	if _, err := (Reports{}).PDF(licensed(edition.StateValid), sampleReport(1), broken); err == nil {
		t.Error("PDF of an envelope whose payload is not base64 succeeded")
	}
}

// Verifies: GOV-026.
// Wrapping never exceeds the width, breaks an unbreakable word where it
// overflows and loses no text.
func TestWrap_GOV_026(t *testing.T) {
	t.Parallel()
	const width = 60.0
	for _, s := range []string{
		"a short line", "the quick brown fox jumps over the lazy dog and keeps running", strings.Repeat("x", 90),
		"mixed " + strings.Repeat("W", 40) + " words", "",
	} {
		lines := wrap(fontRegular, bodySize, width, s)
		if len(lines) == 0 {
			t.Errorf("wrap(%q) is empty", s)
		}
		for _, l := range lines {
			if w := textWidth(fontRegular, bodySize, l); w > width {
				t.Errorf("a line of %q is %.1f points wide in %.1f: %q", s, w, width, l)
			}
		}
		if strings.Join(strings.Fields(strings.Join(lines, " ")), "") != strings.Join(strings.Fields(s), "") {
			t.Errorf("wrap(%q) lost text: %q", s, lines)
		}
	}
	if got := textWidth(fontMono, 10, "abcd"); got != 24 {
		t.Errorf("four Courier characters at 10 points = %v, want 24", got)
	}
}

// Verifies: GOV-026.
// An instant is rendered in UTC whatever its zone.
func TestStampIsUTC_GOV_026(t *testing.T) {
	t.Parallel()
	berlin := time.FixedZone("CET", 3600)
	if got := stamp(time.Date(2026, 3, 1, 10, 0, 0, 0, berlin)); got != "2026-03-01 09:00:00" {
		t.Errorf("stamp = %q", got)
	}
}
