// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"connectrpc.com/connect"

	"github.com/nightCode42/plux3/backend/internal/netsim"
	"github.com/nightCode42/plux3/backend/internal/pluxv1"
)

// devRequest is a command request under construction.
type devRequest = pluxv1.DevSessionServiceSendCommandRequest

// devEnumValue finds the value of an enum by its lower-case, dash-separated
// name, such as memory-pressure for DEV_LIFECYCLE_EVENT_MEMORY_PRESSURE.
func devEnumValue(values map[string]int32, prefix, name string) (int32, bool) {
	v, ok := values[prefix+strings.ToUpper(strings.ReplaceAll(name, "-", "_"))]
	return v, ok
}

// devEnumNames lists the names an enum accepts, sorted.
func devEnumNames(values map[string]int32, prefix string) string {
	var names []string
	for k, v := range values {
		if v != 0 && strings.HasPrefix(k, prefix) {
			names = append(names, strings.ToLower(strings.ReplaceAll(strings.TrimPrefix(k, prefix), "_", "-")))
		}
	}
	slices.Sort(names)
	return strings.Join(names, ", ")
}

// devOpen opens a screen (DEV-011).
func (e env) devOpen(args []string) int {
	var params, mocks multiFlag
	var locale, theme string
	var scale float64
	d, pos, code, ok := e.devParse("open", args, "Usage: plux dev open <route> [--param k=v]... [--locale tag] [--theme light|dark|system] [--text-scale factor] [--mock source=state]... [--device id] [--session id]\n\nOpens a screen on the device with the given parameters, locale, theme, text scale and data-source mock states.", 1, func(set *flag.FlagSet) {
		set.Var(&params, "param", "a route parameter `key=value`; repeatable")
		set.Var(&mocks, "mock", "a mock selection `source=state`; repeatable")
		set.StringVar(&locale, "locale", "", "a BCP 47 `tag`")
		set.StringVar(&theme, "theme", "", "`light`, dark or system")
		set.Float64Var(&scale, "text-scale", 0, "the text scale `factor`")
	})
	if !ok {
		return code
	}
	cmd := &pluxv1.OpenScreenCommand{Route: pos[0], Locale: locale, TextScale: scale, Params: map[string]string{}}
	for _, p := range params {
		k, v, found := strings.Cut(p, "=")
		if !found || k == "" {
			return e.fail("dev open", usageError("--param is key=value"))
		}
		cmd.Params[k] = v
	}
	for _, m := range mocks {
		src, state, found := strings.Cut(m, "=")
		if !found || src == "" || state == "" {
			return e.fail("dev open", usageError("--mock is source=state"))
		}
		cmd.Mocks = append(cmd.Mocks, &pluxv1.MockSelection{Source: src, State: state})
	}
	if theme != "" {
		v, found := devEnumValue(pluxv1.DevThemeMode_value, "DEV_THEME_MODE_", theme)
		if !found || v == 0 {
			return e.fail("dev open", usageError("--theme is light, dark or system"))
		}
		cmd.Theme = pluxv1.DevThemeMode(v)
	}
	if scale < 0 {
		return e.fail("dev open", usageError("--text-scale is positive"))
	}
	return e.devSend(d, "open", &devRequest{Command: &pluxv1.DevSessionServiceSendCommandRequest_OpenScreen{OpenScreen: cmd}}, "")
}

// devSet sets a state value (DEV-031).
func (e env) devSet(args []string) int {
	d, pos, code, ok := e.devParse("set", args, "Usage: plux dev set <scope> <path> <json> [--device id] [--session id]\n\nSets a state value. The scope is app, page:<route> or component:<node path>; the value is JSON. The device refuses a path tagged sensitive (PLX-9206).", 3, nil)
	if !ok {
		return code
	}
	if !json.Valid([]byte(pos[2])) {
		return e.fail("dev set", usageError("the value is not valid JSON"))
	}
	return e.devSend(d, "set", &devRequest{Command: &pluxv1.DevSessionServiceSendCommandRequest_SetState{
		SetState: &pluxv1.SetStateCommand{Scope: pos[0], Path: pos[1], Value: []byte(pos[2])},
	}}, "")
}

