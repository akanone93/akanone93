// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/encoding/protodelim"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
)

// devEventKind names the payload of an event, such as action_trace.
func devEventKind(ev *pluxv1.DevEvent) string {
	m := ev.ProtoReflect()
	if f := m.WhichOneof(m.Descriptor().Oneofs().ByName("payload")); f != nil {
		return string(f.Name())
	}
	return ""
}

// devEventLine renders an event: its sequence, time, device and kind, then a
// summary. A payload with parts, such as an action's steps, continues on
// indented lines.
func devEventLine(ev *pluxv1.DevEvent) string {
	at := time.UnixMicro(ev.GetTimeUnixMicros()).UTC().Format("15:04:05.000")
	device := ev.GetDeviceId()
	if device == "" {
		device = "-"
	}
	head, more := devSummary(ev)
	var b strings.Builder
	fmt.Fprintf(&b, "%d %s %s %s %s", ev.GetSequence(), at, device, devEventKind(ev), head)
	for _, l := range more {
		b.WriteString("\n  " + l)
	}
	return b.String()
}

// devMicros renders a duration given in microseconds.
func devMicros(us int64) string {
	return (time.Duration(us) * time.Microsecond).String()
}

// devValue renders a JSON value of an event on one line, cut when long.
func devValue(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	var b bytes.Buffer
	if json.Compact(&b, raw) != nil {
		b.Reset()
		b.Write(raw)
	}
	const most = 160
	if b.Len() > most {
		return b.String()[:most] + "…"
	}
	return b.String()
}

// devErrorText renders a failure of the stream.
func devErrorText(e *pluxv1.DevError) string {
	out := e.GetCode()
	if e.GetMessage() != "" {
		out += " " + e.GetMessage()
	}
	return strings.TrimSpace(out)
}

func devStatusName(s pluxv1.DevRunStatus) string {
	return strings.ToLower(strings.TrimPrefix(s.String(), "DEV_RUN_STATUS_"))
}

func devLevelName(l pluxv1.DevLogLevel) string {
	return strings.ToLower(strings.TrimPrefix(l.String(), "DEV_LOG_LEVEL_"))
}

// devSummary returns the one-line summary of an event and its further lines.
func devSummary(ev *pluxv1.DevEvent) (string, []string) {
	switch p := ev.GetPayload().(type) {
	case *pluxv1.DevEvent_Log:
		return devLogText(p.Log), nil
	case *pluxv1.DevEvent_ActionTrace:
		return devActionTrace(p.ActionTrace)
	case *pluxv1.DevEvent_StateDiff:
		return devStateDiffText(p.StateDiff), nil
	case *pluxv1.DevEvent_NetworkCall:
		return devNetworkText(p.NetworkCall), nil
	case *pluxv1.DevEvent_FunctionCall:
		return devFunctionCall(p.FunctionCall)
	case *pluxv1.DevEvent_Sync:
		return devSyncText(p.Sync), nil
	case *pluxv1.DevEvent_FrameTiming:
		return devFrameText(p.FrameTiming), nil
	case *pluxv1.DevEvent_Error:
		return devErrorEventText(p.Error), nil
	case *pluxv1.DevEvent_SecurityDetection:
		return devDetectionText(p.SecurityDetection), nil
	case *pluxv1.DevEvent_CommandResult:
		return devResultText(p.CommandResult), nil
	case *pluxv1.DevEvent_HotApply:
		return devHotApplyText(p.HotApply), nil
	case *pluxv1.DevEvent_Screenshot:
		return fmt.Sprintf("%dx%d, %d bytes", p.Screenshot.GetWidth(), p.Screenshot.GetHeight(), len(p.Screenshot.GetPng())), nil
	case *pluxv1.DevEvent_Inspect:
		return p.Inspect.GetNodePath() + " " + p.Inspect.GetWidget(), nil
	case *pluxv1.DevEvent_Drop:
		return fmt.Sprintf("%d events dropped (%s)", p.Drop.GetDropped(), strings.ToLower(strings.TrimPrefix(p.Drop.GetReason().String(), "DEV_DROP_REASON_"))), nil
	}
	return "", nil
}

func devLogText(l *pluxv1.LogEvent) string {
	s := fmt.Sprintf("%s %s: %s", devLevelName(l.GetLevel()), l.GetSource(), l.GetMessage())
	if l.GetNodePath() != "" {
		s += " (" + l.GetNodePath() + ")"
	}
	return s
}

