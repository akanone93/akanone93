// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package experiment_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/controldoc"
	"github.com/nightCode42/plux3/backend/internal/experiment"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
)

func (w *world) flag(x tenant, key string) experiment.FlagRef {
	return experiment.FlagRef{AppID: x.app, EnvironmentID: x.prod, Key: key}
}

func boolSpec(def string, seg ...experiment.SegmentValue) experiment.FlagSpec {
	return experiment.FlagSpec{Type: controldoc.FlagBool, Default: json.RawMessage(def), Segments: seg}
}

// Verifies: ABT-006.
// A flag is created, changed and deleted, and the control documents carry
// exactly what the environment's flags are after each change: the default,
// then the per-segment values in precedence order with the segment's rule.
func TestFlagLifecycleWritesControl_ABT_006(t *testing.T) {
	t.Parallel()
	w := newWorld(t, nil)
	ctx := context.Background()
	f, err := w.svc.CreateFlag(ctx, w.a.admin, w.flag(w.a, "newCheckout"), boolSpec("false",
		experiment.SegmentValue{Segment: "beta-testers", Value: json.RawMessage("true")}))
	if err != nil {
		t.Fatal(err)
	}
	if f.Type != controldoc.FlagBool || string(f.Default) != "false" || len(f.Segments) != 1 {
		t.Fatalf("flag = %+v", f)
	}
	got := w.control.last
	if w.control.env != w.a.prod || len(got.Flags) != 1 {
		t.Fatalf("control after create: env %s, %+v", w.control.env, got)
	}
	want := controldoc.FlagValue{Key: "newCheckout", Type: controldoc.FlagBool, Default: "false", Segments: []controldoc.FlagSegmentValue{
		{Segment: "beta-testers", Rule: controldoc.Rule{Attribute: "os", Operator: controldoc.OpEq, ValueType: controldoc.ValueString, Values: []string{"ios"}}, Value: "true"},
	}}
	if !equalFlag(got.Flags[0], want) {
		t.Fatalf("control flag = %+v, want %+v", got.Flags[0], want)
	}

	// Two segments: the order given is the precedence.
	if _, err := w.svc.UpdateFlag(ctx, w.a.admin, w.flag(w.a, "newCheckout"), boolSpec("true",
		experiment.SegmentValue{Segment: "germans", Value: json.RawMessage("false")},
		experiment.SegmentValue{Segment: "beta-testers", Value: json.RawMessage("true")})); err != nil {
		t.Fatal(err)
	}
	flag := w.control.last.Flags[0]
	if flag.Default != "true" || len(flag.Segments) != 2 || flag.Segments[0].Segment != "germans" || flag.Segments[0].Value != "false" {
		t.Fatalf("control flag after update = %+v", flag)
	}

	if err := w.svc.DeleteFlag(ctx, w.a.admin, w.flag(w.a, "newCheckout")); err != nil {
		t.Fatal(err)
	}
	if len(w.control.last.Flags) != 0 {
		t.Fatalf("control after delete = %+v", w.control.last)
	}
	if _, err := w.svc.GetFlag(ctx, w.a.admin, w.flag(w.a, "newCheckout")); err == nil {
		t.Fatal("a deleted flag was read")
	}
}

func equalFlag(a, b controldoc.FlagValue) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}

