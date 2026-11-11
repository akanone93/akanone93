// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package renderworker_test

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/plxgolden"
	"github.com/nightCode42/plux3/backend/internal/plxtest"
	"github.com/nightCode42/plux3/backend/internal/renderworker"
)

// Verifies: STU-100.
// The worker renders with the real Flutter on a host with no route out, as
// in its image: every proxy leads nowhere, so an online package resolution
// would fail (pub's exit status 69), and the render succeeds only because
// the packages come from the pub cache alone. It needs the Flutter SDK and a
// pub cache that holds the runtime's packages (flutter pub get in the
// workspace), which the other tests do not.
func TestRenderWithFlutterAndNoNetwork_STU_100(t *testing.T) {
	flutter, err := exec.LookPath("flutter")
	if err != nil {
		t.Skip("flutter is not installed: ", err)
	}
	runtime, err := filepath.Abs("../../../packages/plux_flutter")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy"} {
		t.Setenv(name, "http://127.0.0.1:9")
	}
	// The Flutter tool and its tester talk over loopback; only that bypasses
	// the proxy.
	t.Setenv("NO_PROXY", "localhost,127.0.0.1,::1")
	t.Setenv("no_proxy", "localhost,127.0.0.1,::1")
	fonts, err := plxgolden.LoadFonts(fontsDir)
	if err != nil {
		t.Fatal(err)
	}
	var log strings.Builder
	h, err := renderworker.New(renderworker.Options{
		Flutter: flutter, Fonts: fonts, Runtime: plxtest.Dependency{Path: runtime}, WorkDir: t.TempDir(), Log: &log,
	})
	if err != nil {
		t.Fatal(err)
	}
	screens, err := render(t, serve(t, h), nil, project(t), smallest)
	if err != nil {
		t.Fatalf("render: %v\n%s", err, log.String())
	}
	if len(screens) != 5 {
		t.Fatalf("a first release drew %d screens, want the starter's 5", len(screens))
	}
	for _, s := range screens {
		if !strings.HasPrefix(string(s.GetAfterPng()), pngHeader) {
			t.Errorf("screen %s/%s has no PNG", s.GetPluginKey(), s.GetScreen())
		}
	}
}
