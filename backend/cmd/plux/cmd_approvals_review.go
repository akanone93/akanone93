// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"path/filepath"
	"regexp"
	"strings"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
)

// rawJSON presents a document the server returned as JSON in --json output.
func rawJSON(b []byte) json.RawMessage {
	if len(b) == 0 {
		return json.RawMessage("null")
	}
	return json.RawMessage(b)
}

// approvalShow prints a request with its review bundle and writes the
// evidence next to it: what an approver looks at before deciding (GOV-023).
func (e env) approvalShow(ctx context.Context, client pluxv1connect.ApprovalServiceClient, c common, id, out string) int {
	res, err := client.GetApprovalReview(ctx, connect.NewRequest(&pluxv1.GetApprovalReviewRequest{AppId: c.app, Id: id}))
	if err != nil {
		return e.fail("approvals show", err)
	}
	review := res.Msg
	if out == "" {
		out = "approval-" + id
	}
	files, err := writeReview(out, review)
	if err != nil {
		return e.fail("approvals show", err)
	}
	view := approvalView(review.GetApproval())
	view["risk"] = map[string]any{"level": review.GetRisk().GetLevel(), "reasons": nonNil(review.GetRisk().GetReasons())}
	view["changelog"], view["notes"], view["written"] = nonNil(review.GetChangelog()), review.GetNotes(), files
	return e.emit(c.json, view, reviewText(review, out))
}

// reviewText summarises a review for a terminal.
func reviewText(r *pluxv1.GetApprovalReviewResponse, dir string) string {
	a := r.GetApproval()
	var b strings.Builder
	b.WriteString(approvalLine(a))
	fmt.Fprintf(&b, "risk: %s\n", r.GetRisk().GetLevel())
	for _, reason := range r.GetRisk().GetReasons() {
		fmt.Fprintf(&b, "  - %s\n", reason)
	}
	for _, line := range r.GetChangelog() {
		fmt.Fprintf(&b, "change: %s\n", line)
	}
	devices := int64(0)
	for _, e := range r.GetInstallBase() {
		devices += e.GetDevices()
	}
	fmt.Fprintf(&b, "devices in the environment: %d; golden differences: %d\n", devices, len(r.GetGoldenDiffs()))
	fmt.Fprintf(&b, "review written to %s\n", filepath.Join(dir, "review.html"))
	return b.String()
}

// unsafeName matches what a file name taken from the server may not hold.
var unsafeName = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// fileName makes a name from the server safe to use in a path.
func fileName(s string) string {
	if s = unsafeName.ReplaceAllString(s, "_"); s == "" || s == "." || s == ".." {
		return "_"
	}
	return s
}

// writeReview writes the evidence of a review under dir and returns the
// paths written, relative to it: the diff, the reports, each golden image
// with a pixel diff, and review.html, which links them.
func writeReview(dir string, r *pluxv1.GetApprovalReviewResponse) ([]string, error) {
	var written []string
	put := func(rel string, content []byte) error {
		written = append(written, rel)
		return writeUnder(dir, filepath.Join(dir, filepath.FromSlash(rel)), content)
	}
	if len(r.GetDiff()) > 0 {
		if err := put("diff.json", r.GetDiff()); err != nil {
			return nil, err
		}
	}
	for _, rep := range r.GetReports() {
		if err := put("reports/"+fileName(rep.GetKind())+".json", rep.GetContent()); err != nil {
			return nil, err
		}
	}
	images, err := writeGolden(put, r.GetGoldenDiffs())
	if err != nil {
		return nil, err
	}
	page, err := reviewPage(r, written, images)
	if err != nil {
		return nil, err
	}
	if err := put("review.html", page); err != nil {
		return nil, err
	}
	return written, nil
}

// goldenFiles are the files written for one golden-image difference.
type goldenFiles struct {
	Case                    string
	Baseline, Actual, Delta string
}

// writeGolden writes the images of each difference and a pixel diff of
// those that have a baseline.
func writeGolden(put func(string, []byte) error, diffs []*pluxv1.ReviewImage) ([]goldenFiles, error) {
	var out []goldenFiles
	for _, d := range diffs {
		base := "golden/" + fileName(d.GetPluginKey()) + "/" + fileName(d.GetCaseId())
		g := goldenFiles{Case: d.GetPluginKey() + " " + d.GetCaseId(), Actual: base + "-actual.png"}
		if err := put(g.Actual, d.GetActual()); err != nil {
			return nil, err
		}
		if len(d.GetBaseline()) > 0 {
			g.Baseline, g.Delta = base+"-baseline.png", base+"-diff.png"
			delta, err := pixelDiff(d.GetBaseline(), d.GetActual())
			if err != nil {
				return nil, fmt.Errorf("compare the images of %s: %w", g.Case, err)
			}
			if err := put(g.Baseline, d.GetBaseline()); err != nil {
				return nil, err
			}
			if err := put(g.Delta, delta); err != nil {
				return nil, err
			}
		}
		out = append(out, g)
	}
	return out, nil
}

