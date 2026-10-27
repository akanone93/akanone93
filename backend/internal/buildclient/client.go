// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package buildclient

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/fnpb"
	"github.com/nightCode42/plux3/backend/internal/fnpb/fnpbconnect"
	"github.com/nightCode42/plux3/backend/internal/function"
	"github.com/nightCode42/plux3/backend/internal/storage/objects"
)

// BuildSteps is the number of tools one build runs in sequence (lock file,
// metadata, tree, two compilations and wasm-opt), each under the build
// timeout of the registry; the deadline of a whole build is that many times
// the timeout, plus the time to move the source and the module.
const BuildSteps = 6

// transferSlack is the time added to a build's deadline for moving the source
// and the module over the network and for the schema and test runs.
const transferSlack = time.Minute

// Options configures a Client.
type Options struct {
	// HTTP is the mutual-TLS client (httpx.NewInternalClient); its own
	// timeout must exceed the longest build.
	HTTP *http.Client
	// URL is the base URL of the builder role.
	URL string
	// Objects holds the uploaded sources.
	Objects objects.Store
	// MaxResponseBytes bounds a response: the registry maximum of
	// function.moduleSize plus room for the schemas and test results.
	MaxResponseBytes int
}

// Client calls the builder role. It implements function.Builder.
type Client struct {
	rpc fnpbconnect.BuilderServiceClient
	// stream is the same route without the HTTP client's overall timeout,
	// which would cut a language session that lasts hours (FN-033); each
	// session has its own limit.
	stream  fnpbconnect.BuilderServiceClient
	objects objects.Store
}

// NewClient returns a Client.
func NewClient(o Options) (*Client, error) {
	if o.HTTP == nil || o.URL == "" || o.Objects == nil || o.MaxResponseBytes <= 0 {
		return nil, errors.New("buildclient.NewClient: HTTP, URL, Objects and MaxResponseBytes are required")
	}
	streaming := *o.HTTP
	streaming.Timeout = 0
	return &Client{
		rpc:     fnpbconnect.NewBuilderServiceClient(o.HTTP, o.URL, connect.WithReadMaxBytes(o.MaxResponseBytes)),
		stream:  fnpbconnect.NewBuilderServiceClient(&streaming, o.URL, connect.WithReadMaxBytes(o.MaxResponseBytes)),
		objects: o.Objects,
	}, nil
}

// Build implements function.Builder: it sends the stored source to the
// builder role and converts the response. A failure of the role, of the
// network or of the object store is an error the job retries; a source that
// does not build is a BuildOutput with error diagnostics.
func (c *Client) Build(ctx context.Context, in function.BuildInput) (function.BuildOutput, error) {
	source, _, err := c.objects.Get(ctx, in.SourceObjectKey)
	if err != nil {
		return function.BuildOutput{}, fmt.Errorf("buildclient.Build: read the source: %w", err)
	}
	if sum := sha256.Sum256(source); !strings.EqualFold(hex.EncodeToString(sum[:]), in.SourceSHA256) {
		return function.BuildOutput{}, errors.New("buildclient.Build: the stored source does not match its recorded SHA-256")
	}
	if in.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, in.Timeout*BuildSteps+transferSlack)
		defer cancel()
	}
	resp, err := c.rpc.Build(ctx, connect.NewRequest(&fnpb.BuildRequest{
		BuildId: in.BuildID, Source: source, SourceSha256: in.SourceSHA256, Name: in.Name, Placement: string(in.Placement),
		Limits: &fnpb.BuildLimits{TimeoutMs: uint32(min(in.Timeout.Milliseconds(), 1<<32-1)), MemoryBytes: uint64(max(in.MemoryBytes, 0))}, //nolint:gosec // G115: bounded and non-negative.
	}))
	if err != nil {
		return function.BuildOutput{}, fmt.Errorf("buildclient.Build: %w", err)
	}
	return outputOf(resp.Msg)
}

