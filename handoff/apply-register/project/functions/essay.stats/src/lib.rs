// SPDX-FileCopyrightText: 2026 Apply Register
// SPDX-License-Identifier: Apache-2.0

//! `essay.stats`: Counts an essay's words and sentences as the applicant writes.

use plux_fn::{BusinessError, DeviceContext};
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};

/// The essay.
#[derive(Debug, Deserialize, JsonSchema)]
pub struct Input {
    /// The text as written.
    pub text: String,
}

/// What the essay counts.
#[derive(Debug, Serialize, JsonSchema)]
#[serde(rename_all = "camelCase")]
pub struct Output {
    /// Words.
    pub words: u32,
    /// Sentences.
    pub sentences: u32,
    /// Minutes to read at 200 words a minute, rounded up.
    pub reading_minutes: u32,
    /// Words of seven letters or more.
    pub long_words: u32,
}

const MAX_WORDS: u32 = 5000;

fn handler(ctx: &DeviceContext, i: Input) -> Result<Output, BusinessError> {
    let mut words = 0u32;
    let mut long_words = 0u32;
    for w in i.text.split_whitespace() {
        words += 1;
        if w.chars().filter(|c| c.is_alphabetic()).count() >= 7 {
            long_words += 1;
        }
        if words > MAX_WORDS {
            return Err(BusinessError::new("TOO_LONG", "essay.tooLong").with_param("max", MAX_WORDS));
        }
    }
    // A sentence ends at a run of . ! ? (or their full-width forms) after some text.
    let mut sentences = 0u32;
    let mut open = false;
    for c in i.text.chars() {
        if matches!(c, '.' | '!' | '?' | '。' | '！' | '？' | '።') {
            if open {
                sentences += 1;
                open = false;
            }
        } else if c.is_alphanumeric() {
            open = true;
        }
    }
    if open {
        sentences += 1;
    }
    // The last count stays on the device, for the draft's summary.
    ctx.kv().put("lastWords", words.to_string().as_bytes()).unwrap_or_else(|e| e.trap());
    Ok(Output { words, sentences, reading_minutes: words.div_ceil(200), long_words })
}

plux_fn::function!(handler);

#[cfg(test)]
mod tests {
    use super::*;
    use plux_fn::testing::{TestDevice, replay};

    #[test]
    fn counts_and_remembers_the_words() {
        let host = TestDevice::new();
        let out = handler(&host.context(), Input { text: "Plux renders native widgets. It is fast!".into() }).unwrap();
        assert_eq!((out.words, out.sentences, out.reading_minutes, out.long_words), (7, 2, 1, 2));
        assert_eq!(host.host().kv("lastWords").as_deref(), Some(&b"7"[..]));
    }

    #[test]
    fn every_stored_case_replays() {
        let host = TestDevice::new();
        let cases = replay(include_str!("../function.json"), |case, input| {
            if let Some(claims) = case.get("claims") {
                host.host().set_claims(claims.clone());
            }
            __plux_entry_with(host.static_host(), input)
        });
        assert!(cases >= 2);
    }
}