// devMock switches a data-source mock (DAT-080).
func (e env) devMock(args []string) int {
	d, pos, code, ok := e.devParse("mock", args, "Usage: plux dev mock <source> <state> [--device id] [--session id]\n\nSwitches the state of a data-source mock.", 2, nil)
	if !ok {
		return code
	}
	return e.devSend(d, "mock", &devRequest{Command: &pluxv1.DevSessionServiceSendCommandRequest_SwitchMock{
		SwitchMock: &pluxv1.SwitchMockCommand{Source: pos[0], State: pos[1]},
	}}, "")
}

// devClock fakes the clock of a device (DEV-031).
func (e env) devClock(args []string) int {
	d, pos, code, ok := e.devParse("clock", args, "Usage: plux dev clock set <time> | advance <duration> | reset [--device id] [--session id]\n\nset fixes the clock at an RFC 3339 time or at Unix microseconds; advance moves a faked clock forward by a duration such as 90m or by microseconds; reset returns to the real clock.", -1, nil)
	if !ok {
		return code
	}
	cmd := &pluxv1.FakeClockCommand{}
	switch {
	case len(pos) == 1 && pos[0] == "reset":
		cmd.Operation = &pluxv1.FakeClockCommand_Reset_{Reset_: true}
	case len(pos) == 2 && pos[0] == "set":
		micros, err := parseClockInstant(pos[1])
		if err != nil {
			return e.fail("dev clock", err)
		}
		cmd.Operation = &pluxv1.FakeClockCommand_SetUnixMicros{SetUnixMicros: micros}
	case len(pos) == 2 && pos[0] == "advance":
		micros, err := parseClockStep(pos[1])
		if err != nil {
			return e.fail("dev clock", err)
		}
		cmd.Operation = &pluxv1.FakeClockCommand_AdvanceMicros{AdvanceMicros: micros}
	default:
		return e.fail("dev clock", usageError("use clock set <time>, clock advance <duration> or clock reset"))
	}
	return e.devSend(d, "clock", &devRequest{Command: &pluxv1.DevSessionServiceSendCommandRequest_FakeClock{FakeClock: cmd}}, "")
}

// parseClockInstant reads an RFC 3339 time or Unix microseconds.
func parseClockInstant(v string) (int64, error) {
	if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
		return t.UnixMicro(), nil
	}
	if n, err := strconv.ParseInt(v, 10, 64); err == nil {
		return n, nil
	}
	return 0, usageError("the time is RFC 3339 or Unix microseconds")
}

// parseClockStep reads a positive duration or a number of microseconds.
func parseClockStep(v string) (int64, error) {
	if d, err := time.ParseDuration(v); err == nil && d > 0 {
		return d.Microseconds(), nil
	}
	if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
		return n, nil
	}
	return 0, usageError("the step is a positive duration such as 90m, or microseconds")
}

// devLifecycle simulates a lifecycle event (DEV-031).
func (e env) devLifecycle(args []string) int {
	d, pos, code, ok := e.devParse("lifecycle", args, "Usage: plux dev lifecycle pause|resume|inactive|detached|memory-pressure [--device id] [--session id]\n\nMakes the device simulate a lifecycle event.", 1, nil)
	if !ok {
		return code
	}
	v, found := devEnumValue(pluxv1.DevLifecycleEvent_value, "DEV_LIFECYCLE_EVENT_", pos[0])
	if !found || v == 0 {
		return e.fail("dev lifecycle", usageError("the event is one of "+devEnumNames(pluxv1.DevLifecycleEvent_value, "DEV_LIFECYCLE_EVENT_")))
	}
	return e.devSend(d, "lifecycle", &devRequest{Command: &pluxv1.DevSessionServiceSendCommandRequest_Lifecycle{
		Lifecycle: &pluxv1.LifecycleCommand{Event: pluxv1.DevLifecycleEvent(v)},
	}}, "")
}

// devNetwork applies a network profile (DEV-031).
func (e env) devNetwork(args []string) int {
	d, pos, code, ok := e.devParse("network", args, "Usage: plux dev network <profile> [--device id] [--session id]\n\nApplies a network profile to the device; good_wifi removes the simulation. Profiles: "+networkProfileNames()+".", 1, nil)
	if !ok {
		return code
	}
	if _, found := netsim.LookupDevProfile(pos[0]); !found {
		return e.fail("dev network", usageError("the profile is one of "+networkProfileNames()))
	}
	return e.devSend(d, "network", &devRequest{Command: &pluxv1.DevSessionServiceSendCommandRequest_NetworkProfile{
		NetworkProfile: &pluxv1.NetworkProfileCommand{ProfileId: pos[0]},
	}}, "")
}

