// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package devsession_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/coder/websocket"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/nightCode42/plux3/backend/internal/compiler"
	"github.com/nightCode42/plux3/backend/internal/compiler/incremental/incrementaltest"
	"github.com/nightCode42/plux3/backend/internal/device/devicetest"
	"github.com/nightCode42/plux3/backend/internal/devsession"
	"github.com/nightCode42/plux3/backend/internal/document"
	"github.com/nightCode42/plux3/backend/internal/icons"
	"github.com/nightCode42/plux3/backend/internal/netsim"
	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
)

// The budgets of the edit-to-device latency (NFR-030, DEV-010): 500 ms from
// the save to the changed page on the device at the 95th percentile, of which
// the device's apply takes at most 150 ms (make bench-devapply, measured in
// Dart), leaving the server and the transport the rest.
const (
	devloopEdits       = 200
	devloopWarmup      = 5
	devloopPages       = 48 // copies of the calculator page; with the original and the result page, a 50-page plugin
	devloopNodes       = 100
	devloopBudget      = 350 * time.Millisecond
	devloopProfileName = "good_wifi"
)

// bigDrafts serves a prepared project as the environment's draft.
type bigDrafts struct{ files fstest.MapFS }

func (d bigDrafts) DraftFiles(context.Context, pgx.Tx, string) (fstest.MapFS, error) {
	out := make(fstest.MapFS, len(d.files))
	for k, v := range d.files {
		out[k] = v
	}
	return out, nil
}

// editedNode is the expression of one of the Text nodes the generator adds to
// every page: an edit that reaches the compiled page, unlike a description.
const editedNode = "/root/slots/body/slots/child/children/50/props/data/$expr"

// devloopSample is one edit: the stages of the server, and the whole.
type devloopSample struct {
	server    time.Duration // compile + sign + deliver, from the push's timings
	admit     time.Duration // the rest of the push call: authorise, read, patch, audit
	transport time.Duration // from the push's return to the device having parsed the frame
	total     time.Duration // from the save to the device having parsed the frame
}

// arrival is a bundle frame the simulated device received and parsed.
type arrival struct {
	sequence uint64
	at       time.Time
	bytes    int
}

// pct returns the p-th percentile of the durations (nearest rank).
func pct(ds []time.Duration, p int) time.Duration {
	s := slices.Clone(ds)
	slices.Sort(s)
	return s[max((len(s)*p+99)/100-1, 0)]
}

func stat(ds []time.Duration) map[string]float64 {
	ms := func(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }
	return map[string]float64{"p50_ms": ms(pct(ds, 50)), "p95_ms": ms(pct(ds, 95)), "max_ms": ms(pct(ds, 100))}
}

