// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"flag"
	"fmt"
	"strings"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/pluxv1/pluxv1connect"
)

const auditUsage = "Usage: plux audit reconstruct|reconstructions [flags]\n"

// audit reconstructs what a user saw and lists the reconstructions made
// (SEC-142). It needs reconstruction.view on the app.
func (e env) audit(args []string) int {
	var app, deviceID, userRef, at string
	c, rest, code, ok := e.governanceCmd("audit", auditUsage, args, map[string]subUsage{
		"reconstruct":     {"Usage: plux audit reconstruct --app <app> (--device <id> | --user <id>) --at <time> [--json]\n\nMakes a signed, read-only reconstruction release of what the device, or the device the pseudonymous user was on, ran at the moment (RFC 3339). Open it in the viewer host with the printed URI. Fails with PLX-8502 when the moment is past reconstruction.retentionDays or its bundles are no longer retained.", 0},
		"reconstructions": {"Usage: plux audit reconstructions --app <app> [--json] [<reconstruction-id>]\n\nLists the app's reconstruction releases, newest first; with an identifier, opens one and prints its manifest. Opening is audited.", -1},
	}, func(set *flag.FlagSet) {
		set.StringVar(&app, "app", "", "the `app` identifier")
		set.StringVar(&deviceID, "device", "", "the `device` identifier to reconstruct")
		set.StringVar(&userRef, "user", "", "the host's pseudonymous `user` identifier whose device to reconstruct")
		set.StringVar(&at, "at", "", "the moment to reconstruct, as an RFC 3339 `time`")
	})
	if !ok {
		return code
	}
	opts, err := e.authOptions(c)
	if err != nil {
		return e.fail("audit", err)
	}
	client := pluxv1connect.NewAuditServiceClient(e.httpClient, c.server, opts)
	ctx := context.Background()
	if args[0] == "reconstruct" {
		return e.auditReconstruct(ctx, client, c, app, deviceID, userRef, at)
	}
	if len(rest) > 1 {
		return e.fail("audit reconstructions", usageError("at most one reconstruction identifier"))
	}
	if len(rest) == 1 {
		return e.auditOpen(ctx, client, c, rest[0])
	}
	return e.auditList(ctx, client, c, app)
}

// auditReconstruct makes a reconstruction release.
func (e env) auditReconstruct(ctx context.Context, client pluxv1connect.AuditServiceClient, c common, app, deviceID, userRef, at string) int {
	if app == "" || at == "" || (deviceID == "") == (userRef == "") {
		return e.fail("audit reconstruct", usageError("--app, --at and exactly one of --device and --user are required"))
	}
	moment, err := time.Parse(time.RFC3339, at)
	if err != nil {
		return e.fail("audit reconstruct", usageError("--at must be an RFC 3339 time, such as 2026-03-01T12:00:00Z"))
	}
	res, err := client.Reconstruct(ctx, connect.NewRequest(&pluxv1.ReconstructRequest{
		AppId: app, DeviceId: deviceID, UserRef: userRef, At: timestamppb.New(moment),
	}))
	if err != nil {
		return e.fail("audit reconstruct", err)
	}
	r := res.Msg.GetReconstruction()
	return e.emit(c.json, reconstructionView(r), fmt.Sprintf("Reconstruction %s (%s)\nOpen it in the viewer: %s\n", r.GetId(), r.GetState(), r.GetOpenUri()))
}

// auditOpen opens one reconstruction release and prints its manifest.
func (e env) auditOpen(ctx context.Context, client pluxv1connect.AuditServiceClient, c common, id string) int {
	res, err := client.GetReconstruction(ctx, connect.NewRequest(&pluxv1.GetReconstructionRequest{Id: id}))
	if err != nil {
		return e.fail("audit reconstructions", err)
	}
	m := res.Msg
	bundles := []map[string]any{}
	for _, b := range m.GetBundles() {
		bundles = append(bundles, map[string]any{"key": b.GetKey(), "sha256": b.GetSha256(), "size": b.GetSize(), "url": b.GetUrl()})
	}
	view := reconstructionView(m.GetReconstruction())
	view["manifest"] = string(m.GetManifest())
	view["signatures"] = len(m.GetSignatures())
	view["bundles"] = bundles
	text := reconstructionLine(m.GetReconstruction())
	if len(m.GetManifest()) > 0 {
		text += string(m.GetManifest()) + "\n"
	}
	return e.emit(c.json, view, text)
}

// auditList lists the reconstruction releases of an app.
func (e env) auditList(ctx context.Context, client pluxv1connect.AuditServiceClient, c common, app string) int {
	if app == "" {
		return e.fail("audit reconstructions", usageError("--app is required"))
	}
	views := []map[string]any{}
	var text strings.Builder
	if err := eachPage(func(token string) (string, error) {
		res, err := client.ListReconstructions(ctx, connect.NewRequest(&pluxv1.ListReconstructionsRequest{
			AppId: app, Page: &pluxv1.Page{PageSize: 100, PageToken: token},
		}))
		if err != nil {
			return "", err //nolint:wrapcheck // e.fail names the command
		}
		for _, r := range res.Msg.GetReconstructions() {
			views = append(views, reconstructionView(r))
			text.WriteString(reconstructionLine(r))
		}
		return res.Msg.GetPage().GetNextPageToken(), nil
	}); err != nil {
		return e.fail("audit reconstructions", err)
	}
	return e.emit(c.json, map[string]any{"reconstructions": views}, text.String())
}

// reconstructionView converts a reconstruction release for JSON output.
func reconstructionView(r *pluxv1.Reconstruction) map[string]any {
	return map[string]any{
		"id": r.GetId(), "appId": r.GetAppId(), "environmentId": r.GetEnvironmentId(), "deviceId": r.GetDeviceId(),
		"userRef": r.GetUserRef(), "moment": r.GetMoment().AsTime(), "releaseSequence": r.GetReleaseSequence(),
		"state": r.GetState(), "createdAt": r.GetCreatedAt().AsTime(), "createdBy": r.GetCreatedBy(), "openUri": r.GetOpenUri(),
	}
}

// reconstructionLine renders a reconstruction release on one line.
func reconstructionLine(r *pluxv1.Reconstruction) string {
	return fmt.Sprintf("%s\t%s\trelease %d\t%s\t%s\n", r.GetId(), r.GetState(), r.GetReleaseSequence(),
		r.GetMoment().AsTime().Format("2006-01-02T15:04:05Z"), r.GetDeviceId())
}