// pixelDiff renders where two PNG images differ: unchanged pixels faded, the
// others red. The image is as large as the larger of the two.
func pixelDiff(baseline, actual []byte) ([]byte, error) {
	a, err := png.Decode(bytes.NewReader(baseline))
	if err != nil {
		return nil, fmt.Errorf("decode the baseline: %w", err)
	}
	b, err := png.Decode(bytes.NewReader(actual))
	if err != nil {
		return nil, fmt.Errorf("decode the new image: %w", err)
	}
	bounds := a.Bounds().Union(b.Bounds())
	out := image.NewRGBA(bounds)
	draw.Draw(out, bounds, image.White, image.Point{}, draw.Src)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			out.Set(x, y, diffPixel(a, b, x, y))
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, out); err != nil {
		return nil, fmt.Errorf("encode the difference: %w", err)
	}
	return buf.Bytes(), nil
}

// diffPixel is the colour of the difference at a point.
func diffPixel(a, b image.Image, x, y int) color.Color {
	pa, pb := image.Pt(x, y), image.Pt(x, y)
	inA, inB := pa.In(a.Bounds()), pb.In(b.Bounds())
	if inA && inB && color.RGBAModel.Convert(a.At(x, y)) == color.RGBAModel.Convert(b.At(x, y)) {
		return color.RGBA{R: 0xee, G: 0xee, B: 0xee, A: 0xff}
	}
	return color.RGBA{R: 0xd3, G: 0x2f, B: 0x2f, A: 0xff}
}

// reviewTemplate is review.html: a standalone page, escaped by html/template.
var reviewTemplate = template.Must(template.New("review").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><title>Approval {{.A.GetId}}</title>
<style>body{font:15px/1.5 system-ui,sans-serif;max-width:60rem;margin:2rem auto;padding:0 1rem}
table{border-collapse:collapse}td,th{border:1px solid #ccc;padding:.25rem .6rem;text-align:left}
img{max-width:18rem;border:1px solid #ccc}.high{color:#b71c1c}.medium{color:#e65100}</style></head><body>
<h1>{{.A.GetSummary}}</h1>
<p>{{.A.GetResource}} / {{.A.GetAction}} / {{.A.GetSubject}} in {{if .A.GetEnvironment}}{{.A.GetEnvironment}}{{else}}no environment{{end}}<br>
Status <strong>{{.A.GetStatus}}</strong>, {{.A.GetApprovalsRequired}} approval(s) required by {{range .A.GetPolicyKeys}}{{.}} {{end}}<br>
Content <code>{{.A.GetContentHash}}</code><br>Requested by {{.A.GetRequestedBy}}, expires {{.A.GetExpiresAt.AsTime}}</p>
<h2>Risk: <span class="{{.R.GetLevel}}">{{.R.GetLevel}}</span></h2><ul>{{range .R.GetReasons}}<li>{{.}}</li>{{else}}<li>none computed</li>{{end}}</ul>
<h2>Changes</h2><ul>{{range .Changelog}}<li>{{.}}</li>{{else}}<li>none</li>{{end}}</ul>
{{if .Notes}}<p>Notes: {{.Notes}}</p>{{end}}
<h2>Install base</h2><table><tr><th>Channel</th><th>Release</th><th>Host build</th><th>Devices</th></tr>
{{range .Base}}<tr><td>{{.GetChannel}}</td><td>{{.GetReleaseSequence}}</td><td>{{.GetHostBuild}}</td><td>{{.GetDevices}}</td></tr>{{else}}<tr><td colspan="4">no devices</td></tr>{{end}}</table>
<h2>Golden images that differ</h2>{{range .Golden}}<h3>{{.Case}}</h3><p>{{if .Baseline}}<a href="{{.Baseline}}"><img src="{{.Baseline}}" alt="baseline"></a> {{end}}<a href="{{.Actual}}"><img src="{{.Actual}}" alt="new"></a>{{if .Delta}} <a href="{{.Delta}}"><img src="{{.Delta}}" alt="difference"></a>{{end}}</p>{{else}}<p>none</p>{{end}}
<h2>Files</h2><ul>{{range .Files}}<li><a href="{{.}}">{{.}}</a></li>{{end}}</ul>
<h2>Decisions</h2><table><tr><th>Who</th><th>Decision</th><th>When</th><th>Signed</th><th>Comment</th></tr>
{{range .A.GetDecisions}}<tr><td>{{.GetDecider}}</td><td>{{.GetDecision}}</td><td>{{.GetDecidedAt.AsTime}}</td><td>{{if .GetSignature}}yes{{else}}pending{{end}}</td><td>{{.GetComment}}</td></tr>{{else}}<tr><td colspan="5">none yet</td></tr>{{end}}</table>
</body></html>
`))

// reviewPage renders review.html.
func reviewPage(r *pluxv1.GetApprovalReviewResponse, files []string, golden []goldenFiles) ([]byte, error) {
	var buf bytes.Buffer
	err := reviewTemplate.Execute(&buf, map[string]any{
		"A": r.GetApproval(), "R": r.GetRisk(), "Changelog": r.GetChangelog(), "Notes": r.GetNotes(),
		"Base": r.GetInstallBase(), "Golden": golden, "Files": files,
	})
	if err != nil {
		return nil, fmt.Errorf("render the review: %w", err)
	}
	return buf.Bytes(), nil
}
