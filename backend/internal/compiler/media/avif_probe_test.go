// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package media

import (
	"bytes"
	"context"
	"encoding/hex"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// avifProbeDart is the runtime's AVIF probe, relative to this package.
var avifProbeDart = filepath.Join("..", "..", "..", "..", "packages", "plux_flutter", "lib", "src", "assets", "avif_probe.dart")

// avifProbePixel is the probe's one pixel: half transparent, so the image
// carries an alpha plane as the variants of a transparent PNG do (#278).
var avifProbePixel = []byte{0x33, 0x66, 0x99, 0x80}

// Verifies: CMP-030.
// The runtime decides whether a device decodes the AVIF variants by
// decoding one AVIF image at start: that image must be what the variants
// are — this codec at the variants' settings, with an alpha plane — and the
// smallest a variant can be (1×1), so a device that decodes it decodes
// every variant it is offered (#278).
func TestAVIFProbe(t *testing.T) {
	t.Parallel()
	want, err := codecs(t).encodeAVIF(context.Background(), &RGBA{Pix: avifProbePixel, W: 1, H: 1}, avifQuality, avifSpeed)
	if err != nil {
		t.Fatal(err)
	}
	src, err := os.ReadFile(avifProbeDart)
	if err != nil {
		t.Fatal(err)
	}
	var got []byte
	for _, m := range regexp.MustCompile(`0x([0-9a-f]{2})`).FindAllSubmatch(src, -1) {
		b, err := hex.DecodeString(string(m[1]))
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, b...)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s holds %d bytes, the codec gives %d:\n%s", avifProbeDart, len(got), len(want), hex.EncodeToString(want))
	}
	if !bytes.Contains(want, []byte("auxC")) {
		t.Error("the probe has no alpha plane")
	}
}
