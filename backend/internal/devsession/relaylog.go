// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package devsession

import (
	"encoding/binary"
	"errors"
	"fmt"

	"google.golang.org/protobuf/proto"

	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// The records of a session's logs on the shared layer. An event record is
// the device's ID, the device's own sequence and the event, which carries
// the session sequence: the session sequence is the watchers' cursor, the
// device sequence what a device's resent events are checked against. A
// frame record is a frame for a device, carrying that device's frame
// number; a device's hello, with the last frame it carried out; or the
// session's end with its code.

// recordKind tells the frame records apart.
type recordKind byte

const (
	recordFrame recordKind = 'f'
	recordHello recordKind = 'h'
	recordEnd   recordKind = 'e'
)

// errRecord reports a log entry that is not a record of the relay.
var errRecord = errors.New("devsession: the log entry is not a relay record")

// frameEntry is a decoded frame record.
type frameEntry struct {
	kind   recordKind
	device string
	frame  *pluxv1.DevServerFrame
	done   uint64
	code   plxerr.Code
}

// appendString appends a length-prefixed string.
func appendString(b []byte, s string) []byte {
	return append(binary.AppendUvarint(b, uint64(len(s))), s...)
}

// readUvarint reads a varint off the front of b.
func readUvarint(b []byte) (uint64, []byte, error) {
	v, n := binary.Uvarint(b)
	if n <= 0 {
		return 0, nil, errRecord
	}
	return v, b[n:], nil
}

// readString reads a length-prefixed string off the front of b.
func readString(b []byte) (string, []byte, error) {
	n, rest, err := readUvarint(b)
	if err != nil || n > uint64(len(rest)) {
		return "", nil, errRecord
	}
	return string(rest[:n]), rest[n:], nil
}

// encodeEvent encodes an event record.
func encodeEvent(deviceID string, deviceSeq uint64, ev *pluxv1.DevEvent) ([]byte, error) {
	b := binary.AppendUvarint(appendString(nil, deviceID), deviceSeq)
	out, err := proto.MarshalOptions{}.MarshalAppend(b, ev)
	if err != nil {
		return nil, fmt.Errorf("devsession: encode the event: %w", err)
	}
	return out, nil
}

// decodeEvent decodes an event record.
func decodeEvent(b []byte) (deviceID string, deviceSeq uint64, ev *pluxv1.DevEvent, err error) {
	if deviceID, b, err = readString(b); err != nil {
		return "", 0, nil, err
	}
	if deviceSeq, b, err = readUvarint(b); err != nil {
		return "", 0, nil, err
	}
	ev = &pluxv1.DevEvent{}
	if err := proto.Unmarshal(b, ev); err != nil {
		return "", 0, nil, errRecord
	}
	return deviceID, deviceSeq, ev, nil
}

// frameRecord encodes a frame for a device.
func frameRecord(deviceID string, f *pluxv1.DevServerFrame) ([]byte, error) {
	out, err := proto.MarshalOptions{}.MarshalAppend(appendString([]byte{byte(recordFrame)}, deviceID), f)
	if err != nil {
		return nil, fmt.Errorf("devsession: encode the frame: %w", err)
	}
	return out, nil
}

// helloRecord encodes a device's hello.
func helloRecord(deviceID string, done uint64) []byte {
	return binary.AppendUvarint(appendString([]byte{byte(recordHello)}, deviceID), done)
}

// endRecord encodes the session's end.
func endRecord(code plxerr.Code) []byte {
	return binary.AppendUvarint(appendString([]byte{byte(recordEnd)}, ""), uint64(code))
}

// decodeFrame decodes a frame record.
func decodeFrame(b []byte) (frameEntry, error) {
	if len(b) == 0 {
		return frameEntry{}, errRecord
	}
	e := frameEntry{kind: recordKind(b[0])}
	device, rest, err := readString(b[1:])
	if err != nil {
		return frameEntry{}, err
	}
	e.device = device
	switch e.kind {
	case recordFrame:
		e.frame = &pluxv1.DevServerFrame{}
		if err := proto.Unmarshal(rest, e.frame); err != nil {
			return frameEntry{}, errRecord
		}
	case recordHello:
		if e.done, _, err = readUvarint(rest); err != nil {
			return frameEntry{}, err
		}
	case recordEnd:
		code, _, err := readUvarint(rest)
		if err != nil || code == 0 || code > uint64(^plxerr.Code(0)) {
			return frameEntry{}, errRecord
		}
		e.code = plxerr.Code(code)
	default:
		return frameEntry{}, errRecord
	}
	return e, nil
}
