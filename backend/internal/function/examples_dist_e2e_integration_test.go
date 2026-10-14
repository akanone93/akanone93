// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package function_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nightCode42/plux3/backend/internal/buildclient"
	"github.com/nightCode42/plux3/backend/internal/builder"
	"github.com/nightCode42/plux3/backend/internal/builderrole"
	"github.com/nightCode42/plux3/backend/internal/function"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage/objects"
)

// exampleBuildTime and exampleBuilder are what the committed artefacts
// record as the time and the builder of their build: fixed, so that a build
// of the same sources gives the same record.
var exampleBuildTime = time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)

const exampleBuilder = "plux-examples"

// distRecord is the file build.json of an example's dist directory: what the
// builder reported of the build besides the module and the declaration. The
// reference end-to-end tests (backend/internal/server) read the same file
// into the same fields.
type distRecord struct {
	ABIVersion  int32                 `json:"abiVersion"`
	Placement   function.Placement    `json:"placement"`
	Functions   []function.Export     `json:"functions"`
	Provenance  function.Provenance   `json:"provenance"`
	TestResults []function.TestResult `json:"testResults"`
}

// linkRecord is the file link.json: the dispatch indices of the module that
// links the example (FN-051), which is module.wasm.
type linkRecord struct {
	ABIVersion int32             `json:"abiVersion"`
	Index      map[string]uint32 `json:"index"`
}

// example is one of the example functions the reference apps call.
type example struct {
	// dir is the module crate of sdk/fn/examples/modules the artefacts
	// belong to; crate the function crate beside it.
	dir, crate, name, manifest string
	// links says whether the example is a device function, which the
	// plugin's device module links (FN-051).
	links bool
}

var examples = []example{
	{dir: "fee-module", crate: "fee", name: "bank.transferFee", manifest: feeManifest},
	{dir: "loan-module", crate: "loan", name: "loan.calculateSchedule", manifest: loanManifest, links: true},
}