func devStateDiffText(d *pluxv1.StateDiffEvent) string {
	var parts []string
	for _, c := range d.GetChanges() {
		switch {
		case c.GetRedacted():
			parts = append(parts, c.GetPath()+" (redacted)")
		case len(c.GetNewValue()) == 0 && len(c.GetOldValue()) == 0:
			parts = append(parts, c.GetPath())
		default:
			parts = append(parts, fmt.Sprintf("%s %s -> %s", c.GetPath(), devValue(c.GetOldValue()), devValue(c.GetNewValue())))
		}
	}
	s := d.GetScope() + ": " + strings.Join(parts, "; ")
	if d.GetTruncated() {
		s += " (truncated)"
	}
	return s
}

func devNetworkText(n *pluxv1.NetworkCallEvent) string {
	s := fmt.Sprintf("%s %s %d %s", n.GetMethod(), n.GetUrlTemplate(), n.GetStatus(), devMicros(n.GetTotalMicros()))
	if n.GetError() != nil {
		s += " " + devErrorText(n.GetError())
	}
	return s
}

func devSyncText(y *pluxv1.SyncEvent) string {
	s := strings.ToLower(strings.TrimPrefix(y.GetPhase().String(), "DEV_SYNC_PHASE_"))
	if y.GetPlugin() != "" {
		s += " " + y.GetPlugin()
	}
	if y.GetReleaseSequence() != 0 {
		s += fmt.Sprintf(" release %d", y.GetReleaseSequence())
	}
	s += fmt.Sprintf(" %d bytes %s", y.GetBytesTransferred(), devMicros(y.GetDurationMicros()))
	if y.GetError() != nil {
		s += " " + devErrorText(y.GetError())
	}
	return s
}

func devFrameText(f *pluxv1.FrameTimingEvent) string {
	janky := 0
	for _, fr := range f.GetFrames() {
		if fr.GetJanky() {
			janky++
		}
	}
	return fmt.Sprintf("%s %d frames, %d janky", f.GetRoute(), len(f.GetFrames()), janky)
}

func devErrorEventText(e *pluxv1.ErrorEvent) string {
	s := strings.ToLower(strings.TrimPrefix(e.GetSeverity().String(), "SEVERITY_")) + " " + devErrorText(e.GetError())
	if e.GetNodePath() != "" {
		s += " (" + e.GetNodePath() + ")"
	}
	if e.GetFatal() {
		s += " fatal"
	}
	return s
}

func devDetectionText(d *pluxv1.SecurityDetectionEvent) string {
	s := strings.ToLower(strings.TrimPrefix(d.GetKind().String(), "DEV_SECURITY_DETECTION_KIND_")) + " response " + d.GetResponse()
	if d.GetSimulated() {
		s += " (simulated)"
	}
	return s
}

func devHotApplyText(h *pluxv1.HotApplyEvent) string {
	s := fmt.Sprintf("bundle %s applied in %s", shortHash(h.GetBundleSha256()), devMicros(h.GetApplyMicros()))
	if len(h.GetPagesApplied()) > 0 {
		s += ", pages " + strings.Join(h.GetPagesApplied(), ", ")
	}
	if len(h.GetScopesReset()) > 0 {
		s += ", state reset: " + strings.Join(h.GetScopesReset(), ", ")
	}
	return s
}

// devActionTrace renders an action run: its header and a tree of its steps,
// the steps off the main branch one level deeper.
func devActionTrace(t *pluxv1.ActionTraceEvent) (string, []string) {
	head := fmt.Sprintf("%s %s on %s %s", t.GetGraph(), t.GetTrigger(), orDash(t.GetNodePath()), devStatusName(t.GetStatus()))
	if end := t.GetEndUnixMicros(); end > 0 {
		head += " " + devMicros(end-t.GetStartUnixMicros())
	}
	if t.GetError() != nil {
		head += " " + devErrorText(t.GetError())
	}
	var lines []string
	for _, s := range t.GetSteps() {
		indent := ""
		if s.GetBranch() != "" && s.GetBranch() != "main" {
			indent = "  "
		}
		line := fmt.Sprintf("%s%s %s", indent, s.GetStepId(), s.GetAction())
		if s.GetBranch() != "" {
			line += " [" + s.GetBranch() + "]"
		}
		line += " " + devStatusName(s.GetStatus()) + " " + devMicros(s.GetDurationMicros())
		if s.GetError() != nil {
			line += " " + devErrorText(s.GetError())
		}
		lines = append(lines, line)
	}
	return head, lines
}

