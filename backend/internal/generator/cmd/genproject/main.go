// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

// Command genproject writes the Flutter host project the generator makes for
// a fixed app, without a server, for the release inspection (SEC-074,
// DEV-050; build log row 273): the same templates `plux create` and Studio's
// project download render, depending on this repository's plux_flutter and
// plux_devtools by path, with a development key so that its debug builds
// carry the dev session the release build must not.
//
//	go run ./internal/generator/cmd/genproject -out DIR -plux-path PATH [-router none|go_router|auto_route]
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/nightCode42/plux3/backend/internal/codegen"
	"github.com/nightCode42/plux3/backend/internal/generator"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "genproject:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	set := flag.NewFlagSet("genproject", flag.ContinueOnError)
	out := set.String("out", "", "the new or empty `directory` to write the project into")
	pluxPath := set.String("plux-path", "", "the plux_flutter `directory` the project depends on")
	router := set.String("router", "none", "the `router` of the shell: none, go_router or auto_route")
	if err := set.Parse(args); err != nil {
		return err //nolint:wrapcheck // the flag package's own message
	}
	if *out == "" || *pluxPath == "" {
		return errors.New("give -out and -plux-path")
	}
	if err := generator.CheckRouter(*router); err != nil {
		return err //nolint:wrapcheck // a usage error
	}
	files, err := generator.Generate(input(*pluxPath, *router))
	if err != nil {
		return err //nolint:wrapcheck // the generator's own message
	}
	return generator.WriteDir(*out, files) //nolint:wrapcheck // the generator's own message
}

// input is the fixed app the inspection builds: placeholder keys, as the
// build only has to compile; the release is never run.
func input(pluxPath, router string) generator.Input {
	key := func(id string, b byte) codegen.RootKey {
		pub := make([]byte, 32)
		for i := range pub {
			pub[i] = b
		}
		return codegen.RootKey{KeyID: id, Algorithm: "ed25519", Role: "targets", PublicKey: pub}
	}
	if router == "none" {
		router = ""
	}
	return generator.Input{
		Spec: generator.ShellSpec{
			Name: "Release Inspection", Package: "release_inspection", ApplicationID: "com.example.release_inspection",
			BundleID: "com.example.releaseInspection", AppVersion: "1.0.0+1", SplashColor: "#FFFFFF", Router: router,
			Plux: generator.PluxSource{
				AppKey: "release-inspection", AppID: "01e0c450-6c00-7000-8000-0000000002a3", Endpoint: "https://plux.example.com",
				Environment: "production", Channel: "production", EntryRoute: "home",
			},
		},
		Keys:     []codegen.RootKey{key("production-1", 7)},
		DevKeys:  []codegen.RootKey{key("development-1", 9)},
		PluxPath: pluxPath,
	}
}