// Verifies: NFR-030, DEV-010.
// 200 edits of one page of a 50-page plugin, each pushed through
// Service.PushPatch to a device connected through the real dev socket over the
// good_wifi profile, measured from the save to the device having parsed the
// bundle frame: the 95th percentile of the server and the transport together
// is at most 350 ms; the device's share is measured by make bench-devapply.
// Run with make bench-devloop.
func TestDevloopEditToDeviceLatency_NFR_030_DEV_010(t *testing.T) {
	if os.Getenv("PLUX_BENCH_DEVLOOP") != "1" {
		t.Skip("benchmark: run it with make bench-devloop")
	}
	profile, ok := netsim.LookupDevProfile(devloopProfileName)
	if !ok {
		t.Fatalf("no network profile %q", devloopProfileName)
	}
	root := filepath.Join("..", "..", "..")
	project := incrementaltest.ManyPages(t, root, devloopPages, devloopNodes)

	f := newFixture(t, 0)
	svc, err := devsession.New(devsession.Options{
		DB: f.DB, Audit: f.Log, IDs: f.Gen, Limits: func(ctx context.Context, tx pgx.Tx, org, _ string) (limits.Set, error) {
			return f.Limits(ctx, tx, org)
		},
		Tokens: f.signer, Environments: []string{"development"}, PublicBaseURL: "https://plux.example.com", Now: f.Clock,
		Drafts: bigDrafts{project}, Signer: fileSigner{f},
		CompileOptions: func(context.Context, pgx.Tx, string, string, string) (compiler.Options, error) {
			o := compiler.DefaultOptions()
			o.IconFont = func(set icons.Set, names []string) ([]byte, error) {
				return fmt.Appendf(nil, "font %s %v", set, names), nil
			}
			return o, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	f.svc = svc
	r := &relay{fixture: f, used: map[string]bool{}}
	mux := http.NewServeMux()
	mux.Handle("GET "+devsession.SocketPath, svc.SocketHandler(r.proof))
	r.srv = httptest.NewServer(mux)
	t.Cleanup(r.srv.Close)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	network := netsim.New(profile.Profile)
	fwd, err := network.Forward(ctx, "127.0.0.1:0", strings.TrimPrefix(r.srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fwd.Close() })

	s, _ := f.open(t)
	d := r.pairDevice(t, s.ID, "device-1")
	proof := devicetest.SignProof(t, d.key, devicetest.Proof{Method: http.MethodGet, URL: "http://" + fwd.Addr() + devsession.SocketPath})
	h := http.Header{}
	h.Set(devsession.TicketHeader, d.Ticket)
	h.Set("DPoP", proof)
	dialCtx, dialCancel := context.WithTimeout(ctx, wait)
	defer dialCancel()
	conn, resp, err := websocket.Dial(dialCtx, "ws://"+fwd.Addr()+devsession.SocketPath, &websocket.DialOptions{HTTPHeader: h, CompressionMode: websocket.CompressionContextTakeover})
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.CloseNow() })
	conn.SetReadLimit(64 << 20)
	k := &client{c: conn}
	k.send(t, &pluxv1.DevDeviceFrame{Payload: &pluxv1.DevDeviceFrame_Hello{Hello: &pluxv1.DevDeviceHello{}}})

	// The device: reads frames and parses them, as the Dart channel does.
	arrivals := make(chan arrival, 4*devloopEdits)
	var readers sync.WaitGroup
	readers.Go(func() {
		for {
			_, data, err := conn.Read(ctx)
			if err != nil {
				return
			}
			frame := &pluxv1.DevServerFrame{}
			if err := protojson.Unmarshal(data, frame); err != nil {
				t.Errorf("the device cannot parse a frame: %v", err)
				return
			}
			if b := frame.GetBundle(); b != nil {
				arrivals <- arrival{sequence: b.GetBundleSequence(), at: time.Now(), bytes: len(data)}
			}
		}
	})
	t.Cleanup(func() { cancel(); readers.Wait() })

	edit := func(i int) (devloopSample, int) {
		patch := devsession.DocumentPatch{
			Path: incrementaltest.PageFile((i * 7) % devloopPages),
			Ops:  []document.Op{{Op: "replace", Path: editedNode, Value: fmt.Sprintf(`"Month " + string(page.months + %d)`, 1000+i)}},
		}
		start := time.Now()
		res, err := svc.PushPatch(context.Background(), f.Developer, s.ID, []devsession.DocumentPatch{patch})
		returned := time.Now()
		if err != nil {
			t.Fatalf("edit %d: %v", i, err)
		}
		timeout := time.After(wait)
		for {
			select {
			case a := <-arrivals:
				if a.sequence != res.BundleSequence {
					continue
				}
				call := returned.Sub(start)
				server := res.Timings.Compile + res.Timings.Sign + res.Timings.Deliver
				return devloopSample{
					server:    server,
					admit:     call - server,
					transport: max(a.at.Sub(returned), 0),
					total:     a.at.Sub(start),
				}, a.bytes
			case <-timeout:
				t.Fatalf("edit %d: the device did not receive bundle %d", i, res.BundleSequence)
			}
		}
	}

	// The first push loads the draft and compiles it whole, and sends every
	// bundle; the next few warm the caches. None is measured.
	var firstBytes int
	for i := range devloopWarmup {
		_, n := edit(i)
		if i == 0 {
			firstBytes = n
		}
	}
	var samples []devloopSample
	var frameBytes int
	for i := range devloopEdits {
		sample, n := edit(devloopWarmup + i)
		samples = append(samples, sample)
		frameBytes += n
	}

	column := func(get func(devloopSample) time.Duration) []time.Duration {
		out := make([]time.Duration, len(samples))
		for i, s := range samples {
			out[i] = get(s)
		}
		return out
	}
	total := column(func(s devloopSample) time.Duration { return s.total })
	report := map[string]any{
		"requirements":   []string{"NFR-030", "DEV-010"},
		"edits":          devloopEdits,
		"pages":          devloopPages + 2,
		"nodesPerPage":   devloopNodes,
		"profile":        devloopProfileName,
		"server":         stat(column(func(s devloopSample) time.Duration { return s.server })),
		"admit":          stat(column(func(s devloopSample) time.Duration { return s.admit })),
		"transport":      stat(column(func(s devloopSample) time.Duration { return s.transport })),
		"total":          stat(total),
		"budgetMs":       devloopBudget.Milliseconds(),
		"frameBytes":     frameBytes / devloopEdits,
		"firstPushBytes": firstBytes,
	}
	out, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("%s\n", out)
	if p95 := pct(total, 95); p95 > devloopBudget {
		t.Errorf("p95 of the edit-to-device latency is %v, above the budget of %v", p95, devloopBudget)
	}
}
