// SPDX-FileCopyrightText: 2026 Apply Register
// SPDX-License-Identifier: Apache-2.0

//! `pay.simulate`: Simulates a wallet charge for the Pay demo; never moves money.

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

/// The charge to simulate.
#[derive(Debug, Deserialize, JsonSchema)]
pub struct Input {
    /// Amount as decimal text with at most two places.
    pub amount: String,
    /// ISO 4217 code.
    pub currency: String,
    /// The caller's reference: letters, digits and hyphens.
    pub reference: String,
}

/// How the simulated charge ended.
#[derive(Debug, Serialize, JsonSchema)]
#[serde(rename_all = "camelCase")]
pub struct Output {
    /// `success` or `error`.
    pub status: String,
    /// Set on success.
    pub transaction_id: Option<String>,
    /// Set on error.
    pub error_code: Option<String>,
}

const FAILURES: [&str; 4] = ["INSUFFICIENT_FUNDS", "DECLINED", "LIMIT_EXCEEDED", "WALLET_UNAVAILABLE"];

fn handler(_ctx: &DeviceContext, i: Input) -> Result<Output, BusinessError> {
    let amount = cents(&i.amount)
        .filter(|c| *c > 0)
        .ok_or_else(|| BusinessError::new("AMOUNT_INVALID", "pay.amountInvalid"))?;
    if i.currency.len() != 3 || !i.currency.bytes().all(|b| b.is_ascii_uppercase()) {
        return Err(BusinessError::new("CURRENCY_INVALID", "pay.currencyInvalid"));
    }
    if i.reference.is_empty()
        || i.reference.len() > 64
        || !i.reference.bytes().all(|b| b.is_ascii_alphanumeric() || b == b'-')
    {
        return Err(BusinessError::new("REFERENCE_INVALID", "pay.referenceInvalid"));
    }
    // One charge in five fails. The outcome follows from the input, not the clock, so a
    // case ends the same way every time and on every engine.
    let h = i
        .reference
        .bytes()
        .fold(amount.unsigned_abs(), |a, b| a.wrapping_mul(31).wrapping_add(u64::from(b)));
    if h % 5 == 0 {
        let code = FAILURES[usize::try_from((h / 5) % 4).unwrap_or(0)];
        return Ok(Output { status: "error".into(), transaction_id: None, error_code: Some(code.into()) });
    }
    Ok(Output { status: "success".into(), transaction_id: Some(format!("SIM-{}", i.reference)), error_code: None })
}

plux_fn::function!(handler);

#[cfg(test)]
mod tests {
    use super::*;
    use plux_fn::testing::{TestDevice, replay};

    #[test]
    fn a_zero_amount_is_refused() {
        let host = TestDevice::new();
        let i = Input { amount: "0".into(), currency: "EUR".into(), reference: "R-1".into() };
        assert_eq!(handler(&host.context(), i).unwrap_err().code(), "AMOUNT_INVALID");
    }

    #[test]
    fn the_same_charge_always_ends_the_same_way() {
        let host = TestDevice::new();
        let run = || {
            let i = Input { amount: "25.00".into(), currency: "EUR".into(), reference: "CR-2027-0042".into() };
            handler(&host.context(), i).unwrap().status
        };
        assert_eq!(run(), run());
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