// networkProfileNames lists the profiles of dev sessions.
func networkProfileNames() string {
	names := make([]string, 0, len(netsim.DevProfiles))
	for _, p := range netsim.DevProfiles {
		names = append(names, p.ID)
	}
	return strings.Join(names, ", ")
}

// devSecurityDetect simulates a detection of plux_security (SEC-071).
func (e env) devSecurityDetect(args []string) int {
	const prefix = "DEV_SECURITY_DETECTION_KIND_"
	d, pos, code, ok := e.devParse("security-detect", args, "Usage: plux dev security-detect <kind> [--device id] [--session id]\n\nMakes plux_security report a detection, to try the app's response to it. Kinds: "+devEnumNames(pluxv1.DevSecurityDetectionKind_value, prefix)+".", 1, nil)
	if !ok {
		return code
	}
	v, found := devEnumValue(pluxv1.DevSecurityDetectionKind_value, prefix, pos[0])
	if !found || v == 0 {
		return e.fail("dev security-detect", usageError("the kind is one of "+devEnumNames(pluxv1.DevSecurityDetectionKind_value, prefix)))
	}
	return e.devSend(d, "security-detect", &devRequest{Command: &pluxv1.DevSessionServiceSendCommandRequest_SimulateSecurityDetection{
		SimulateSecurityDetection: &pluxv1.SimulateSecurityDetectionCommand{Kind: pluxv1.DevSecurityDetectionKind(v)},
	}}, "")
}

// devPXL evaluates a PXL expression on the device (DEV-032).
func (e env) devPXL(args []string) int {
	d, pos, code, ok := e.devParse("pxl", args, "Usage: plux dev pxl <expression> [--device id] [--session id]\n\nEvaluates a PXL expression against the device's live state and prints the redacted result.", 1, nil)
	if !ok {
		return code
	}
	return e.devSend(d, "pxl", &devRequest{Command: &pluxv1.DevSessionServiceSendCommandRequest_EvaluatePxl{
		EvaluatePxl: &pluxv1.EvaluatePxlCommand{Expression: pos[0]},
	}}, "")
}

// devScreenshot captures the screen (DEV-033).
func (e env) devScreenshot(args []string) int {
	var out string
	d, _, code, ok := e.devParse("screenshot", args, "Usage: plux dev screenshot --out file.png [--device id] [--session id]\n\nCaptures the screen and writes the PNG. For several devices, each file gets the device's identifier before the extension. Platform views (maps, native slots) are not captured.", 0, func(set *flag.FlagSet) {
		set.StringVar(&out, "out", "", "the `file` to write the PNG to")
	})
	if !ok {
		return code
	}
	if out == "" {
		return e.fail("dev screenshot", usageError("--out names the PNG file"))
	}
	return e.devSend(d, "screenshot", &devRequest{Command: &pluxv1.DevSessionServiceSendCommandRequest_CaptureScreenshot{
		CaptureScreenshot: &pluxv1.CaptureScreenshotCommand{},
	}}, out)
}

// devInspect highlights a node (DEV-030).
func (e env) devInspect(args []string) int {
	d, pos, code, ok := e.devParse("inspect", args, "Usage: plux dev inspect <node-path> [--device id] [--session id]\n\nHighlights a node of the running screen on the device.", 1, nil)
	if !ok {
		return code
	}
	return e.devSend(d, "inspect", &devRequest{Command: &pluxv1.DevSessionServiceSendCommandRequest_InspectSelect{
		InspectSelect: &pluxv1.InspectSelectCommand{NodePath: pos[0]},
	}}, "")
}

