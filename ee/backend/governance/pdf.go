// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

package governance

import (
	"bytes"
	"fmt"
	"strings"
)

// The page: A4 in points, with a margin all round and a strip below the
// text for the page number.
const (
	pageWidth  = 595.0
	pageHeight = 842.0
	margin     = 50.0
	footerRoom = 24.0
)

// font is one of the three standard fonts every PDF reader has, so that the
// document embeds none.
type font int

const (
	fontRegular font = iota
	fontBold
	fontMono
)

// baseFonts are the PDF names of the fonts, in font order; resource names are
// F1, F2 and F3.
var baseFonts = [...]string{"Helvetica", "Helvetica-Bold", "Courier"}

// helveticaWidths are the advance widths, in thousandths of the font size, of
// the printable ASCII characters from the space to the tilde in Helvetica,
// from Adobe's font metrics. They are only used to wrap text, so the bold
// face, a little wider, is measured with them too.
var helveticaWidths = [...]int{
	278, 278, 355, 556, 556, 889, 667, 191, 333, 333, 389, 584, 278, 333, 278, 278, // space to /
	556, 556, 556, 556, 556, 556, 556, 556, 556, 556, 278, 278, 584, 584, 584, 556, // 0 to ?
	1015, 667, 667, 722, 722, 667, 611, 778, 722, 278, 500, 667, 556, 833, 722, 778, // @ to O
	667, 778, 722, 667, 611, 722, 667, 944, 667, 667, 611, 278, 278, 278, 469, 556, // P to _
	333, 556, 556, 500, 556, 556, 278, 556, 556, 222, 222, 500, 222, 833, 556, 556, // ` to o
	556, 556, 333, 500, 278, 556, 500, 722, 500, 500, 500, 334, 260, 334, 584, // p to ~
}

// Advances of the characters that are not printable ASCII, and of Courier.
const (
	otherWidth = 556
	monoWidth  = 600
)

// textWidth returns the width in points of s set in f at size.
func textWidth(f font, size float64, s string) float64 {
	total := 0
	for _, r := range s {
		switch {
		case f == fontMono:
			total += monoWidth
		case r >= ' ' && r <= '~':
			total += helveticaWidths[r-' ']
		default:
			total += otherWidth
		}
	}
	return float64(total) * size / 1000
}

// wrap breaks s into lines no wider than width in f at size, at spaces; a
// word wider than a line is broken where it overflows.
func wrap(f font, size, width float64, s string) []string {
	var lines []string
	line := ""
	for _, word := range strings.Fields(s) {
		for textWidth(f, size, word) > width {
			head := fit(f, size, width, word)
			if line != "" {
				lines, line = append(lines, line), ""
			}
			lines, word = append(lines, head), word[len(head):]
		}
		switch {
		case line == "":
			line = word
		case textWidth(f, size, line+" "+word) <= width:
			line += " " + word
		default:
			lines, line = append(lines, line), word
		}
	}
	if line != "" || len(lines) == 0 {
		lines = append(lines, line)
	}
	return lines
}

// fit returns the longest prefix of word, at least one character, that fits
// in width.
func fit(f font, size, width float64, word string) string {
	end := 0
	for i, r := range word {
		if i > 0 && textWidth(f, size, word[:i+len(string(r))]) > width {
			break
		}
		end = i + len(string(r))
	}
	return word[:end]
}

// pdfPage is the drawing operators of one page.
type pdfPage struct{ ops bytes.Buffer }

// pdfWriter lays text out on pages and writes them as a PDF 1.7 file with
// the standard fonts. It writes no date, so that the same document is the
// same bytes.
type pdfWriter struct {
	title string
	pages []*pdfPage
	// y is the baseline the next line is set on, measured up from the
	// bottom of the current page.
	y float64
}

// newPDFWriter starts a document with a first page.
func newPDFWriter(title string) *pdfWriter {
	w := &pdfWriter{title: title}
	w.newPage()
	return w
}

// newPage starts a page and moves to its top.
func (w *pdfWriter) newPage() {
	w.pages = append(w.pages, &pdfPage{})
	w.y = pageHeight - margin
}

// room starts a new page unless height fits above the bottom margin.
func (w *pdfWriter) room(height float64) {
	if w.y-height < margin+footerRoom {
		w.newPage()
	}
}

// line sets s at x on the current baseline, then moves down by leading.
func (w *pdfWriter) line(f font, size, x, leading float64, s string) {
	w.room(leading)
	w.at(f, size, x, w.y-size, s)
	w.y -= leading
}

// at sets s with its baseline at the given position of the current page.
func (w *pdfWriter) at(f font, size, x, y float64, s string) {
	page := w.pages[len(w.pages)-1]
	fmt.Fprintf(&page.ops, "BT /F%d %.1f Tf %.2f %.2f Td %s Tj ET\n", int(f)+1, size, x, y, literal(s))
}

