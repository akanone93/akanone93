// SPDX-FileCopyrightText: 2026 Apply Register
// SPDX-License-Identifier: Apache-2.0

//! `fees.quote`: Prices a registration from the environment's fee table, with 19 % tax rounded half-even.

use plux_fn::{BusinessError, ServerContext};
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

/// What to price.
#[derive(Debug, Deserialize, JsonSchema)]
pub struct Input {
    /// The organisation's slug.
    pub org: String,
    /// The fee tier within it.
    pub tier: String,
    /// How many.
    pub quantity: u32,
    /// ISO 4217 code; the table is in EUR.
    pub currency: String,
}

/// The price.
#[derive(Debug, Serialize, JsonSchema)]
pub struct Output {
    /// Before tax, decimal text.
    pub subtotal: String,
    /// The tax, decimal text.
    pub tax: String,
    /// subtotal + tax, decimal text.
    pub total: String,
    /// ISO 4217 code.
    pub currency: String,
}

/// 19 % of `c` hundredths, rounded half to even.
fn tax(c: i64) -> i64 {
    let (q, r) = (c * 19 / 100, c * 19 % 100);
    if r > 50 || (r == 50 && q % 2 == 1) { q + 1 } else { q }
}

fn handler(ctx: &ServerContext, i: Input) -> Result<Output, BusinessError> {
    if !(1..=50).contains(&i.quantity) {
        return Err(BusinessError::new("QUANTITY_INVALID", "fees.quantityInvalid").with_param("max", 50));
    }
    if i.currency != "EUR" {
        return Err(BusinessError::new("CURRENCY_UNSUPPORTED", "fees.currencyUnsupported"));
    }
    // No table, or one that does not read, is no price: never a guessed one.
    let raw = ctx
        .env("FEE_TABLE")
        .ok()
        .flatten()
        .ok_or_else(|| BusinessError::new("FEES_UNAVAILABLE", "fees.unavailable"))?;
    let table: serde_json::Value =
        serde_json::from_str(&raw).map_err(|_| BusinessError::new("FEES_UNAVAILABLE", "fees.unavailable"))?;
    let base = table[&i.org][&i.tier]
        .as_str()
        .and_then(cents)
        .ok_or_else(|| BusinessError::new("UNKNOWN_TIER", "fees.unknownTier").with_param("tier", &i.tier))?;
    let subtotal = base * i64::from(i.quantity);
    let t = tax(subtotal);
    Ok(Output { subtotal: money(subtotal), tax: money(t), total: money(subtotal + t), currency: i.currency })
}

plux_fn::function!(handler);

#[cfg(test)]
mod tests {
    use super::*;
    use plux_fn::testing::{TestServer, replay};

    #[test]
    fn prices_from_the_table_with_half_even_tax() {
        let host = TestServer::new();
        host.host().set_env("FEE_TABLE", r#"{"campus": {"bachelor": "45.00", "odd": "0.50"}}"#);
        let out = handler(&host.context(), Input { org: "campus".into(), tier: "bachelor".into(), quantity: 2, currency: "EUR".into() }).unwrap();
        assert_eq!((out.subtotal.as_str(), out.tax.as_str(), out.total.as_str()), ("90.00", "17.10", "107.10"));
        // 19 % of 0.50 is 9.5 hundredths: half-even rounds it to the even 10.
        let out = handler(&host.context(), Input { org: "campus".into(), tier: "odd".into(), quantity: 1, currency: "EUR".into() }).unwrap();
        assert_eq!(out.tax, "0.10");
    }

    #[test]
    fn an_unknown_tier_is_named() {
        let host = TestServer::new();
        host.host().set_env("FEE_TABLE", r#"{"campus": {}}"#);
        let e = handler(&host.context(), Input { org: "campus".into(), tier: "phd".into(), quantity: 1, currency: "EUR".into() }).unwrap_err();
        assert_eq!(e.code(), "UNKNOWN_TIER");
    }

    #[test]
    fn every_stored_case_replays() {
        let host = TestServer::new();
        let cases = replay(include_str!("../function.json"), |case, input| {
            if let Some(claims) = case.get("claims") {
                host.host().set_claims(claims.clone());
            }
            __plux_entry_with(host.static_host(), input)
        });
        assert!(cases >= 2);
    }
}
