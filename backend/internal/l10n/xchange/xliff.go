// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package xchange

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"strconv"
	"strings"
)

// XLIFF 2.0 (OASIS, urn:oasis:names:tc:xliff:document:2.0). One <file
// id="app"> holds one <unit> per translation key: id is the key identifier,
// name the key. Notes carry the key's description and maximum length. The
// unit's single <segment> has the review state and the <source> and
// <target> texts.

const (
	xliffNamespace = "urn:oasis:names:tc:xliff:document:2.0"
	xliffFileID    = "app"
	// xliffMaxDepth bounds element nesting; a real file nests four deep.
	xliffMaxDepth = 16
)

// xliffStates maps the review states to those of XLIFF 2.0 and back.
var (
	toXLIFFState = map[string]string{
		stateNew: "initial", stateTranslated: "translated", stateReviewed: "reviewed", stateApproved: "final",
	}
	fromXLIFFState = map[string]string{
		"initial": stateNew, "translated": stateTranslated, "reviewed": stateReviewed, "final": stateApproved,
	}
)

// exportXLIFF writes an XLIFF 2.0 document.
func exportXLIFF(keys []Key, source, target Doc) ([]byte, error) {
	var b bytes.Buffer
	b.WriteString(xml.Header)
	b.WriteString(`<xliff xmlns="` + xliffNamespace + `" version="2.0" srcLang="`)
	xmlText(&b, source.Locale)
	b.WriteString(`" trgLang="`)
	xmlText(&b, target.Locale)
	b.WriteString("\">\n  <file id=\"" + xliffFileID + "\">\n")
	for _, k := range keys {
		b.WriteString(`    <unit id="`)
		xmlText(&b, k.ID)
		b.WriteString(`" name="`)
		xmlText(&b, k.Key)
		b.WriteString("\">\n")
		if k.Description != "" || k.MaxLength > 0 {
			b.WriteString("      <notes>\n")
			if k.Description != "" {
				b.WriteString(`        <note id="description" category="description">`)
				xmlText(&b, k.Description)
				b.WriteString("</note>\n")
			}
			if k.MaxLength > 0 {
				b.WriteString(`        <note id="maxLength" category="maxLength">` + strconv.Itoa(k.MaxLength) + "</note>\n")
			}
			b.WriteString("      </notes>\n")
		}
		text, translated := target.Messages[k.ID]
		state := toXLIFFState[stateNew]
		if translated {
			state = toXLIFFState[target.State(k.ID)]
		}
		b.WriteString(`      <segment state="` + state + "\">\n        <source>")
		xmlText(&b, source.Messages[k.ID])
		b.WriteString("</source>\n")
		if translated {
			b.WriteString("        <target>")
			xmlText(&b, text)
			b.WriteString("</target>\n")
		}
		b.WriteString("      </segment>\n    </unit>\n")
	}
	b.WriteString("  </file>\n</xliff>\n")
	return b.Bytes(), nil
}

// xmlText writes s escaped for element content and attribute values.
func xmlText(b *bytes.Buffer, s string) {
	_ = xml.EscapeText(b, []byte(s)) // a bytes.Buffer does not fail
}

// parseXLIFF reads an XLIFF 2.0 document. The decoder is strict, accepts
// only UTF-8, knows only the five predefined entities and is never given a
// way to fetch anything; a document type declaration or an entity
// declaration is refused outright.
func parseXLIFF(data []byte) (importFile, error) {
	if hasDeclaration(data) {
		return importFile{}, invalid("an XLIFF file may not carry a DOCTYPE or ENTITY declaration")
	}
	d := xml.NewDecoder(bytes.NewReader(data))
	d.Strict = true
	d.Entity = nil
	d.CharsetReader = nil

	x := &xliffReader{d: d}
	for {
		tok, err := d.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return importFile{}, invalid("the file is not well-formed XML: %s", clip(err.Error()))
		}
		switch t := tok.(type) {
		case xml.Directive:
			return importFile{}, invalid("an XLIFF file may not carry a DOCTYPE or ENTITY declaration")
		case xml.StartElement:
			if err := x.start(t); err != nil {
				return importFile{}, err
			}
		case xml.CharData:
			if x.text != nil {
				x.text.Write(t)
			}
		case xml.EndElement:
			if err := x.end(); err != nil {
				return importFile{}, err
			}
		}
	}
	if !x.sawRoot {
		return importFile{}, invalid("the file has no <xliff> element")
	}
	return x.out, nil
}

