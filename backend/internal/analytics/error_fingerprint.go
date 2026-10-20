// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package analytics

import (
	"crypto/sha256"
	"encoding/hex"
	"path"
	"regexp"
	"strings"
)

// fingerprintFrames is how many frames, from the top of the stack, a
// fingerprint covers. Changing it regroups every error from then on, so it
// is a property of the algorithm, not a tunable limit.
const fingerprintFrames = 5

var (
	// frameIndex is the "#3" a Dart stack trace puts before each frame.
	frameIndex = regexp.MustCompile(`^#\d+\s+`)
	// frameAddress is a memory address, which differs on every run.
	frameAddress = regexp.MustCompile(`\b0x[0-9a-fA-F]+\b`)
	// frameLocation is a "(file:line:col)" or "(file:line)" location.
	frameLocation = regexp.MustCompile(`\(([^()]*)\)`)
	// framePosition is the ":line:col" a location ends with.
	framePosition = regexp.MustCompile(`(:\d+){1,2}$`)
	blanks        = regexp.MustCompile(`\s+`)
)

// Fingerprint identifies a kind of error (ANL-040): the error code, the
// node path where the runtime reported it, and the top frames of its stack,
// as NormaliseFrames returns them. The same error on two devices has the
// same fingerprint; a different code, node or frame has another.
//
// The node path stands for the document JSON path. Within one release the
// two correspond one to one, because the source map maps each compiled node
// to one pointer, and groups are kept per release; the fingerprint can
// therefore be computed at ingestion, without reading a source map, and the
// path is resolved when a group is read (see Service.GetErrorGroup).
func Fingerprint(code, nodePath string, frames []string) string {
	h := sha256.New()
	for _, part := range append([]string{code, nodePath}, frames...) {
		h.Write([]byte(part))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil)[:16])
}

// NormaliseFrames reads the top frames of a stack trace, one frame per line,
// and removes what differs from device to device or build to build: frame
// numbers, memory addresses, line and column numbers, and the directories of
// files outside a package. "#0  Foo.bar (package:app/src/x.dart:12:5)"
// becomes "Foo.bar (package:app/src/x.dart)". Blank lines are dropped, and
// at most fingerprintFrames frames are kept.
func NormaliseFrames(stack string) []string {
	var out []string
	for _, line := range strings.Split(stack, "\n") {
		if f := normaliseFrame(line); f != "" {
			out = append(out, f)
			if len(out) == fingerprintFrames {
				break
			}
		}
	}
	return out
}

// normaliseFrame normalises one frame.
func normaliseFrame(line string) string {
	line = frameIndex.ReplaceAllString(strings.TrimSpace(line), "")
	line = frameAddress.ReplaceAllString(line, "")
	line = frameLocation.ReplaceAllStringFunc(line, func(loc string) string {
		return "(" + normaliseLocation(loc[1:len(loc)-1]) + ")"
	})
	return strings.TrimSpace(blanks.ReplaceAllString(line, " "))
}

// normaliseLocation drops the position from a location, and the directory
// from a file that is not addressed by a scheme such as package: or dart:.
func normaliseLocation(loc string) string {
	loc = framePosition.ReplaceAllString(strings.TrimSpace(loc), "")
	if !strings.Contains(loc, ":") || strings.HasPrefix(loc, "file:") {
		return path.Base(strings.TrimPrefix(strings.ReplaceAll(loc, `\`, "/"), "file://"))
	}
	return loc
}