// Verifies: ABT-006.
// A flag's type never changes (PLX-8203), whatever else an update says.
func TestFlagTypeChangeRefused_ABT_006(t *testing.T) {
	t.Parallel()
	w := newWorld(t, nil)
	ctx := context.Background()
	if _, err := w.svc.CreateFlag(ctx, w.a.admin, w.flag(w.a, "maxItems"), experiment.FlagSpec{Type: controldoc.FlagNumber, Default: json.RawMessage("10")}); err != nil {
		t.Fatal(err)
	}
	writes := w.control.writes
	_, err := w.svc.UpdateFlag(ctx, w.a.admin, w.flag(w.a, "maxItems"), experiment.FlagSpec{Type: controldoc.FlagString, Default: json.RawMessage(`"10"`)})
	wantCode(t, err, plxerr.FlagTypeChangeRefused)
	if w.control.writes != writes {
		t.Error("a refused type change reached the control documents")
	}
	// Leaving the type out keeps it, and the value is still checked against it.
	if _, err := w.svc.UpdateFlag(ctx, w.a.admin, w.flag(w.a, "maxItems"), experiment.FlagSpec{Default: json.RawMessage("20")}); err != nil {
		t.Fatalf("update without a type: %v", err)
	}
	_, err = w.svc.UpdateFlag(ctx, w.a.admin, w.flag(w.a, "maxItems"), experiment.FlagSpec{Default: json.RawMessage(`"many"`)})
	wantCode(t, err, plxerr.WrongJSONType)
}

// Verifies: ABT-006.
// Values must be of the flag's type; a json flag's must satisfy its schema.
func TestFlagValuesAreTypeChecked_ABT_006(t *testing.T) {
	t.Parallel()
	w := newWorld(t, nil)
	ctx := context.Background()
	create := func(key string, spec experiment.FlagSpec) error {
		_, err := w.svc.CreateFlag(ctx, w.a.admin, w.flag(w.a, key), spec)
		return err
	}
	wantCode(t, create("a", boolSpec(`"yes"`)), plxerr.WrongJSONType)
	wantCode(t, create("b", experiment.FlagSpec{Type: controldoc.FlagNumber, Default: json.RawMessage("true")}), plxerr.WrongJSONType)
	wantCode(t, create("c", experiment.FlagSpec{Type: controldoc.FlagString, Default: json.RawMessage("3")}), plxerr.WrongJSONType)
	wantCode(t, create("d", boolSpec("{")), plxerr.InvalidJSON)
	wantCode(t, create("e", experiment.FlagSpec{Type: "float", Default: json.RawMessage("1")}), plxerr.UnknownFlagType)
	wantCode(t, create("f", boolSpec("true", experiment.SegmentValue{Segment: "nobody", Value: json.RawMessage("true")})), plxerr.UnresolvedReference)
	wantCode(t, create("g", boolSpec("true", experiment.SegmentValue{Segment: "germans", Value: json.RawMessage("1")})), plxerr.WrongJSONType)
	wantCode(t, create("h", boolSpec("true",
		experiment.SegmentValue{Segment: "germans", Value: json.RawMessage("true")},
		experiment.SegmentValue{Segment: "germans", Value: json.RawMessage("false")})), plxerr.DuplicateKey)
	wantCode(t, create("Bad-Key", boolSpec("true")), plxerr.InvalidFormat)
	wantCode(t, create("i", experiment.FlagSpec{Type: controldoc.FlagBool, Default: json.RawMessage("true"), Schema: json.RawMessage(`{"type":"boolean"}`)}), plxerr.InvalidFormat)

	schema := json.RawMessage(`{"type":"object","required":["limit"],"properties":{"limit":{"type":"integer","maximum":5}},"additionalProperties":false}`)
	spec := func(def string) experiment.FlagSpec {
		return experiment.FlagSpec{Type: controldoc.FlagJSON, Schema: schema, Default: json.RawMessage(def)}
	}
	if err := create("limits", spec(`{ "limit": 3 }`)); err != nil {
		t.Fatalf("a value that satisfies the schema: %v", err)
	}
	if got := w.control.last.Flags[0].Default; got != `{"limit":3}` {
		t.Errorf("the default is written canonically, got %s", got)
	}
	wantCode(t, create("limits2", spec(`{"limit": 9}`)), plxerr.InvalidStructure)
	wantCode(t, create("limits3", spec(`{"other": 1}`)), plxerr.InvalidStructure)
	wantCode(t, create("limits4", experiment.FlagSpec{Type: controldoc.FlagJSON, Default: json.RawMessage("{}")}), plxerr.MissingProperty)
	wantCode(t, create("limits5", experiment.FlagSpec{Type: controldoc.FlagJSON, Schema: json.RawMessage(`{"$ref":"https://example.test/x.json"}`), Default: json.RawMessage("{}")}), plxerr.InvalidStructure)
	wantCode(t, create("limits", spec(`{"limit": 1}`)), plxerr.DuplicateKey)
}

