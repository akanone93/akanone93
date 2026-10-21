// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package walkthrough

import (
	"strconv"
	"strings"
	"testing"
	"time"
)

// fleetReport is a plux-load report in the shape the walkthrough reads.
const fleetReport = `{"schema":"plux.load.report/1","registered":3,"operations":{"manifest":{"requests":10,"errors":1},"control":{"requests":5,"errors":0}},"controlApplied":6}`

// The walkthrough reads registered devices, summed requests and errors, and
// the control applications from plux-load's report.
func TestParseFleetReport(t *testing.T) {
	t.Parallel()
	rep, err := ParseFleetReport([]byte(fleetReport))
	if err != nil {
		t.Fatal(err)
	}
	if rep.Registered != 3 || rep.Requests != 15 || rep.Errors != 1 || rep.ControlApplied != 6 || string(rep.Raw) != fleetReport {
		t.Errorf("report = %+v", rep)
	}
	if _, err := ParseFleetReport([]byte("not json")); err == nil {
		t.Error("a malformed report must be an error")
	}
}

// fakeLoad is a plux-load that records its flags, writes its device list and
// prints a report.
func fakeLoad(t *testing.T, exit int) string {
	t.Helper()
	return writeScript(t, `ids=""
args="$*"
while [ $# -gt 0 ]; do
  if [ "$1" = "-device-ids-file" ]; then ids="$2"; fi
  shift
done
printf 'dev-1\ndev-2\ndev-3\n' > "$ids"
echo "$args" > "$ids.args"
echo '`+fleetReport+`'
echo "plux-load: too many errors" >&2
exit `+strconv.Itoa(exit)+`
`)
}

// The fleet is started with the spec's flags, its device IDs are available
// once registered, and Wait returns the parsed report.
func TestExecFleetStartsPluxLoad(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	f := &ExecFleet{Binary: fakeLoad(t, 0), Server: "https://s", App: "app1", CAFile: "/ca.pem", Dir: dir, Seed: 7}
	run, err := f.Start(t.Context(), FleetSpec{Devices: 50, Duration: 90 * time.Second, Env: "staging", InjectErrors: true, ErrorRelease: 12, MaxErrorRatio: 0.01})
	if err != nil {
		t.Fatal(err)
	}
	ids, err := run.DeviceIDs(t.Context())
	if err != nil || strings.Join(ids, ",") != "dev-1,dev-2,dev-3" {
		t.Fatalf("DeviceIDs = %v, %v", ids, err)
	}
	rep, err := run.Wait()
	if err != nil || rep.Registered != 3 || rep.ControlApplied != 6 {
		t.Fatalf("Wait = %+v, %v", rep, err)
	}
	args := readFile(t, dir+"/devices-1.txt.args")
	for _, want := range []string{"-server https://s", "-app app1", "-env staging", "-devices 50", "-duration 1m30s", "-max-error-ratio 0.01", "-seed 7", "-ca-file /ca.pem", "-inject-errors -error-release 12"} {
		if !strings.Contains(args, want) {
			t.Errorf("flags %q lack %q", args, want)
		}
	}
	second, err := f.Start(t.Context(), FleetSpec{Devices: 1, Duration: time.Second, Env: "staging"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := second.Wait(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(readFile(t, dir+"/devices-2.txt.args"), "-inject-errors") {
		t.Error("a healthy fleet must not inject errors")
	}
}

// A fleet that exits non-zero is an error that still carries its report and
// its own message, so that an upgrade with failed requests fails the step.
func TestExecFleetReportsAFailedRun(t *testing.T) {
	t.Parallel()
	f := &ExecFleet{Binary: fakeLoad(t, 1), Server: "https://s", App: "a", Dir: t.TempDir()}
	run, err := f.Start(t.Context(), FleetSpec{Devices: 1, Duration: time.Second, Env: "staging"})
	if err != nil {
		t.Fatal(err)
	}
	rep, err := run.Wait()
	if err == nil || !strings.Contains(err.Error(), "too many errors") || rep.Errors != 1 {
		t.Errorf("Wait = %+v, %v", rep, err)
	}
}

// A fleet that ends before its devices registered says why instead of
// waiting for ever.
func TestExecFleetDeviceIDsFailWhenTheFleetDies(t *testing.T) {
	t.Parallel()
	f := &ExecFleet{Binary: writeScript(t, "echo 'plux-load: no device registered' >&2\nexit 1\n"), Server: "s", App: "a", Dir: t.TempDir()}
	run, err := f.Start(t.Context(), FleetSpec{Devices: 1, Duration: time.Second, Env: "staging"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := run.DeviceIDs(t.Context()); err == nil || !strings.Contains(err.Error(), "no device registered") {
		t.Errorf("err = %v", err)
	}
	if _, err := (&ExecFleet{Binary: "/nonexistent", Dir: t.TempDir()}).Start(t.Context(), FleetSpec{}); err == nil {
		t.Error("a binary that cannot start must be an error")
	}
}