// outputOf converts a response. Output that is inconsistent is an error, not
// a diagnostic: the role is trusted to answer in its own format.
func outputOf(r *fnpb.BuildResponse) (function.BuildOutput, error) {
	out := function.BuildOutput{
		Module: r.GetModule(), ABIVersion: r.GetAbiVersion(), Placement: function.Placement(r.GetPlacement()),
		Declaration: r.GetDeclaration(),
	}
	if raw := r.GetDiagnosticsJson(); len(raw) > 0 {
		if err := json.Unmarshal(raw, &out.Diagnostics); err != nil {
			return function.BuildOutput{}, fmt.Errorf("buildclient.Build: read the diagnostics: %w", err)
		}
	}
	for _, f := range r.GetFunctions() {
		out.Functions = append(out.Functions, function.Export{Name: f.GetName(), InputSchema: f.GetInputSchema(), OutputSchema: f.GetOutputSchema()})
	}
	for _, t := range r.GetTestResults() {
		out.TestResults = append(out.TestResults, function.TestResult{Name: t.GetName(), Passed: t.GetPassed(), Message: t.GetMessage()})
	}
	if p := r.GetProvenance(); p != nil && len(out.Module) > 0 {
		prov, err := provenanceOf(p)
		if err != nil {
			return function.BuildOutput{}, err
		}
		out.Provenance = prov
	}
	if len(out.Module) == 0 && !out.Diagnostics.HasErrors() {
		return function.BuildOutput{}, errors.New("buildclient.Build: the builder returned neither a module nor an error diagnostic")
	}
	return out, nil
}

// provenanceOf converts the provenance of a response (FN-005).
func provenanceOf(p *fnpb.BuildProvenance) (function.Provenance, error) {
	built, err := time.Parse(time.RFC3339, p.GetBuildTime())
	if err != nil {
		return function.Provenance{}, fmt.Errorf("buildclient.Build: the build time %q: %w", p.GetBuildTime(), err)
	}
	prov := function.Provenance{
		SourceSHA256: p.GetSourceSha256(), SDKSHA256: p.GetSdkSha256(), Toolchain: p.GetToolchain(),
		WasmOptVersion: p.GetWasmOptVersion(), BuildTime: built.UTC(), BuilderIdentity: p.GetBuilderIdentity(),
		Dependencies: []function.Dependency{},
	}
	for _, d := range p.GetDependencies() {
		prov.Dependencies = append(prov.Dependencies, function.Dependency{Name: d.GetName(), Version: d.GetVersion(), Checksum: d.GetChecksum()})
	}
	return prov, nil
}

var _ function.Linker = (*Client)(nil)

// Link implements function.Linker: it sends the stored sources of a plugin's
// device-placed functions to the builder role and converts the response
// (FN-051). Failures of the role, of the network or of the object store are
// errors; sources that do not link are a LinkOutput with error diagnostics.
func (c *Client) Link(ctx context.Context, in function.LinkInput) (function.LinkOutput, error) {
	sources := make([]*fnpb.LinkSource, 0, len(in.Sources))
	for _, src := range in.Sources {
		data, _, err := c.objects.Get(ctx, src.SourceObjectKey)
		if err != nil {
			return function.LinkOutput{}, fmt.Errorf("buildclient.Link: read the source of %s: %w", src.Name, err)
		}
		if sum := sha256.Sum256(data); !strings.EqualFold(hex.EncodeToString(sum[:]), src.SourceSHA256) {
			return function.LinkOutput{}, fmt.Errorf("buildclient.Link: the stored source of %s does not match its recorded SHA-256", src.Name)
		}
		sources = append(sources, &fnpb.LinkSource{Name: src.Name, Source: data, SourceSha256: src.SourceSHA256})
	}
	if in.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, in.Timeout*BuildSteps+transferSlack)
		defer cancel()
	}
	resp, err := c.rpc.Link(ctx, connect.NewRequest(&fnpb.LinkRequest{
		LinkId: in.LinkID, Sources: sources,
		Limits: &fnpb.BuildLimits{TimeoutMs: uint32(min(in.Timeout.Milliseconds(), 1<<32-1)), MemoryBytes: uint64(max(in.MemoryBytes, 0))}, //nolint:gosec // G115: bounded and non-negative.
	}))
	if err != nil {
		return function.LinkOutput{}, fmt.Errorf("buildclient.Link: %w", err)
	}
	return linkOutputOf(resp.Msg)
}

// linkOutputOf converts a response; output that is inconsistent is an error.
func linkOutputOf(r *fnpb.LinkResponse) (function.LinkOutput, error) {
	out := function.LinkOutput{Module: r.GetModule(), ABIVersion: r.GetAbiVersion(), Index: map[string]uint32{}}
	if raw := r.GetDiagnosticsJson(); len(raw) > 0 {
		if err := json.Unmarshal(raw, &out.Diagnostics); err != nil {
			return function.LinkOutput{}, fmt.Errorf("buildclient.Link: read the diagnostics: %w", err)
		}
	}
	for _, f := range r.GetFunctions() {
		out.Index[f.GetName()] = f.GetIndex()
	}
	if len(out.Module) == 0 && !out.Diagnostics.HasErrors() {
		return function.LinkOutput{}, errors.New("buildclient.Link: the builder returned neither a module nor an error diagnostic")
	}
	return out, nil
}