// devOverlay switches a debugging overlay (DEV-034).
func (e env) devOverlay(args []string) int {
	const prefix = "DEV_OVERLAY_"
	d, pos, code, ok := e.devParse("overlay", args, "Usage: plux dev overlay <name> on|off [--device id] [--session id]\n\nSwitches a debugging overlay on the device. Overlays: "+devEnumNames(pluxv1.DevOverlay_value, prefix)+".", 2, nil)
	if !ok {
		return code
	}
	v, found := devEnumValue(pluxv1.DevOverlay_value, prefix, pos[0])
	if !found || v == 0 {
		return e.fail("dev overlay", usageError("the overlay is one of "+devEnumNames(pluxv1.DevOverlay_value, prefix)))
	}
	if pos[1] != "on" && pos[1] != "off" {
		return e.fail("dev overlay", usageError("say on or off"))
	}
	return e.devSend(d, "overlay", &devRequest{Command: &pluxv1.DevSessionServiceSendCommandRequest_ToggleOverlay{
		ToggleOverlay: &pluxv1.ToggleOverlayCommand{Overlay: pluxv1.DevOverlay(v), Enabled: pos[1] == "on"},
	}}, "")
}

// devOutcome is what one device answered to a command.
type devOutcome struct {
	Device string          `json:"device"`
	Status string          `json:"status"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
	File   string          `json:"file,omitempty"`
	// answered, imaged and ok track what has arrived from the device.
	answered, imaged, ok bool
	// single is true when the command went to one device only.
	single bool
}

// finished reports whether nothing more is awaited from the device: it
// answered, and either failed or, for a screenshot, sent the image.
func (o *devOutcome) finished(wantImage bool) bool {
	return o.answered && (!wantImage || o.imaged || !o.ok)
}

// devResultText renders the answer of a device to a command.
func devResultText(r *pluxv1.CommandResultEvent) string {
	s := devStatusOf(r.GetStatus())
	if v := devValue(r.GetResult()); v != "" {
		s += " " + v
	}
	if r.GetError() != nil {
		s += " " + devErrorText(r.GetError())
	}
	return r.GetCommandId() + " " + s
}

func devStatusOf(s pluxv1.DevCommandStatus) string {
	return strings.ToLower(strings.TrimPrefix(s.String(), "DEV_COMMAND_STATUS_"))
}

// devSend sends a command and waits, for at most devResultWait, for the
// answer of each device it went to (DEV-011). A screenshot also waits for
// the image and writes it to shot.
func (e env) devSend(d *devCall, name string, req *devRequest, shot string) int {
	cmd := "dev " + name
	cl, session, err := e.devConnect(d)
	if err != nil {
		return e.fail(cmd, err)
	}
	ctx, cancel := e.context()
	req.SessionId, req.DeviceId = session, d.device

	// Watch alongside the send. The server answers the call only with its
	// first event, so the watch cannot be awaited; it replays what the
	// server retains, so no answer can slip past it.
	defer cancel()
	events := make(chan *pluxv1.DevEvent, 64)
	streamErr := make(chan error, 1)
	go func() {
		defer close(events)
		stream, err := cl.dev.WatchEvents(ctx, connect.NewRequest(&pluxv1.DevSessionServiceWatchEventsRequest{SessionId: session}))
		if err != nil {
			streamErr <- err
			return
		}
		defer func() { _ = stream.Close() }()
		for stream.Receive() {
			select {
			case events <- stream.Msg().GetEvent():
			case <-ctx.Done():
				streamErr <- nil
				return
			}
		}
		streamErr <- stream.Err()
	}()

	sent, err := cl.dev.SendCommand(ctx, connect.NewRequest(req))
	if err != nil {
		return e.fail(cmd, err)
	}
	id, devices := sent.Msg.GetCommandId(), slices.Clone(sent.Msg.GetDeviceIds())
	slices.Sort(devices)
	if !d.c.json {
		_, _ = fmt.Fprintf(e.stdout, "command %s sent to %s\n", id, orDash(strings.Join(devices, ", ")))
	}
	outcomes, err := e.devAwait(ctx, events, streamErr, id, devices, shot, d.c.json)
	failed := err != nil
	for _, o := range outcomes {
		failed = failed || !o.ok
	}
	if d.c.json {
		if code := e.emit(true, map[string]any{"commandId": id, "devices": devices, "results": outcomes}, ""); code != exitOK {
			return code
		}
	}
	if err != nil {
		return e.fail(cmd, err)
	}
	if failed {
		return exitFailed
	}
	return exitOK
}

// devAwait collects the answers of the devices to command id and prints
// each as it arrives.
func (e env) devAwait(ctx context.Context, events <-chan *pluxv1.DevEvent, streamErr <-chan error, id string, devices []string, shot string, jsonOut bool) ([]*devOutcome, error) {
	outcomes := make([]*devOutcome, len(devices))
	byDevice := map[string]*devOutcome{}
	for i, dev := range devices {
		outcomes[i] = &devOutcome{Device: dev, single: len(devices) == 1}
		byDevice[dev] = outcomes[i]
	}
	timeout := time.NewTimer(devResultWait)
	defer timeout.Stop()
	for slices.ContainsFunc(outcomes, func(o *devOutcome) bool { return !o.finished(shot != "") }) {
		select {
		case ev, open := <-events:
			if !open {
				return outcomes, devStreamEnded(ctx, streamErr, id)
			}
			if o := byDevice[ev.GetDeviceId()]; o != nil && !o.finished(shot != "") {
				if err := e.devAnswer(o, ev, id, shot, jsonOut); err != nil {
					return outcomes, err
				}
			}
		case <-timeout.C:
			return outcomes, devSilent(outcomes, shot != "")
		case <-ctx.Done():
			return outcomes, fmt.Errorf("waiting for command %s: %w", id, ctx.Err())
		}
	}
	return outcomes, nil
}

// devStreamEnded explains why the watch ended before every device answered.
func devStreamEnded(ctx context.Context, streamErr <-chan error, id string) error {
	if err := <-streamErr; err != nil && ctx.Err() == nil {
		return err
	}
	return fmt.Errorf("the event stream ended before every device answered command %s", id)
}

// devSilent is the error for the devices that did not answer in time.
func devSilent(outcomes []*devOutcome, wantImage bool) error {
	var silent []string
	for _, o := range outcomes {
		if !o.finished(wantImage) {
			silent = append(silent, o.Device)
		}
	}
	return fmt.Errorf("no answer from %s within %s", strings.Join(silent, ", "), devResultWait)
}

// devAnswer notes an event of a device that may answer command id: its
// result or, for a screenshot, its image.
func (e env) devAnswer(o *devOutcome, ev *pluxv1.DevEvent, id, shot string, jsonOut bool) error {
	switch p := ev.GetPayload().(type) {
	case *pluxv1.DevEvent_CommandResult:
		if p.CommandResult.GetCommandId() != id {
			return nil
		}
		o.record(p.CommandResult)
		if !jsonOut {
			_, _ = fmt.Fprintf(e.stdout, "%s: %s\n", o.Device, strings.TrimPrefix(devResultText(p.CommandResult), id+" "))
		}
	case *pluxv1.DevEvent_Screenshot:
		if shot == "" || p.Screenshot.GetCommandId() != id {
			return nil
		}
		path := shotPath(shot, o.Device, 1)
		if !o.single {
			path = shotPath(shot, o.Device, 2)
		}
		if err := os.WriteFile(path, p.Screenshot.GetPng(), 0o600); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
		o.File, o.imaged = path, true
		if !jsonOut {
			note := ""
			if p.Screenshot.GetPlatformViewsNotCaptured() {
				note = " (platform views are not in the image)"
			}
			_, _ = fmt.Fprintf(e.stdout, "%s: wrote %s (%dx%d)%s\n", o.Device, path, p.Screenshot.GetWidth(), p.Screenshot.GetHeight(), note)
		}
	}
	return nil
}

// record notes the answer of a device.
func (o *devOutcome) record(r *pluxv1.CommandResultEvent) {
	o.answered = true
	o.Status = devStatusOf(r.GetStatus())
	o.ok = r.GetStatus() == pluxv1.DevCommandStatus_DEV_COMMAND_STATUS_OK
	if res := r.GetResult(); len(res) > 0 {
		if json.Valid(res) {
			o.Result = json.RawMessage(res)
		} else {
			quoted, _ := json.Marshal(string(res))
			o.Result = quoted
		}
	}
	if r.GetError() != nil {
		o.Error = devErrorText(r.GetError())
	}
}

// shotPath names the file of one device's screenshot: the path itself for a
// single device, else with the device before the extension.
func shotPath(out, device string, devices int) string {
	if devices <= 1 {
		return out
	}
	ext := filepath.Ext(out)
	return strings.TrimSuffix(out, ext) + "-" + device + ext
}
