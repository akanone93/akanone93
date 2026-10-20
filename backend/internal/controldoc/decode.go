// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package controldoc

import (
	"fmt"
	"slices"
	"time"

	flatbuffers "github.com/google/flatbuffers/go"

	"github.com/nightCode42/plux3/backend/internal/controldoc/controlfbs"
)

// Decode reads an encoded document and validates it. A buffer that does
// not carry the file identifier, that reads outside itself, or that holds
// a document the schema's rules refuse, is ErrMalformed or ErrInvalid.
// Unknown kill switch scopes are an error here: the server only decodes
// what it wrote, and a device skips them in its own reader.
func Decode(buf []byte) (d Document, err error) {
	if len(buf) < flatbuffers.SizeUOffsetT+len(Identifier) || !controlfbs.ControlDocumentBufferHasIdentifier(buf) {
		return Document{}, fmt.Errorf("controldoc.Decode: %w: no PLXC identifier", ErrMalformed)
	}
	// Slicing past len(buf) but within its capacity would read bytes the
	// caller never passed, so the capacity is cut to the length.
	buf = slices.Clip(buf)
	// The generated accessors index the buffer without a bounds check, so a
	// buffer that points outside itself panics. That is converted to the
	// error it means; nothing else is recovered.
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(error); !ok {
				panic(r)
			}
			d, err = Document{}, fmt.Errorf("controldoc.Decode: %w: reads outside the buffer", ErrMalformed)
		}
	}()
	root := controlfbs.GetRootAsControlDocument(buf, 0)
	d = Document{
		Version: root.Version(), AppID: string(root.AppId()), Environment: string(root.Environment()), Channel: string(root.Channel()),
		IssuedAt: time.Unix(root.IssuedAt(), 0).UTC(), ExpiresAt: time.Unix(root.ExpiresAt(), 0).UTC(),
	}
	if d.KillSwitches, err = readSwitches(root); err != nil {
		return Document{}, err
	}
	if m := root.MandatoryUpdate(nil); m != nil {
		d.MandatoryUpdate = &MandatoryUpdate{ReleaseID: string(m.ReleaseId()), ReleaseSequence: m.ReleaseSequence(), MessageKey: string(m.MessageKey())}
	}
	if r := root.ForcedRollback(nil); r != nil {
		d.ForcedRollback = &ForcedRollback{ReleaseID: string(r.ReleaseId()), ReleaseSequence: r.ReleaseSequence()}
	}
	if err := d.Validate(); err != nil {
		return Document{}, fmt.Errorf("controldoc.Decode: %w", err)
	}
	return d, nil
}

// readSwitches reads the kill switch vector.
func readSwitches(root *controlfbs.ControlDocument) ([]KillSwitch, error) {
	n := root.KillSwitchesLength()
	if n > len(root.Table().Bytes) {
		return nil, fmt.Errorf("controldoc.Decode: %w: %d kill switches in a buffer of %d bytes", ErrMalformed, n, len(root.Table().Bytes))
	}
	byWire := make(map[controlfbs.SwitchScope]Scope, len(wireScopes))
	for scope, wire := range wireScopes {
		byWire[wire] = scope
	}
	out := make([]KillSwitch, 0, n)
	var k controlfbs.KillSwitch
	for i := range n {
		if !root.KillSwitches(&k, i) {
			break
		}
		scope, ok := byWire[k.Scope()]
		if !ok {
			return nil, fmt.Errorf("controldoc.Decode: %w: kill switch scope %s", ErrInvalid, k.Scope())
		}
		out = append(out, KillSwitch{Scope: scope, Target: string(k.Target())})
	}
	return out, nil
}
