// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
)

const dsrUsage = "Usage: plux dsr export|erase|status|map [flags]\n"

// dsr exports and erases the data held about a device or a pseudonymous user
// and shows where personal data is stored (SEC-162). It needs dsr.manage.
func (e env) dsr(args []string) int {
	var kind, out string
	c, rest, code, ok := e.governanceCmd("dsr", dsrUsage, args, map[string]subUsage{
		"export": {"Usage: plux dsr export --kind device|user [--out file] <subject-id>\n\nWrites a JSON archive of everything held about the device or pseudonymous user, one section per store, to the file or to standard output.", 1},
		"erase":  {"Usage: plux dsr erase --kind device|user [--json] <subject-id>\n\nErases everything held about the device or pseudonymous user. Audit entries stay; they name the subject by hash only. Repeating it changes nothing.", 1},
		"status": {"Usage: plux dsr status [--json] [<request-id>]\n\nShows one completed request, or the latest ones.", -1},
		"map":    {"Usage: plux dsr map [--json]\n\nLists the stores of personal data, their retention and what an erasure does to each.", 0},
	}, func(set *flag.FlagSet) {
		set.StringVar(&kind, "kind", "", "what the subject identifier names: `device` or user")
		set.StringVar(&out, "out", "", "the `file` an export is written to; standard output without it")
	})
	if !ok {
		return code
	}
	opts, err := e.authOptions(c)
	if err != nil {
		return e.fail("dsr", err)
	}
	client := pluxv1connect.NewDataSubjectServiceClient(e.httpClient, c.server, opts)
	ctx := context.Background()
	switch args[0] {
	case "export":
		return e.dsrExport(ctx, client, kind, rest[0], out)
	case "erase":
		return e.dsrErase(ctx, client, c, kind, rest[0])
	case "status":
		return e.dsrStatus(ctx, client, c, rest)
	default: // map
		return e.dsrMap(ctx, client, c)
	}
}

// subjectKind converts the --kind flag.
func subjectKind(kind string) (pluxv1.SubjectKind, error) {
	switch kind {
	case "device":
		return pluxv1.SubjectKind_SUBJECT_KIND_DEVICE, nil
	case "user":
		return pluxv1.SubjectKind_SUBJECT_KIND_USER, nil
	default:
		return pluxv1.SubjectKind_SUBJECT_KIND_UNSPECIFIED, usageError("--kind must be device or user")
	}
}

// dsrExport writes the archive of a subject to a file or standard output.
func (e env) dsrExport(ctx context.Context, client pluxv1connect.DataSubjectServiceClient, kind, id, out string) int {
	k, err := subjectKind(kind)
	if err != nil {
		return e.fail("dsr export", err)
	}
	res, err := client.ExportSubject(ctx, connect.NewRequest(&pluxv1.ExportSubjectRequest{Kind: k, SubjectId: id}))
	if err != nil {
		return e.fail("dsr export", err)
	}
	if out == "" {
		_, _ = e.stdout.Write(res.Msg.GetArchive())
		_, _ = fmt.Fprintln(e.stdout)
		return exitOK
	}
	if err := os.WriteFile(out, res.Msg.GetArchive(), 0o600); err != nil {
		return e.fail("dsr export", err)
	}
	_, _ = fmt.Fprintf(e.stderr, "Wrote %d rows to %s (request %s)\n", res.Msg.GetRowCount(), out, res.Msg.GetRequestId())
	return exitOK
}

// dsrErase erases a subject and reports the stores that lost rows.
func (e env) dsrErase(ctx context.Context, client pluxv1connect.DataSubjectServiceClient, c common, kind, id string) int {
	k, err := subjectKind(kind)
	if err != nil {
		return e.fail("dsr erase", err)
	}
	res, err := client.EraseSubject(ctx, connect.NewRequest(&pluxv1.EraseSubjectRequest{Kind: k, SubjectId: id}))
	if err != nil {
		return e.fail("dsr erase", err)
	}
	m := res.Msg
	text := "Already erased; nothing left to remove"
	if !m.GetAlreadyErased() {
		text = "Erased request " + m.GetRequestId() + "\n" + storeCountLines(m.GetErased())
	}
	return e.emit(c.json, map[string]any{
		"requestId": m.GetRequestId(), "alreadyErased": m.GetAlreadyErased(), "erased": storeCountViews(m.GetErased()),
	}, text)
}