// devFunctionCall renders a function call with the log lines merged into it.
func devFunctionCall(f *pluxv1.FunctionCallEvent) (string, []string) {
	s := fmt.Sprintf("%s@v%d %s %s", f.GetFunction(), f.GetVersion(), f.GetPlacement(), devMicros(f.GetDurationMicros()))
	if f.GetReplayed() {
		s += " replayed"
	}
	if f.GetBusinessErrorCode() != "" {
		s += " business error " + f.GetBusinessErrorCode()
	}
	if f.GetError() != nil {
		s += " " + devErrorText(f.GetError())
	}
	var lines []string
	for _, l := range f.GetLogs() {
		lines = append(lines, fmt.Sprintf("| %s %s", devLevelName(l.GetLevel()), l.GetMessage()))
	}
	return s, lines
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// devEventMatches reports whether an event passes --filter: the name of its
// payload kind (action_trace, action-trace or action trace), or text that
// its rendering contains, in any case.
func devEventMatches(ev *pluxv1.DevEvent, filter string) bool {
	if filter == "" {
		return true
	}
	f := strings.ToLower(filter)
	if strings.NewReplacer("-", "_", " ", "_").Replace(f) == devEventKind(ev) {
		return true
	}
	return strings.Contains(strings.ToLower(devEventLine(ev)), f)
}

// devPrintEvent prints an event as a line of text or as one line of proto3
// JSON.
func devPrintEvent(w io.Writer, ev *pluxv1.DevEvent, jsonOut bool) error {
	if !jsonOut {
		_, err := fmt.Fprintln(w, devEventLine(ev))
		return err //nolint:wrapcheck // a write error
	}
	return devPrintJSON(w, ev)
}

// devPrintJSON prints a message as one compact line of proto3 JSON. The
// protojson encoder varies its spacing between builds; compacting makes the
// line the same everywhere.
func devPrintJSON(w io.Writer, m proto.Message) error {
	raw, err := protojson.Marshal(m)
	if err != nil {
		return fmt.Errorf("encode an event: %w", err)
	}
	var b bytes.Buffer
	if err := json.Compact(&b, raw); err != nil {
		return fmt.Errorf("encode an event: %w", err)
	}
	b.WriteByte('\n')
	_, err = w.Write(b.Bytes())
	return err //nolint:wrapcheck // a write error
}

// devLogs watches the events of a session (DEV-021).
func (e env) devLogs(args []string) int {
	var filter string
	var since uint64
	d, _, code, ok := e.devParse("logs", args, "Usage: plux dev logs [--session id] [--device id] [--filter kind|text] [--since seq] [--json]\n\nPrints the session's events, one line each, until interrupted: sequence, time, device, kind and a summary. An action run is followed by its steps; a function call by the log lines merged into it. --filter keeps the events of one payload kind (log, action_trace, state_diff, network_call, function_call, sync, frame_timing, error, security_detection, command_result, hot_apply, screenshot, inspect, drop) or whose line contains the text. --since resumes after a sequence number. With --json, each event is one line of proto3 JSON.", 0, func(set *flag.FlagSet) {
		set.StringVar(&filter, "filter", "", "only events of this payload `kind` or containing this text")
		set.Uint64Var(&since, "since", 0, "resume after this event `sequence`")
	})
	if !ok {
		return code
	}
	cl, session, err := e.devConnect(d)
	if err != nil {
		return e.fail("dev logs", err)
	}
	ctx, cancel := e.context()
	defer cancel()
	req := &pluxv1.DevSessionServiceWatchEventsRequest{SessionId: session, AfterSequence: since}
	if d.device != "" {
		req.DeviceIds = []string{d.device}
	}
	stream, err := cl.dev.WatchEvents(ctx, connect.NewRequest(req))
	if err != nil {
		return e.fail("dev logs", err)
	}
	defer func() { _ = stream.Close() }()
	for stream.Receive() {
		ev := stream.Msg().GetEvent()
		if !devEventMatches(ev, filter) {
			continue
		}
		if err := devPrintEvent(e.stdout, ev, d.c.json); err != nil {
			return e.fail("dev logs", err)
		}
	}
	if err := stream.Err(); err != nil && ctx.Err() == nil {
		return e.fail("dev logs", err)
	}
	return exitOK
}

// devTrace runs plux dev trace.
func (e env) devTrace(args []string) int {
	const usage = "Usage: plux dev trace export --out file [--session id] [--device id]\n       plux dev trace show [--filter kind|text] [--json] <file>\n"
	if len(args) > 0 {
		switch args[0] {
		case "export":
			return e.devTraceExport(args[1:])
		case "show":
			return e.devTraceShow(args[1:])
		}
	}
	_, _ = fmt.Fprint(e.stderr, usage)
	return exitUsage
}

// devTraceExport saves the session's retained events as a trace file
// (DEV-021).
func (e env) devTraceExport(args []string) int {
	var out string
	d, _, code, ok := e.devParse("trace export", args, "Usage: plux dev trace export --out file [--session id] [--device id]\n\nWrites the session's retained events, redacted as the live stream is, to a gzip-compressed trace file for a bug report. Read it with plux dev trace show.", 0, func(set *flag.FlagSet) {
		set.StringVar(&out, "out", "", "the trace `file` to write")
	})
	if !ok {
		return code
	}
	if out == "" {
		return e.fail("dev trace export", usageError("--out names the file to write"))
	}
	cl, session, err := e.devConnect(d)
	if err != nil {
		return e.fail("dev trace export", err)
	}
	req := &pluxv1.DevSessionServiceExportTraceRequest{SessionId: session}
	if d.device != "" {
		req.DeviceIds = []string{d.device}
	}
	res, err := cl.dev.ExportTrace(context.Background(), connect.NewRequest(req))
	if err != nil {
		return e.fail("dev trace export", err)
	}
	if err := os.WriteFile(out, res.Msg.GetTrace(), 0o600); err != nil {
		return e.fail("dev trace export", fmt.Errorf("write %s: %w", out, err))
	}
	return e.emit(d.c.json, map[string]any{"file": out, "events": res.Msg.GetEventCount(), "bytes": len(res.Msg.GetTrace())},
		fmt.Sprintf("wrote %s (%d events, %d bytes)", out, res.Msg.GetEventCount(), len(res.Msg.GetTrace())))
}

// devTraceShow prints a trace file like plux dev logs, without a server.
func (e env) devTraceShow(args []string) int {
	var filter string
	var jsonOut bool
	set := flag.NewFlagSet("dev trace show", flag.ContinueOnError)
	set.StringVar(&filter, "filter", "", "only events of this payload `kind` or containing this text")
	set.BoolVar(&jsonOut, "json", false, "print each event as one line of proto3 JSON")
	pos, code, ok := parseFn(set, args, e.stderr, "Usage: plux dev trace show [--filter kind|text] [--json] <file>\n\nReads a trace file offline and prints its header and its events like plux dev logs. A file that cannot be read fails with PLX-9233.", 1)
	if !ok {
		return code
	}
	header, events, err := readTrace(pos[0])
	if err != nil {
		return e.fail("dev trace show", err)
	}
	if jsonOut {
		if err := devPrintJSON(e.stdout, header); err != nil {
			return e.fail("dev trace show", err)
		}
	} else {
		_, _ = fmt.Fprintf(e.stdout, "trace of session %s: app %s, environment %s, exported %s by server %s, %d events\n",
			header.GetSessionId(), orDash(header.GetAppKey()), orDash(header.GetEnvironmentKey()),
			header.GetExportedAt().AsTime().UTC().Format(time.RFC3339), orDash(header.GetServerVersion()), len(events))
	}
	for _, ev := range events {
		if !devEventMatches(ev, filter) {
			continue
		}
		if err := devPrintEvent(e.stdout, ev, jsonOut); err != nil {
			return e.fail("dev trace show", err)
		}
	}
	return exitOK
}

// traceFormatVersion is the newest trace file format this reader knows.
const traceFormatVersion = 1

// readTrace reads a trace file: a gzip stream of length-prefixed messages,
// a header and then events. Any fault is PLX-9233.
func readTrace(path string) (*pluxv1.TraceFileHeader, []*pluxv1.DevEvent, error) {
	f, err := os.Open(path) //nolint:gosec // G304: a file the user names.
	if err != nil {
		return nil, nil, plxerr.Wrap(plxerr.TraceFileUnreadable, err, "open the trace file")
	}
	defer func() { _ = f.Close() }()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return nil, nil, plxerr.Wrap(plxerr.TraceFileUnreadable, err, "%s is not a gzip-compressed trace file", path)
	}
	defer func() { _ = zr.Close() }()
	def, _ := limits.Lookup(limits.DevtoolsEventBytes)
	read := protodelim.UnmarshalOptions{MaxSize: def.Max}
	br := bufio.NewReader(zr)
	header := &pluxv1.TraceFileHeader{}
	if err := read.UnmarshalFrom(br, header); err != nil {
		return nil, nil, plxerr.Wrap(plxerr.TraceFileUnreadable, err, "read the header of %s", path)
	}
	if header.GetFormatVersion() == 0 || header.GetFormatVersion() > traceFormatVersion {
		return nil, nil, plxerr.New(plxerr.TraceFileUnreadable, "%s has format version %d; this reader knows version %d", path, header.GetFormatVersion(), traceFormatVersion)
	}
	var events []*pluxv1.DevEvent
	for {
		ev := &pluxv1.DevEvent{}
		err := read.UnmarshalFrom(br, ev)
		if errors.Is(err, io.EOF) {
			return header, events, nil
		}
		if err != nil {
			return nil, nil, plxerr.Wrap(plxerr.TraceFileUnreadable, err, "read event %d of %s", len(events)+1, path)
		}
		events = append(events, ev)
	}
}