// exampleSources lays out an example as an uploadable crate: as feeSource
// and loanSource do, but with the dependency versions of sdk/fn/Cargo.lock,
// so that the archive, and with it the build, does not change when a crate
// is published.
func exampleSources(t *testing.T, root string, ex example) []byte {
	t.Helper()
	proj := filepath.Join(t.TempDir(), ex.crate)
	if err := os.MkdirAll(proj, 0o750); err != nil {
		t.Fatal(err)
	}
	examplesDir := filepath.Join(root, "examples", ex.crate)
	if err := os.CopyFS(filepath.Join(proj, "src"), os.DirFS(filepath.Join(examplesDir, "src"))); err != nil {
		t.Fatal(err)
	}
	doc, err := os.ReadFile(filepath.Join(examplesDir, "function.json")) //nolint:gosec // G304: a fixed path in the repository.
	if err != nil {
		t.Fatal(err)
	}
	lock, err := os.ReadFile(filepath.Join(root, "Cargo.lock")) //nolint:gosec // G304: a fixed path in the repository.
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{
		"function.json": doc, "Cargo.lock": lock,
		"Cargo.toml": fmt.Appendf(nil, ex.manifest, filepath.Join(root, "plux-fn")),
	} {
		if err := os.WriteFile(filepath.Join(proj, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	runIn(t, proj, "cargo", "vendor", "--quiet")
	return packReproducibly(t, proj)
}

// packReproducibly packs a crate directory as packSource does, with the
// modification times, owners and permissions of its entries fixed: the
// digest of the archive is part of the provenance a build records, so the
// same files must give the same archive.
func packReproducibly(t *testing.T, dir string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || path == dir {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		h, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		h.Name = filepath.ToSlash(rel)
		h.Uid, h.Gid, h.Uname, h.Gname = 0, 0, "", ""
		h.ModTime, h.AccessTime, h.ChangeTime = time.Unix(0, 0), time.Time{}, time.Time{}
		h.Format = tar.FormatPAX
		h.Mode = 0o644
		if info.IsDir() {
			h.Mode = 0o755
		}
		if err := tw.WriteHeader(h); err != nil || !info.Mode().IsRegular() {
			return err
		}
		data, err := os.ReadFile(path) //nolint:gosec // G304: walked from the test's own directory.
		if err != nil {
			return err
		}
		_, err = tw.Write(data)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// startExampleBuilder serves a builder role over the real toolchain whose
// clock and identity are fixed, and returns a client of it.
func startExampleBuilder(t *testing.T, root string, store objects.Store) *buildclient.Client {
	t.Helper()
	wasmOpt := filepath.Join(t.TempDir(), "binaryen")
	if err := os.MkdirAll(wasmOpt, 0o750); err != nil {
		t.Fatal(err)
	}
	runIn(t, wasmOpt, "bun", "add", "--exact", "binaryen@132.0.0")
	sdk := filepath.Join(root, "plux-fn")
	role, err := builderrole.New(t.Context(), builderrole.Options{
		Listen: "127.0.0.1:0", TLS: &tls.Config{ClientAuth: tls.RequireAndVerifyClientCert},
		Tools: builder.Config{
			Sandbox: builder.NoSandbox{}, Clock: func() time.Time { return exampleBuildTime }, Identity: exampleBuilder, SDKDir: sdk,
			WasmOpt: builder.DefaultWasmOpt(wasmOpt),
		},
		Unsandboxed: true, Limits: limits.Defaults(), Concurrency: 1, Clock: time.Now, Random: rand.Reader,
	})
	if err != nil {
		t.Fatal(err)
	}
	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	if err := role.Serve(ln); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = role.Stop(t.Context()) })
	client, err := buildclient.NewClient(buildclient.Options{
		HTTP: &http.Client{Timeout: 30 * time.Minute}, URL: "http://" + ln.Addr().String(),
		Objects: store, MaxResponseBytes: 80 << 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

// stored puts a source archive in the store as UploadSource does and returns
// its key and digest.
func stored(ctx context.Context, t *testing.T, store objects.Store, archive []byte) (key, digest string) {
	t.Helper()
	sum := sha256.Sum256(archive)
	digest = hex.EncodeToString(sum[:])
	key, err := objects.Key(objects.KindSource, digest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Put(ctx, key, archive, "application/gzip"); err != nil {
		t.Fatal(err)
	}
	return key, digest
}

// artefact is one file of a dist directory.
type artefact struct {
	name string
	data []byte
}

// marshalIndented renders a record the way it is committed.
func marshalIndented(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return append(data, '\n')
}

// Verifies: FN-019, FN-016, FN-051.
// The artefacts that the reference end-to-end tests deploy in place of a
// build (backend/internal/server, reference_functions_integration_test.go)
// are what the real builder role makes of the example functions: the module
// of each, its declaration, schemas, provenance and test results, and for
// the device function the module that links it. A rebuild that differs from
// the committed files fails the test; PLUX_UPDATE_DIST=1 writes them anew
// .
func TestExampleArtefactsAreWhatTheBuilderMakes_FN_019(t *testing.T) {
	if os.Getenv("PLUX_RUST_E2E") != "1" {
		t.Skip("set PLUX_RUST_E2E=1 to build with the real Rust toolchain and Binaryen")
	}
	root, err := filepath.Abs("../../../sdk/fn")
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	store, err := objects.NewFilesystem(t.TempDir(), "https://p.example")
	if err != nil {
		t.Fatal(err)
	}
	client := startExampleBuilder(t, root, store)
	lim := limits.Defaults()

	for _, ex := range examples {
		t.Run(ex.name, func(t *testing.T) {
			key, digest := stored(ctx, t, store, exampleSources(t, root, ex))
			out, err := client.Build(ctx, function.BuildInput{
				BuildID: "examples", SourceObjectKey: key, SourceSHA256: digest, Name: ex.name,
				Timeout:     time.Duration(lim.Get(limits.FunctionBuildTimeout)) * time.Millisecond,
				MemoryBytes: lim.Get(limits.FunctionBuildMemory), ModuleBytes: lim.Get(limits.FunctionModuleSize),
			})
			if err != nil {
				t.Fatalf("Build: %v", err)
			}
			if out.Diagnostics.HasErrors() || len(out.Module) == 0 {
				t.Fatalf("the build failed: %v", out.Diagnostics)
			}
			for _, r := range out.TestResults {
				if !r.Passed {
					t.Errorf("test %q failed: %s", r.Name, r.Message)
				}
			}
			files := []artefact{
				{"module.wasm", out.Module},
				{"declaration.json", append(bytes.Clone(out.Declaration), '\n')},
				{"build.json", marshalIndented(t, distRecord{
					ABIVersion: out.ABIVersion, Placement: out.Placement, Functions: out.Functions,
					Provenance: out.Provenance, TestResults: out.TestResults,
				})},
			}
			if ex.links {
				linked, err := client.Link(ctx, function.LinkInput{
					LinkID: "examples", Sources: []function.LinkSource{{Name: ex.name, SourceObjectKey: key, SourceSHA256: digest}},
					Timeout:     time.Duration(lim.Get(limits.FunctionBuildTimeout)) * time.Millisecond,
					MemoryBytes: lim.Get(limits.FunctionBuildMemory),
				})
				if err != nil || linked.Diagnostics.HasErrors() {
					t.Fatalf("Link: %v %v", err, linked.Diagnostics)
				}
				// A set of one function links to the module that was built
				// for it, so one file serves both.
				if !bytes.Equal(linked.Module, out.Module) {
					t.Fatalf("the module that links %s (%d bytes) is not the one built for it (%d bytes); commit it as linked.wasm",
						ex.name, len(linked.Module), len(out.Module))
				}
				files = append(files, artefact{"link.json", marshalIndented(t, linkRecord{ABIVersion: linked.ABIVersion, Index: linked.Index})})
			}

			dist := filepath.Join(root, "examples", "modules", ex.dir, "dist")
			if os.Getenv("PLUX_UPDATE_DIST") == "1" {
				if err := os.MkdirAll(dist, 0o750); err != nil {
					t.Fatal(err)
				}
			}
			for _, f := range files {
				path := filepath.Join(dist, f.name)
				if os.Getenv("PLUX_UPDATE_DIST") == "1" {
					if err := os.WriteFile(path, f.data, 0o644); err != nil { //nolint:gosec // G306: a committed, public file.
						t.Fatal(err)
					}
					continue
				}
				want, err := os.ReadFile(path) //nolint:gosec // G304: a fixed path in the repository.
				if err != nil {
					t.Fatalf("%v; run with PLUX_UPDATE_DIST=1 to write the artefacts", err)
				}
				if !bytes.Equal(want, f.data) {
					t.Errorf("%s: the build gives %d bytes (sha256 %x), the committed file has %d (sha256 %x); if the sources or the toolchain changed on purpose, run with PLUX_UPDATE_DIST=1 and commit the files",
						path, len(f.data), sha256.Sum256(f.data), len(want), sha256.Sum256(want))
				}
			}
		})
	}
}
