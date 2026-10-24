// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package devsession

import (
	"testing"

	"github.com/nightCode42/plux3/backend/internal/pluxv1"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/pxl"
)

func evaluateRequest(expression string, program []byte) *pluxv1.DevSessionServiceSendCommandRequest {
	return &pluxv1.DevSessionServiceSendCommandRequest{Command: &pluxv1.DevSessionServiceSendCommandRequest_EvaluatePxl{
		EvaluatePxl: &pluxv1.EvaluatePxlCommand{Expression: expression, Program: program},
	}}
}

func TestPrepareConsole_CompilesExpression_DEV_032(t *testing.T) {
	req := evaluateRequest(`page["title"] + "!"`, nil)
	if err := prepareConsole(req); err != nil {
		t.Fatalf("prepareConsole: %v", err)
	}
	program := req.GetEvaluatePxl().GetProgram()
	if len(program) == 0 {
		t.Fatal("no program set")
	}
	if _, err := pxl.Decode(program); err != nil {
		t.Fatalf("the program does not decode: %v", err)
	}
}

func TestPrepareConsole_RefusesAssignment_DEV_032(t *testing.T) {
	for _, expr := range []string{`page["title"] = "x"`, `page.title = 1`, `state["n"] := 2`} {
		req := evaluateRequest(expr, []byte("client"))
		err := prepareConsole(req)
		if err == nil {
			t.Fatalf("%q compiled", expr)
		}
		if code, ok := plxerr.CodeOf(err); !ok || code.String() == "" {
			t.Errorf("%q: no PLX code in %v", expr, err)
		}
		if len(req.GetEvaluatePxl().GetProgram()) != 0 {
			t.Errorf("%q: a program stayed on the refused command", expr)
		}
	}
}

func TestPrepareConsole_IgnoresClientProgram_DEV_032(t *testing.T) {
	forged := []byte("forged program")
	req := evaluateRequest(`app["name"]`, forged)
	if err := prepareConsole(req); err != nil {
		t.Fatalf("prepareConsole: %v", err)
	}
	got := req.GetEvaluatePxl().GetProgram()
	if string(got) == string(forged) {
		t.Fatal("the client's program was kept")
	}
	if _, err := pxl.Decode(got); err != nil {
		t.Fatalf("the program does not decode: %v", err)
	}
}

func TestPrepareConsole_Roots_DEV_032(t *testing.T) {
	req := evaluateRequest(`page["a"]`, nil)
	req.GetEvaluatePxl().Roots = []string{"app"}
	if err := prepareConsole(req); err == nil {
		t.Fatal("an expression read a root that was not offered")
	}
	req = evaluateRequest(`app["a"]`, nil)
	req.GetEvaluatePxl().Roots = []string{"secrets"}
	if err := prepareConsole(req); err == nil {
		t.Fatal("an unknown root was accepted")
	}
}

func TestPrepareConsole_OtherCommandsPass_DEV_032(t *testing.T) {
	req := &pluxv1.DevSessionServiceSendCommandRequest{Command: &pluxv1.DevSessionServiceSendCommandRequest_CaptureScreenshot{}}
	if err := prepareConsole(req); err != nil {
		t.Fatal(err)
	}
}
