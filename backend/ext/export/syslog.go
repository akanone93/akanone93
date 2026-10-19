// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package export

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Syslog facility and severity numbers of RFC 5424 section 6.2.1 that the
// audit export uses.
const (
	// FacilityLogAudit is facility 13, "log audit".
	FacilityLogAudit = 13
	// SeverityInformational is severity 6.
	SeverityInformational = 6
)

// The field limits of RFC 5424 section 6.
const (
	maxHostname = 255
	maxAppName  = 48
	maxProcID   = 128
	maxMsgID    = 32
	maxSDName   = 32
	maxFacility = 23
	maxSeverity = 7
)

// nilValue is the NILVALUE of RFC 5424 section 6.
const nilValue = "-"

// utf8BOM introduces a UTF-8 message (RFC 5424 section 6.4).
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// SDParam is one name and value of a structured-data element.
type SDParam struct {
	Name  string
	Value string
}

// SDElement is a structured-data element: an identifier and its parameters.
// An identifier that is not registered with IANA must carry "@" and a
// private enterprise number (RFC 5424 section 7).
type SDElement struct {
	ID     string
	Params []SDParam
}

// SyslogMessage is an RFC 5424 message.
type SyslogMessage struct {
	Facility int
	Severity int
	// Time is the event time; the zero time is sent as NILVALUE.
	Time time.Time
	// Hostname, AppName, ProcID and MsgID are truncated to their limits and
	// have characters outside printable US-ASCII replaced; empty is NILVALUE.
	Hostname string
	AppName  string
	ProcID   string
	MsgID    string
	Data     []SDElement
	// Text is the free-form message, UTF-8; it is sent after a byte order
	// mark. Empty omits it.
	Text []byte
}

// Marshal renders the message as RFC 5424 specifies. It refuses a facility
// or severity out of range and a structured-data identifier or parameter
// name that the RFC does not allow.
func (m SyslogMessage) Marshal() ([]byte, error) {
	if m.Facility < 0 || m.Facility > maxFacility || m.Severity < 0 || m.Severity > maxSeverity {
		return nil, fmt.Errorf("export: facility %d and severity %d are out of range", m.Facility, m.Severity)
	}
	sd, err := marshalStructuredData(m.Data)
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	b.WriteString("<" + strconv.Itoa(m.Facility*8+m.Severity) + ">1 ")
	b.WriteString(timestamp(m.Time))
	for _, field := range []struct {
		value string
		limit int
	}{{m.Hostname, maxHostname}, {m.AppName, maxAppName}, {m.ProcID, maxProcID}, {m.MsgID, maxMsgID}} {
		b.WriteByte(' ')
		b.WriteString(headerField(field.value, field.limit))
	}
	b.WriteByte(' ')
	b.WriteString(sd)
	if len(m.Text) > 0 {
		b.WriteByte(' ')
		b.Write(utf8BOM)
		b.WriteString(strings.ToValidUTF8(string(m.Text), "�"))
	}
	return b.Bytes(), nil
}

// timestamp renders a time as the RFC 3339 form RFC 5424 requires: UTC,
// microseconds.
func timestamp(t time.Time) string {
	if t.IsZero() {
		return nilValue
	}
	return t.UTC().Format("2006-01-02T15:04:05.000000Z")
}

// headerField makes a HOSTNAME, APP-NAME, PROCID or MSGID: printable
// US-ASCII, no longer than limit, NILVALUE when empty.
func headerField(s string, limit int) string {
	out := make([]byte, 0, min(len(s), limit))
	for i := 0; i < len(s) && len(out) < limit; i++ {
		c := s[i]
		if c < 33 || c > 126 {
			c = '_'
		}
		out = append(out, c)
	}
	if len(out) == 0 {
		return nilValue
	}
	return string(out)
}

// marshalStructuredData renders the STRUCTURED-DATA field.
func marshalStructuredData(elements []SDElement) (string, error) {
	if len(elements) == 0 {
		return nilValue, nil
	}
	var b strings.Builder
	for _, e := range elements {
		if err := checkSDName(e.ID); err != nil {
			return "", fmt.Errorf("export: structured-data identifier: %w", err)
		}
		b.WriteString("[" + e.ID)
		for _, p := range e.Params {
			if err := checkSDName(p.Name); err != nil {
				return "", fmt.Errorf("export: structured-data parameter: %w", err)
			}
			b.WriteString(" " + p.Name + "=\"" + EscapeParamValue(p.Value) + "\"")
		}
		b.WriteByte(']')
	}
	return b.String(), nil
}

// checkSDName reports why s cannot be an SD-ID or a PARAM-NAME: one to 32
// printable US-ASCII characters other than '=', space, ']' and '"'.
func checkSDName(s string) error {
	if s == "" || len(s) > maxSDName {
		return fmt.Errorf("%q must be 1 to %d characters", s, maxSDName)
	}
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c < 33 || c > 126, c == '=', c == ']', c == '"':
			return errors.New("the name holds a character RFC 5424 forbids")
		}
	}
	return nil
}

// EscapeParamValue escapes a PARAM-VALUE as RFC 5424 section 6.3.3 requires:
// '"', '\' and ']' are preceded by a backslash. Bytes that are not valid
// UTF-8 become U+FFFD, so the message stays UTF-8.
func EscapeParamValue(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range strings.ToValidUTF8(s, "�") {
		if r == '"' || r == '\\' || r == ']' {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// Frame prefixes a message with its length in octets and a space, the
// octet-counting framing of RFC 5425 section 4.3 and RFC 6587 section 3.4.1.
func Frame(message []byte) []byte {
	prefix := strconv.Itoa(len(message)) + " "
	out := make([]byte, 0, len(prefix)+len(message))
	out = append(out, prefix...)
	return append(out, message...)
}
