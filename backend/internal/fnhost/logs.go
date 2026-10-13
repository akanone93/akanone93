// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package fnhost

import "bytes"

// logBudget accounts the log output of one invocation against
// Limits.LogBytes (FN-011) and forwards the records that fit. It is used on
// one goroutine.
type logBudget struct {
	limit     int
	used      int
	truncated bool
	sink      func(LogRecord)
}

// add accepts a record if it fits in the budget. The first record that does
// not fit is replaced by one truncation marker and every later one is
// dropped.
func (b *logBudget) add(level LogLevel, msg []byte) {
	if b.truncated {
		return
	}
	if len(msg) > b.limit-b.used {
		b.truncated = true
		b.emit(LogRecord{Level: LogWarn, Message: LogTruncated})
		return
	}
	b.used += len(msg)
	b.emit(LogRecord{Level: level, Message: string(bytes.TrimSuffix(msg, []byte("\n")))})
}

func (b *logBudget) emit(r LogRecord) {
	if b.sink != nil {
		b.sink(r)
	}
}

// stream is the writer bound to a guest's standard output or error.
type stream struct {
	budget *logBudget
	level  LogLevel
}

// Write implements io.Writer. It never fails, so that a guest that logs too
// much is not blocked by an error it cannot interpret.
func (s stream) Write(p []byte) (int, error) {
	s.budget.add(s.level, p)
	return len(p), nil
}