// dsrStatus shows one request, or the newest ones.
func (e env) dsrStatus(ctx context.Context, client pluxv1connect.DataSubjectServiceClient, c common, rest []string) int {
	if len(rest) > 1 {
		return e.fail("dsr status", usageError("at most one request identifier"))
	}
	if len(rest) == 1 {
		res, err := client.GetRequest(ctx, connect.NewRequest(&pluxv1.GetRequestRequest{Id: rest[0]}))
		if err != nil {
			return e.fail("dsr status", err)
		}
		r := res.Msg.GetRequest()
		return e.emit(c.json, dsrRequestView(r), dsrRequestLine(r)+storeCountLines(r.GetTombstones()))
	}
	views := []map[string]any{}
	var text strings.Builder
	if err := eachPage(func(token string) (string, error) {
		res, err := client.ListRequests(ctx, connect.NewRequest(&pluxv1.ListRequestsRequest{Page: &pluxv1.Page{PageSize: 100, PageToken: token}}))
		if err != nil {
			return "", err //nolint:wrapcheck // e.fail names the command
		}
		for _, r := range res.Msg.GetRequests() {
			views = append(views, dsrRequestView(r))
			text.WriteString(dsrRequestLine(r))
		}
		return res.Msg.GetPage().GetNextPageToken(), nil
	}); err != nil {
		return e.fail("dsr status", err)
	}
	return e.emit(c.json, map[string]any{"requests": views}, text.String())
}

// dsrMap prints the personal-data map.
func (e env) dsrMap(ctx context.Context, client pluxv1connect.DataSubjectServiceClient, c common) int {
	res, err := client.GetPersonalDataMap(ctx, connect.NewRequest(&pluxv1.GetPersonalDataMapRequest{}))
	if err != nil {
		return e.fail("dsr map", err)
	}
	views := []map[string]any{}
	var text strings.Builder
	for _, s := range res.Msg.GetStores() {
		views = append(views, map[string]any{
			"name": s.GetName(), "description": s.GetDescription(), "deviceKey": s.GetDeviceKey(), "userKey": s.GetUserKey(),
			"personalFields": nonNil(s.GetPersonalFields()), "retention": s.GetRetention(), "erase": s.GetErase(), "note": s.GetNote(),
		})
		fmt.Fprintf(&text, "%s\t%s\t%s\n", s.GetName(), s.GetErase(), s.GetRetention())
	}
	return e.emit(c.json, map[string]any{"stores": views}, text.String())
}

// storeCountViews converts per-store counts for JSON output.
func storeCountViews(counts []*pluxv1.StoreCount) []map[string]any {
	out := []map[string]any{}
	for _, c := range counts {
		out = append(out, map[string]any{"store": c.GetStore(), "rows": c.GetRows()})
	}
	return out
}

// storeCountLines renders per-store counts, one per line.
func storeCountLines(counts []*pluxv1.StoreCount) string {
	var b strings.Builder
	for _, c := range counts {
		fmt.Fprintf(&b, "  %s\t%d rows\n", c.GetStore(), c.GetRows())
	}
	return b.String()
}

// dsrRequestView converts a request for JSON output.
func dsrRequestView(r *pluxv1.DataSubjectRequest) map[string]any {
	return map[string]any{
		"id": r.GetId(), "action": r.GetAction(), "kind": r.GetKind().String(), "subjectHash": r.GetSubjectHash(),
		"actor": r.GetActor(), "rowCount": r.GetRowCount(), "archiveBytes": r.GetArchiveBytes(),
		"requestedAt": r.GetRequestedAt().AsTime(), "tombstones": storeCountViews(r.GetTombstones()),
	}
}

// dsrRequestLine renders a request on one line.
func dsrRequestLine(r *pluxv1.DataSubjectRequest) string {
	return fmt.Sprintf("%s\t%s\t%s\t%d rows\t%s\n", r.GetId(), r.GetAction(), r.GetKind(), r.GetRowCount(), r.GetRequestedAt().AsTime().Format("2006-01-02T15:04:05Z"))
}
