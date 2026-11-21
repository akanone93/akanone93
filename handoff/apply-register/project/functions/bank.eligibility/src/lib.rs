// SPDX-FileCopyrightText: 2026 Apply Register
// SPDX-License-Identifier: Apache-2.0

//! `bank.eligibility`: A first look at whether someone may open a Nordvia account; the bank decides on the server.

use plux_fn::{BusinessError, DeviceContext};
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};
/// Money in hundredths: a non-negative decimal text with at most two decimal
/// places and twelve whole digits, so arithmetic stays far from overflow.
fn cents(text: &str) -> Option<i64> {
    let t = text.trim();
    let (whole, frac) = t.split_once('.').unwrap_or((t, ""));
    let digits = |s: &str| s.bytes().all(|b| b.is_ascii_digit());
    if whole.is_empty() || whole.len() > 12 || frac.len() > 2 || !digits(whole) || !digits(frac) {
        return None;
    }
    let w: i64 = whole.parse().ok()?;
    let f: i64 = if frac.is_empty() { 0 } else { format!("{frac:0<2}").parse().ok()? };
    w.checked_mul(100)?.checked_add(f)
}

/// Hundredths as decimal text with two places.
fn money(c: i64) -> String {
    format!("{}.{:02}", c / 100, c % 100)
}

/// What the applicant told us.
#[derive(Debug, Deserialize, JsonSchema)]
#[serde(rename_all = "camelCase")]
pub struct Input {
    /// Monthly income as decimal text.
    pub monthly_income: String,
    /// Age in years.
    pub age: u32,
    /// employed, self, student or retired.
    pub employment: String,
}

/// The first look.
#[derive(Debug, Serialize, JsonSchema)]
pub struct Output {
    /// Whether the score reaches 50.
    pub eligible: bool,
    /// 0 to 80.
    pub score: u32,
    /// Message keys that explain a low score.
    pub reasons: Vec<String>,
}

fn handler(_ctx: &DeviceContext, i: Input) -> Result<Output, BusinessError> {
    if i.age < 18 {
        return Err(BusinessError::new("UNDERAGE", "bank.underage").with_param("min", 18));
    }
    if i.age > 120 {
        return Err(BusinessError::new("AGE_INVALID", "bank.ageInvalid"));
    }
    if !["employed", "self", "student", "retired"].contains(&i.employment.as_str()) {
        return Err(BusinessError::new("EMPLOYMENT_INVALID", "bank.employmentInvalid"));
    }
    let income = cents(&i.monthly_income).ok_or_else(|| BusinessError::new("INCOME_INVALID", "bank.incomeInvalid"))?;
    // One point for every 100 of income, at most 40.
    let points = u32::try_from((income / 10_000).min(40)).unwrap_or(40);
    let mut score = 40 + points;
    let mut reasons = vec![];
    if i.employment == "student" {
        score -= 10;
        reasons.push("bank.reason.student".into());
    }
    if points < 10 {
        reasons.push("bank.reason.income".into());
    }
    Ok(Output { eligible: score >= 50, score, reasons })
}

plux_fn::function!(handler);

#[cfg(test)]
mod tests {
    use super::*;
    use plux_fn::testing::{TestDevice, replay};

    #[test]
    fn a_student_scores_lower_and_is_told_why() {
        let host = TestDevice::new();
        let i = Input { monthly_income: "1500.00".into(), age: 20, employment: "student".into() };
        let out = handler(&host.context(), i).unwrap();
        assert_eq!(out.score, 45);
        assert!(!out.eligible);
        assert_eq!(out.reasons, vec!["bank.reason.student".to_string()]);
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
