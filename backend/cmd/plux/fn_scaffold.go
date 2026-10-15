// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
)

// crateNameOf turns a dotted function name into a Cargo package name:
// bank.transferFee becomes bank_transfer_fee.
func crateNameOf(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r == '.':
			b.WriteByte('_')
		case unicode.IsUpper(r):
			b.WriteByte('_')
			b.WriteRune(unicode.ToLower(r))
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// scaffold returns the files of a new function, by path inside its
// directory (FN-001, FN-002, FN-030). The handler is typed, so the input and
// output schemas are derived at build time (FN-004), and the function.json
// holds one stored test case that both `cargo test` and `plux fn test` run.
func scaffold(name, placement, id, sdk string) (map[string][]byte, error) {
	doc := map[string]any{
		"schemaVersion": "1.0.0", "kind": "function", "id": id, "name": name, "placement": placement,
		"capabilities": map[string]any{},
		"tests": []any{map[string]any{
			"name": "greets by name", "input": map[string]any{"name": "Ada"}, "output": map[string]any{"greeting": "Hello, Ada!"},
		}},
	}
	document, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode function.json: %w", err)
	}
	context, harness := "ServerContext", "TestServer"
	if placement == "device" {
		context, harness = "DeviceContext", "TestDevice"
	}
	source := strings.NewReplacer("{{context}}", context, "{{harness}}", harness, "{{name}}", name, "{{placement}}", strings.ToUpper(placement[:1])+placement[1:]).Replace(librs)
	return map[string][]byte{
		"Cargo.toml":         []byte(strings.Replace(cargoToml, "{{crate}}", crateNameOf(name), 1)),
		"src/lib.rs":         []byte(source),
		"function.json":      append(document, '\n'),
		".cargo/config.toml": []byte(fmt.Sprintf(cargoConfigToml, sdk)),
		".gitignore":         []byte("/target\n/vendor\n/.cargo\n"),
	}, nil
}

const cargoToml = `[package]
name = "{{crate}}"
version = "0.1.0"
edition = "2024"
publish = false

[dependencies]
plux-fn = "0.1"
serde = { version = "1", features = ["derive"] }
serde_json = "1"
schemars = "1.2"
`

// cargoConfigToml resolves the unpublished plux-fn to the pinned SDK on this
// machine; the server resolves it to its own pinned copy (ADR-0063), so the
// file stays out of the source archive and out of version control.
const cargoConfigToml = `# Written by plux fn new. It points plux-fn at the SDK on this machine.
[patch.crates-io]
plux-fn = { path = %q }
`

//nolint:misspell // Rust's serde derive is spelled Serialize.
const librs = `//! {{name}}: a {{placement}} Plux Function.

use plux_fn::{BusinessError, {{context}}};
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};

/// The input; its JSON Schema is derived from this type at build time.
#[derive(Debug, Deserialize, JsonSchema)]
pub struct Input {
    /// Who to greet.
    pub name: String,
}

/// The output; its JSON Schema is derived from this type at build time.
#[derive(Debug, Serialize, JsonSchema)]
pub struct Output {
    /// The greeting.
    pub greeting: String,
}

fn handler(_ctx: &{{context}}, input: Input) -> Result<Output, BusinessError> {
    if input.name.trim().is_empty() {
        return Err(BusinessError::new("NAME_EMPTY", "{{name}}.nameEmpty"));
    }
    Ok(Output {
        greeting: format!("Hello, {}!", input.name),
    })
}

plux_fn::function!(handler);

#[cfg(test)]
mod tests {
    use super::*;
    use plux_fn::testing::{{{harness}}, replay};

    #[test]
    fn rejects_an_empty_name() {
        let host = {{harness}}::new();
        let error = handler(&host.context(), Input { name: " ".into() }).unwrap_err();
        assert_eq!(error.code(), "NAME_EMPTY");
    }

    #[test]
    fn every_stored_case_replays() {
        let host = {{harness}}::new();
        let cases = replay(include_str!("../function.json"), |_, input| {
            __plux_entry_with(host.static_host(), input)
        });
        assert!(cases >= 1);
    }
}
`