// rule draws a horizontal line across the text area at the current position
// and moves down by gap.
func (w *pdfWriter) rule(gap float64) {
	w.room(gap)
	page := w.pages[len(w.pages)-1]
	fmt.Fprintf(&page.ops, "0.5 w %.2f %.2f m %.2f %.2f l S\n", margin, w.y, pageWidth-margin, w.y)
	w.y -= gap
}

// literal encodes s as a PDF string in WinAnsiEncoding, the encoding the
// fonts are declared with. A character the encoding lacks is a question
// mark; every byte outside printable ASCII is written as an octal escape, so
// that the file stays ASCII.
func literal(s string) string {
	var b strings.Builder
	b.WriteByte('(')
	for _, r := range s {
		c, ok := winAnsi(r)
		switch {
		case !ok:
			b.WriteByte('?')
		case c == '(' || c == ')' || c == '\\':
			b.WriteByte('\\')
			b.WriteByte(c)
		case c < ' ' || c > '~':
			fmt.Fprintf(&b, "\\%03o", c)
		default:
			b.WriteByte(c)
		}
	}
	b.WriteByte(')')
	return b.String()
}

// punctuation maps the typographic characters of Windows-1252 that are not in
// Latin-1 to their codes.
var punctuation = map[rune]byte{'€': 0x80, '‘': 0x91, '’': 0x92, '“': 0x93, '”': 0x94, '•': 0x95, '–': 0x96, '—': 0x97}

// winAnsi returns the WinAnsiEncoding code of r.
func winAnsi(r rune) (byte, bool) {
	switch {
	case r >= ' ' && r <= '~', r >= 0xA0 && r <= 0xFF:
		return byte(r & 0xFF), true
	case r == '\t' || r == '\n':
		return ' ', true
	}
	c, ok := punctuation[r]
	return c, ok
}

// render assembles the document. footer returns the text at the foot of page n
// of total.
func (w *pdfWriter) render(footer func(n, total int) string) []byte {
	total := len(w.pages)
	for i := range w.pages {
		page := w.pages[i]
		fmt.Fprintf(&page.ops, "BT /F1 8.0 Tf %.2f %.2f Td %s Tj ET\n", margin, margin-4, literal(footer(i+1, total)))
	}
	return assemble(w.title, w.pages)
}

// The fixed objects of the file: the catalogue, the page tree and the three
// fonts, then each page and its content, then the information dictionary.
const (
	catalogID   = 1
	pageTreeID  = 2
	firstFontID = 3
	firstPageID = firstFontID + len(baseFonts)
)

// assemble writes the objects, the cross-reference table and the trailer.
func assemble(title string, pages []*pdfPage) []byte {
	var out bytes.Buffer
	out.WriteString("%PDF-1.7\n%\xE2\xE3\xCF\xD3\n")
	offsets := map[int]int{}
	object := func(id int, body string) {
		offsets[id] = out.Len()
		fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", id, body)
	}
	infoID := firstPageID + 2*len(pages)
	object(catalogID, fmt.Sprintf("<< /Type /Catalog /Pages %d 0 R >>", pageTreeID))
	kids := make([]string, len(pages))
	for i := range pages {
		kids[i] = fmt.Sprintf("%d 0 R", firstPageID+2*i)
	}
	object(pageTreeID, fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), len(pages)))
	for i, name := range baseFonts {
		object(firstFontID+i, fmt.Sprintf("<< /Type /Font /Subtype /Type1 /BaseFont /%s /Encoding /WinAnsiEncoding >>", name))
	}
	resources := fmt.Sprintf("<< /Font << /F1 %d 0 R /F2 %d 0 R /F3 %d 0 R >> >>", firstFontID, firstFontID+1, firstFontID+2)
	for i, page := range pages {
		id := firstPageID + 2*i
		object(id, fmt.Sprintf("<< /Type /Page /Parent %d 0 R /MediaBox [0 0 %.0f %.0f] /Resources %s /Contents %d 0 R >>", pageTreeID, pageWidth, pageHeight, resources, id+1))
		object(id+1, fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", page.ops.Len(), page.ops.String()))
	}
	object(infoID, fmt.Sprintf("<< /Title %s /Producer (Plux) >>", literal(title)))
	return trailer(&out, offsets, infoID)
}

// trailer appends the cross-reference table and the trailer to the objects
// written so far and returns the file. Every entry of the table is twenty
// bytes, as the format requires.
func trailer(out *bytes.Buffer, offsets map[int]int, infoID int) []byte {
	size := infoID + 1
	start := out.Len()
	fmt.Fprintf(out, "xref\n0 %d\n0000000000 65535 f \n", size)
	for id := 1; id < size; id++ {
		fmt.Fprintf(out, "%010d 00000 n \n", offsets[id])
	}
	fmt.Fprintf(out, "trailer\n<< /Size %d /Root %d 0 R /Info %d 0 R >>\nstartxref\n%d\n%%%%EOF\n", size, catalogID, infoID, start)
	return out.Bytes()
}