// xliffReader is the state of one parseXLIFF pass: the open elements, the
// unit being read and the text of the source or target being collected.
type xliffReader struct {
	d       *xml.Decoder
	out     importFile
	stack   []string
	cur     *entry
	segment int
	text    *strings.Builder
	sawRoot bool
}

// start handles an opening element.
func (x *xliffReader) start(t xml.StartElement) error {
	if len(x.stack) >= xliffMaxDepth {
		return invalid("the file nests elements deeper than %d levels", xliffMaxDepth)
	}
	name := t.Name.Local
	if len(x.stack) == 0 {
		if err := checkRoot(t); err != nil {
			return err
		}
		x.out.locale = attr(t, "trgLang")
		x.sawRoot = true
	} else if t.Name.Space != xliffNamespace {
		return invalid("element %s is not in the XLIFF 2.0 namespace", clip(name))
	}
	if x.text != nil {
		return invalid("<%s> holds a nested element; messages are plain text", x.stack[len(x.stack)-1])
	}
	x.stack = append(x.stack, name)
	switch {
	case name == "unit" && parent(x.stack) != "":
		if x.cur != nil {
			return invalid("units may not nest")
		}
		x.cur = &entry{id: attr(t, "id"), key: attr(t, "name")}
		x.segment = 0
	case name == "segment" && x.cur != nil:
		return x.startSegment(t)
	case (name == "source" || name == "target") && x.cur != nil && parent(x.stack) == "segment":
		x.text = &strings.Builder{}
	case isContainer(name):
	default:
		// Anything else, such as <ignorable> or <originalData>, is not a
		// translation; skip it whole.
		if err := x.d.Skip(); err != nil {
			return invalid("the file is not well-formed XML: %s", clip(err.Error()))
		}
		x.stack = x.stack[:len(x.stack)-1]
	}
	return nil
}

// isContainer reports whether name holds units or notes and is descended into
// without being read.
func isContainer(name string) bool {
	switch name {
	case "xliff", "file", "group", "unit", "notes", "note", "segment":
		return true
	}
	return false
}

// startSegment handles a <segment>: one per unit, with an optional state.
func (x *xliffReader) startSegment(t xml.StartElement) error {
	x.segment++
	if x.segment > 1 {
		return invalid("unit %s has more than one segment; export with segmentation off", clip(x.cur.id))
	}
	if s := attr(t, "state"); s != "" {
		state, ok := fromXLIFFState[s]
		if !ok {
			return invalid("unit %s has the unknown state %q", clip(x.cur.id), clip(s))
		}
		x.cur.state = state
	}
	return nil
}

// end handles a closing element.
func (x *xliffReader) end() error {
	name := x.stack[len(x.stack)-1]
	x.stack = x.stack[:len(x.stack)-1]
	switch {
	case x.text != nil && (name == "source" || name == "target"):
		if name == "source" {
			x.cur.source, x.cur.hasSource = x.text.String(), true
		} else {
			x.cur.target, x.cur.hasTarget = x.text.String(), true
		}
		x.text = nil
	case name == "unit" && x.cur != nil:
		if x.cur.id == "" && x.cur.key == "" {
			return invalid("a unit has neither an id nor a name")
		}
		x.out.entries = append(x.out.entries, *x.cur)
		x.cur = nil
	}
	return nil
}

// parent returns the element above the innermost one of a stack.
func parent(stack []string) string {
	if len(stack) < 2 {
		return ""
	}
	return stack[len(stack)-2]
}

// checkRoot checks the <xliff> element: XLIFF 2.0 in its namespace.
func checkRoot(t xml.StartElement) error {
	if t.Name.Local != "xliff" {
		return invalid("the root element is <%s>, not <xliff>", clip(t.Name.Local))
	}
	if t.Name.Space != xliffNamespace {
		return invalid("the file is not XLIFF 2.0 (namespace %s)", clip(t.Name.Space))
	}
	if v := attr(t, "version"); v != "2.0" {
		return invalid("the file is XLIFF version %s; only 2.0 is supported", clip(v))
	}
	return nil
}

// attr returns the value of an attribute without a namespace.
func attr(t xml.StartElement, name string) string {
	for _, a := range t.Attr {
		if a.Name.Space == "" && a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

// hasDeclaration reports whether the text holds a DOCTYPE or ENTITY
// declaration, in any case. Legitimate content cannot: its "<" is escaped.
func hasDeclaration(data []byte) bool {
	upper := bytes.ToUpper(data)
	return bytes.Contains(upper, []byte("<!DOCTYPE")) || bytes.Contains(upper, []byte("<!ENTITY"))
}
