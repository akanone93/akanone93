// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

//go:build js && wasm

// Command plux-validator-wasm is a measurement prototype (Studio S0.8): it
// compiles the page validator to WebAssembly so a browser worker can call
// it. It exposes one JavaScript function, pluxValidatePage.
package main

import (
	"encoding/base64"
	"encoding/json"
	"syscall/js"
	"testing/fstest"

	"github.com/nightCode42/plux3/backend/internal/compiler"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
)

// validator is the project loaded by the last call that passed files. It
// lives for the lifetime of the WebAssembly instance, which is single
// threaded.
var validator *compiler.Validator

// pluxValidatePage(files, file, data) loads the project in files when files
// is a non-empty array of {path, data, encoding?} (encoding "base64" marks a
// binary file), then validates data as the page at
// file and returns the diagnostics as JSON. An empty file only loads the
// project and returns the project's own diagnostics.
func pluxValidatePage(_ js.Value, args []js.Value) any {
	if len(args) != 3 {
		return errorJSON("pluxValidatePage takes (files, file, data)")
	}
	var loadDiags plxerr.Diagnostics
	if files := args[0]; files.Type() == js.TypeObject && files.Length() > 0 {
		fsys := make(fstest.MapFS, files.Length())
		for i := range files.Length() {
			f := files.Index(i)
			data := []byte(f.Get("data").String())
			if f.Get("encoding").String() == "base64" {
				decoded, err := base64.StdEncoding.DecodeString(f.Get("data").String())
				if err != nil {
					return errorJSON("file " + f.Get("path").String() + ": " + err.Error())
				}
				data = decoded
			}
			fsys[f.Get("path").String()] = &fstest.MapFile{Data: data}
		}
		validator, loadDiags = compiler.NewValidator(fsys, compiler.Options{Limits: limits.Defaults()})
	}
	if validator == nil {
		if len(loadDiags) > 0 {
			return marshal(loadDiags)
		}
		return errorJSON("no project is loaded")
	}
	file := args[1].String()
	if file == "" {
		return marshal(loadDiags)
	}
	return marshal(validator.ValidatePage(file, []byte(args[2].String())))
}

func marshal(diags plxerr.Diagnostics) string {
	if diags == nil {
		diags = plxerr.Diagnostics{}
	}
	out, err := json.Marshal(diags)
	if err != nil {
		return errorJSON(err.Error())
	}
	return string(out)
}

func errorJSON(msg string) string {
	out, _ := json.Marshal(map[string]string{"error": msg})
	return string(out)
}

func main() {
	js.Global().Set("pluxValidatePage", js.FuncOf(pluxValidatePage))
	select {}
}
