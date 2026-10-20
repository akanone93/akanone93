// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

package saml

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/beevik/etree"
)

// The namespaces of the documents this package reads.
const (
	nsProtocol  = "urn:oasis:names:tc:SAML:2.0:protocol"
	nsAssertion = "urn:oasis:names:tc:SAML:2.0:assertion"
	nsMetadata  = "urn:oasis:names:tc:SAML:2.0:metadata"
	nsSignature = "http://www.w3.org/2000/09/xmldsig#"
	nsEncrypt   = "http://www.w3.org/2001/04/xmlenc#"
)

// errTooDeep reports a document nested beyond what a SAML message needs.
var errTooDeep = errors.New("the document is nested too deeply")

// maxDepth bounds the nesting of a document: a SAML response is a dozen
// levels deep, and an encrypted one a few more.
const maxDepth = 64

// parseXML reads a document, refusing what Go's decoder would read
// differently from a stricter one (xml-roundtrip validation), an attribute
// repeated on one element, comments, and absurd nesting.
func parseXML(data []byte) (*etree.Element, error) {
	doc := etree.NewDocument()
	doc.ReadSettings.ValidateInput = true
	doc.ReadSettings.PreserveDuplicateAttrs = true
	if err := doc.ReadFromBytes(data); err != nil {
		return nil, fmt.Errorf("read the document: %w", err)
	}
	root := doc.Root()
	if root == nil {
		return nil, errors.New("the document has no root element")
	}
	if err := checkShape(root, 0); err != nil {
		return nil, err
	}
	return root, nil
}

// checkShape refuses a comment (the classic way to split a name in two so
// that two readers see different ones), a repeated attribute and a document
// that nests beyond maxDepth.
func checkShape(el *etree.Element, depth int) error {
	if depth > maxDepth {
		return errTooDeep
	}
	seen := make(map[string]bool, len(el.Attr))
	for _, a := range el.Attr {
		key := a.Space + ":" + a.Key
		if seen[key] {
			return fmt.Errorf("the attribute %q is repeated on <%s>", a.FullKey(), el.Tag)
		}
		seen[key] = true
	}
	for _, t := range el.Child {
		switch c := t.(type) {
		case *etree.Comment:
			return errors.New("the document contains a comment")
		case *etree.Element:
			if err := checkShape(c, depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}

// is reports whether el is the element tag of namespace ns.
func is(el *etree.Element, ns, tag string) bool {
	return el.Tag == tag && el.NamespaceURI() == ns
}

// children returns the direct children of el named tag in namespace ns.
func children(el *etree.Element, ns, tag string) []*etree.Element {
	var out []*etree.Element
	for _, c := range el.ChildElements() {
		if is(c, ns, tag) {
			out = append(out, c)
		}
	}
	return out
}

// optional returns the only child of el named tag in namespace ns, nil when
// there is none and an error when there are several.
func optional(el *etree.Element, ns, tag string) (*etree.Element, error) {
	found := children(el, ns, tag)
	switch len(found) {
	case 0:
		return nil, nil
	case 1:
		return found[0], nil
	default:
		return nil, fmt.Errorf("<%s> has %d <%s> children, not one", el.Tag, len(found), tag)
	}
}

// required is optional for a child that must be there.
func required(el *etree.Element, ns, tag string) (*etree.Element, error) {
	c, err := optional(el, ns, tag)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, fmt.Errorf("<%s> has no <%s>", el.Tag, tag)
	}
	return c, nil
}

// text returns the text of an element, whole: a text node split by a CDATA
// section is joined, and surrounding space is dropped.
func text(el *etree.Element) string {
	var b strings.Builder
	for _, t := range el.Child {
		if cd, ok := t.(*etree.CharData); ok {
			b.WriteString(cd.Data)
		}
	}
	return strings.TrimSpace(b.String())
}

// timeAttr reads an xs:dateTime attribute; the zero time is an absent one.
func timeAttr(el *etree.Element, name string) (time.Time, error) {
	v := el.SelectAttrValue(name, "")
	if v == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse(time.RFC3339Nano, v)
	if err != nil {
		return time.Time{}, fmt.Errorf("the %s of <%s> is not a time: %w", name, el.Tag, err)
	}
	return t, nil
}

// decodeBase64 decodes base64 that may be wrapped over lines, as PEM and
// metadata documents wrap it.
func decodeBase64(s string) ([]byte, error) {
	clean := strings.Map(func(r rune) rune {
		if r == ' ' || r == '\n' || r == '\r' || r == '\t' {
			return -1
		}
		return r
	}, s)
	return base64.StdEncoding.DecodeString(clean)
}