// Verifies: ABT-006, LIM-001.
// An app defines at most flags.perApp flags; a key it already has in another
// environment is the same flag and costs nothing.
func TestFlagsPerAppLimit_ABT_006(t *testing.T) {
	t.Parallel()
	w := newWorld(t, nil)
	var err error
	if w.lim, err = limits.Defaults().Tighten(limits.FlagsPerApp, limits.ScopeApp, 2); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, key := range []string{"one", "two"} {
		if _, err := w.svc.CreateFlag(ctx, w.a.admin, w.flag(w.a, key), boolSpec("true")); err != nil {
			t.Fatal(err)
		}
	}
	_, err = w.svc.CreateFlag(ctx, w.a.admin, w.flag(w.a, "three"), boolSpec("true"))
	wantCode(t, err, plxerr.LimitExceeded)
	dev := experiment.FlagRef{AppID: w.a.app, EnvironmentID: w.a.dev, Key: "one"}
	if _, err := w.svc.CreateFlag(ctx, w.a.admin, dev, boolSpec("false")); err != nil {
		t.Fatalf("the same flag in another environment: %v", err)
	}
	// Each environment has its own values.
	prod, _ := w.svc.GetFlag(ctx, w.a.admin, w.flag(w.a, "one"))
	devFlag, _ := w.svc.GetFlag(ctx, w.a.admin, dev)
	if string(prod.Default) != "true" || string(devFlag.Default) != "false" {
		t.Errorf("defaults: production %s, development %s", prod.Default, devFlag.Default)
	}
	list, err := w.svc.ListFlags(ctx, w.a.admin, w.a.app, w.a.prod)
	if err != nil || len(list) != 2 || list[0].Key != "one" || list[1].Key != "two" {
		t.Errorf("ListFlags = %+v, %v", list, err)
	}
}

// Verifies: ABT-006, SEC-102, GOV-003.
// Flags are changed with flags.manage on the environment and nothing less,
// and one organisation cannot touch another's.
func TestFlagAuthorisation_ABT_006(t *testing.T) {
	t.Parallel()
	w := newWorld(t, nil)
	ctx := context.Background()
	reader := w.a.as(auth.AppRead)
	if _, err := w.svc.CreateFlag(ctx, reader, w.flag(w.a, "x"), boolSpec("true")); err == nil {
		t.Fatal("a reader created a flag")
	} else {
		wantCode(t, err, plxerr.PermissionDenied)
	}
	scoped := w.a.as(auth.AppRead)
	scoped.EnvPermissions = map[auth.EnvScope][]auth.Permission{{AppID: w.a.app, Environment: "development"}: {auth.FlagsManage}}
	if _, err := w.svc.CreateFlag(ctx, scoped, experiment.FlagRef{AppID: w.a.app, EnvironmentID: w.a.dev, Key: "x"}, boolSpec("true")); err != nil {
		t.Fatalf("flags.manage on development: %v", err)
	}
	_, err := w.svc.CreateFlag(ctx, scoped, w.flag(w.a, "y"), boolSpec("true"))
	wantCode(t, err, plxerr.PermissionDenied)
	if _, err := w.svc.GetFlag(ctx, w.other.admin, experiment.FlagRef{AppID: w.other.app, EnvironmentID: w.a.prod, Key: "x"}); err == nil {
		t.Error("an environment of another app was read")
	}
	if _, err := w.svc.ListFlags(ctx, w.other.admin, w.a.app, w.a.prod); err == nil {
		t.Error("another organisation listed the flags")
	}
}
