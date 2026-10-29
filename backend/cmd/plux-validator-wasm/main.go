// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

//go:build js && wasm

// Command plux-validator-wasm is a measurement prototype (Studio S0.8): it
// compiles the page validator to WebAssembly so a browser worker can call
// it. It exposes pluxValidatePage and, for the PXL language service of the
// editors (PXL-007, ADR-0099), pluxPxl and pluxPxlSites.
package main

import (
	"encoding/base64"
	"encoding/json"
	"syscall/js"
	"testing/fstest"

	"github.com/nightCode42/plux3/backend/internal/compiler"
	"github.com/nightCode42/plux3/backend/internal/importer"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/pxl/langsvc"
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

// language answers the PXL language-service requests of this instance.
var language = langsvc.NewHost()

// pluxPxl(op, scope, src, offset) answers one language-service request as
// JSON; see langsvc.Host.Handle.
func pluxPxl(_ js.Value, args []js.Value) any {
	if len(args) != 4 {
		return errorJSON("pluxPxl takes (op, scope, src, offset)")
	}
	return language.Handle(args[0].String(), args[1].String(), args[2].String(), args[3].Int())
}

// pluxPxlSites(file, data) returns the expression sites of the page, by the
// pointer of their binding, with the distinct environments they read, as
// {"sites": […], "envs": […]}. The project must be loaded.
func pluxPxlSites(_ js.Value, args []js.Value) any {
	if len(args) != 2 {
		return errorJSON("pluxPxlSites takes (file, data)")
	}
	if validator == nil {
		return errorJSON("no project is loaded")
	}
	sites, envs := validator.PXLSites(args[0].String(), []byte(args[1].String()))
	out, err := json.Marshal(map[string]any{"sites": orEmpty(sites), "envs": orEmpty(envs)})
	if err != nil {
		return errorJSON(err.Error())
	}
	return string(out)
}

// pluxImport(kind, files) runs an OpenAPI or GraphQL import on text the user
// provided (L-12); files is an array of {file, data}. It answers
// {"fragment": …, "diagnostics": […]}.
func pluxImport(_ js.Value, args []js.Value) any {
	if len(args) != 2 {
		return errorJSON("pluxImport takes (kind, files)")
	}
	files := args[1]
	var sources []importer.Source
	for i := range files.Length() {
		f := files.Index(i)
		sources = append(sources, importer.Source{File: f.Get("file").String(), Data: []byte(f.Get("data").String())})
	}
	out, err := importer.ImportJSON(args[0].String(), sources)
	if err != nil {
		return errorJSON(err.Error())
	}
	return string(out)
}

// pluxCollectionsPreview(previous, current, dropped) is the DB schema
// designer's migration preview: the publish-time collection comparison
// (DB-005) on JSON arrays of collections, answered as diagnostics.
func pluxCollectionsPreview(_ js.Value, args []js.Value) any {
	if len(args) != 3 {
		return errorJSON("pluxCollectionsPreview takes (previous, current, dropped)")
	}
	out, err := compiler.PreviewCollections([]byte(args[0].String()), []byte(args[1].String()), []byte(args[2].String()))
	if err != nil {
		return errorJSON(err.Error())
	}
	return string(out)
}

func orEmpty[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
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
	js.Global().Set("pluxPxl", js.FuncOf(pluxPxl))
	js.Global().Set("pluxPxlSites", js.FuncOf(pluxPxlSites))
	js.Global().Set("pluxImport", js.FuncOf(pluxImport))
	js.Global().Set("pluxCollectionsPreview", js.FuncOf(pluxCollectionsPreview))
	select {}
}
