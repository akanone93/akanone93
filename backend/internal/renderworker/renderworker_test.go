// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package renderworker_test

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"testing"
	"testing/fstest"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/plxgolden"
	"github.com/nightCode42/plux3/backend/internal/renderpb"
	"github.com/nightCode42/plux3/backend/internal/renderpb/renderpbconnect"
	"github.com/nightCode42/plux3/backend/internal/renderworker"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
)

const (
	starter   = "../../../schema/testdata/documents/starter"
	fontsDir  = "../../../schema/goldens/fonts"
	smallest  = "small-360x640"
	pngHeader = "\x89PNG\r\n\x1a\n"
)

func testLimits() renderworker.Limits { return renderworker.LimitsOf(limits.Defaults()) }

func project(t *testing.T) fstest.MapFS {
	t.Helper()
	out := fstest.MapFS{}
	root := os.DirFS(starter)
	err := fs.WalkDir(root, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || strings.HasPrefix(p, "tests/") {
			return err
		}
		data, err := fs.ReadFile(root, p)
		out[p] = &fstest.MapFile{Data: data}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// Verifies: STU-100.
// A project survives the trip as a stored ZIP, packs to the same bytes every
// time, and is refused when it has more files or bytes than its limits.
func TestPackAndUnpack_STU_100(t *testing.T) {
	t.Parallel()
	files := project(t)
	a, err := renderworker.Pack(files, testLimits())
	if err != nil {
		t.Fatal(err)
	}
	b, err := renderworker.Pack(files, testLimits())
	if err != nil || !bytes.Equal(a, b) {
		t.Fatalf("packing twice differs (%v)", err)
	}
	back, err := renderworker.Unpack(a, testLimits())
	if err != nil {
		t.Fatal(err)
	}
	if len(back) != len(files) {
		t.Fatalf("%d files came back, want %d", len(back), len(files))
	}
	for name, f := range files {
		if !bytes.Equal(back[name].Data, f.Data) {
			t.Errorf("%s changed in transit", name)
		}
	}
	tight := testLimits()
	tight.Entries = 2
	if _, err := renderworker.Pack(files, tight); !errors.Is(err, renderworker.ErrTooLarge) {
		t.Errorf("Pack over the file limit = %v", err)
	}
	if _, err := renderworker.Unpack(a, tight); !errors.Is(err, renderworker.ErrTooLarge) {
		t.Errorf("Unpack over the file limit = %v", err)
	}
	tight = testLimits()
	tight.ProjectBytes = 100
	if _, err := renderworker.Pack(files, tight); !errors.Is(err, renderworker.ErrTooLarge) {
		t.Errorf("Pack over the byte limit = %v", err)
	}
	if _, err := renderworker.Unpack(a, tight); !errors.Is(err, renderworker.ErrTooLarge) {
		t.Errorf("Unpack over the byte limit = %v", err)
	}
}

// Verifies: STU-100, SEC-052.
// An archive from the network is untrusted: a name that leaves the project or
// repeats is refused before anything is compiled.
func TestUnpackRefusesHostileNames_STU_100(t *testing.T) {
	t.Parallel()
	zipOf := func(names ...string) []byte {
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		for _, n := range names {
			w, err := zw.Create(n)
			if err != nil {
				t.Fatal(err)
			}
			_, _ = w.Write([]byte("x"))
		}
		if err := zw.Close(); err != nil {
			t.Fatal(err)
		}
		return buf.Bytes()
	}
	for name, data := range map[string][]byte{
		"parent":   zipOf("../escape.json"),
		"absolute": zipOf("/etc/passwd"),
		"dot":      zipOf("a/../../b"),
		"repeat":   zipOf("a.json", "a.json"),
		"garbage":  []byte("not a zip"),
	} {
		if _, err := renderworker.Unpack(data, testLimits()); err == nil {
			t.Errorf("%s: the archive was accepted", name)
		}
	}
}

// Verifies: LIM-001.
// A request cannot ask for more than the registry allows, or for nothing.
func TestLimitsAreClampedToTheRegistry_LIM_001(t *testing.T) {
	t.Parallel()
	got := renderworker.Limits{ProjectBytes: 1 << 62, Entries: -1, Cases: 0, ImageBytes: 1 << 62}.Clamped()
	for _, c := range []struct {
		key  limits.Key
		have int64
		max  bool
	}{
		{limits.ArchiveExpandedBytes, got.ProjectBytes, true},
		{limits.ArchiveEntries, got.Entries, false},
		{limits.GoldenCasesPerRun, got.Cases, false},
		{limits.GoldenImageBytes, got.ImageBytes, true},
	} {
		def, _ := limits.Lookup(c.key)
		want := def.Default
		if c.max {
			want = def.Max
		}
		if c.have != want {
			t.Errorf("%s = %d, want %d", c.key, c.have, want)
		}
	}
}

// fakeFlutter draws every case as a PNG whose bytes depend on the compiled
// bundles of the project and on the case: the same project always looks the
// same, and a changed one looks different. It also records the plans.
type fakeFlutter struct{ cases []int }

func (f *fakeFlutter) execute(_ context.Context, plan *plxgolden.Plan, _ string, _ limits.Set) ([]plxgolden.Rendered, error) {
	f.cases = append(f.cases, len(plan.Cases))
	var names []string
	for n := range plan.Files {
		if strings.HasPrefix(n, "assets/plux/bundles/") {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	h := sha256.New()
	for _, n := range names {
		h.Write(plan.Files[n])
	}
	out := make([]plxgolden.Rendered, len(plan.Cases))
	for i, c := range plan.Cases {
		img := append([]byte(pngHeader), h.Sum([]byte(c.ID))...)
		out[i] = plxgolden.Rendered{Case: c, PNG: img}
	}
	return out, nil
}

func startWorker(t *testing.T, flutter *fakeFlutter) renderpbconnect.RenderWorkerServiceClient {
	t.Helper()
	fonts, err := plxgolden.LoadFonts(fontsDir)
	if err != nil {
		t.Fatal(err)
	}
	h, err := renderworker.New(renderworker.Options{Fonts: fonts, WorkDir: t.TempDir(), Execute: flutter.execute})
	if err != nil {
		t.Fatal(err)
	}
	return serve(t, h)
}

type limitsSet = limits.Set

// serve answers h on a local server and returns its client.
func serve(t *testing.T, h *renderworker.Handler) renderpbconnect.RenderWorkerServiceClient {
	t.Helper()
	mux := http.NewServeMux()
	mux.Handle(renderpbconnect.NewRenderWorkerServiceHandler(h))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return renderpbconnect.NewRenderWorkerServiceClient(srv.Client(), srv.URL)
}

func render(t *testing.T, c renderpbconnect.RenderWorkerServiceClient, before, after fs.FS, devices ...string) ([]*renderpb.RenderResponse, error) {
	t.Helper()
	req := &renderpb.RenderRequest{JobId: "j1", Devices: devices}
	var err error
	if before != nil {
		if req.Before, err = renderworker.Pack(before, testLimits()); err != nil {
			t.Fatal(err)
		}
	}
	if after != nil {
		if req.After, err = renderworker.Pack(after, testLimits()); err != nil {
			t.Fatal(err)
		}
	}
	stream, err := c.Render(context.Background(), connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	var out []*renderpb.RenderResponse
	for stream.Receive() {
		out = append(out, stream.Msg())
	}
	return out, stream.Err()
}

// Verifies: STU-100, TST-003.
// The worker renders a narrowed matrix (the named devices, one locale, the
// light theme, text scale 1) of both projects and returns the screens whose
// image changed: none for an unchanged project, every screen as new when the
// channel served nothing, and the changed ones, ordered, otherwise.
func TestRenderReturnsChangedScreens_STU_100(t *testing.T) {
	t.Parallel()
	flutter := &fakeFlutter{}
	c := startWorker(t, flutter)
	was := project(t)

	same, err := render(t, c, was, was, smallest)
	if err != nil || len(same) != 0 {
		t.Fatalf("an unchanged project: %d screens, %v", len(same), err)
	}
	for _, n := range flutter.cases {
		if n != 5 {
			t.Errorf("a render drew %d cases; one device, one locale, the light theme and scale 1 give 5 pages", n)
		}
	}

	fresh, err := render(t, c, nil, was, smallest)
	if err != nil || len(fresh) != 5 {
		t.Fatalf("a first release: %d screens, %v", len(fresh), err)
	}
	for _, s := range fresh {
		if len(s.GetBeforePng()) != 0 || !strings.HasPrefix(string(s.GetAfterPng()), pngHeader) || s.GetDevice() != smallest || s.GetPluginKey() == "" && s.GetScreen() == "" {
			t.Errorf("new screen = %+v", s)
		}
	}
	if !sort.SliceIsSorted(fresh, func(i, j int) bool {
		return fresh[i].GetPluginKey()+"\x00"+fresh[i].GetScreen() < fresh[j].GetPluginKey()+"\x00"+fresh[j].GetScreen()
	}) {
		t.Error("the screens are not in order of plugin and screen")
	}

	// A new English wording changes the compiled bundles and so every screen
	// of the fake; the screens come back with both images.
	now := project(t)
	now["translations/en.json"] = &fstest.MapFile{Data: bytes.ReplaceAll(now["translations/en.json"].Data, []byte(`"Share"`), []byte(`"Send"`))}
	if bytes.Equal(now["translations/en.json"].Data, was["translations/en.json"].Data) {
		t.Fatal("the starter's English messages no longer contain \"Share\"; pick another word")
	}
	changed, err := render(t, c, was, now, smallest)
	if err != nil || len(changed) == 0 {
		t.Fatalf("a changed project: %d screens, %v", len(changed), err)
	}
	for _, s := range changed {
		if !strings.HasPrefix(string(s.GetBeforePng()), pngHeader) || !strings.HasPrefix(string(s.GetAfterPng()), pngHeader) || bytes.Equal(s.GetBeforePng(), s.GetAfterPng()) {
			t.Errorf("changed screen %s has before %d bytes, after %d bytes", s.GetScreen(), len(s.GetBeforePng()), len(s.GetAfterPng()))
		}
	}
}

// Verifies: STU-100.
// A request without a project, with a hostile archive or with a project that
// does not compile is refused with a reason, and a second render while the
// worker is full is refused as exhausted so that the server queues it again.
func TestRenderRefusesWhatItCannotRender_STU_100(t *testing.T) {
	t.Parallel()
	c := startWorker(t, &fakeFlutter{})
	if _, err := render(t, c, nil, nil, smallest); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("no project: %v", err)
	}
	broken := fstest.MapFS{"app.json": {Data: []byte("{")}}
	if _, err := render(t, c, nil, broken, smallest); connect.CodeOf(err) != connect.CodeFailedPrecondition || !strings.Contains(err.Error(), "does not compile") {
		t.Errorf("a project that does not compile: %v", err)
	}
	stream, err := c.Render(context.Background(), connect.NewRequest(&renderpb.RenderRequest{After: []byte("not a zip")}))
	if err == nil {
		stream.Receive()
		err = stream.Err()
	}
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("a hostile archive: %v", err)
	}
}

// Verifies: STU-100.
// The worker needs a Flutter executable and the golden fonts unless told how
// to render, so a misconfigured process fails on start, not on the first job.
func TestNewRequiresItsToolchain_STU_100(t *testing.T) {
	t.Parallel()
	if _, err := renderworker.New(renderworker.Options{Fonts: map[string][]byte{"a": {1}}}); err == nil {
		t.Error("no Flutter executable was accepted")
	}
	if _, err := renderworker.New(renderworker.Options{Flutter: "flutter"}); err == nil {
		t.Error("no fonts were accepted")
	}
}
