// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

//! `bank.transferFee`: a server function that prices a transfer from the
//! caller's verified `tier` claim.
//!
//! Money is integer minor units (hundredths), so the function needs no
//! decimal crate: standard tier pays 0.1 % of the amount with a floor of
//! 0.50 and a cap of 25.00; premium pays nothing.

use plux_fn::{BusinessError, HostError, ServerContext};
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};

/// Smallest standard fee, in minor units.
const MIN_FEE: i64 = 50;
/// Largest fee, in minor units.
const MAX_FEE: i64 = 2500;
/// The fee rate is one part in this many.
const RATE_DIVISOR: i64 = 1000;
/// Most digits before the decimal point; keeps arithmetic far from overflow.
const MAX_INTEGER_DIGITS: usize = 12;

/// The transfer to price.
#[derive(Debug, Deserialize, JsonSchema)]
pub struct Input {
    /// Amount as a decimal string with at most two decimal places.
    pub amount: String,
}

/// The fee.
#[derive(Debug, Serialize, JsonSchema)]
pub struct Output {
    /// Fee as a decimal string with two decimal places.
    pub fee: String,
}

/// Parses `digits[.d[d]]` into minor units; anything else is `None`.
fn parse_minor_units(text: &str) -> Option<i64> {
    let (whole, fraction) = text.split_once('.').unwrap_or((text, ""));
    let digits = |s: &str| s.bytes().all(|b| b.is_ascii_digit());
    if whole.is_empty()
        || whole.len() > MAX_INTEGER_DIGITS
        || fraction.len() > 2
        || !digits(whole)
        || !digits(fraction)
        || (text.contains('.') && fraction.is_empty())
    {
        return None;
    }
    let cents = format!("{fraction:0<2}").parse::<i64>().ok()?;
    whole
        .parse::<i64>()
        .ok()?
        .checked_mul(100)?
        .checked_add(cents)
}

fn format_minor_units(value: i64) -> String {
    format!("{}.{:02}", value / 100, value % 100)
}

/// `numerator / denominator` rounded half to even.
fn div_half_even(numerator: i64, denominator: i64) -> i64 {
    let quotient = numerator / denominator;
    let twice_remainder = 2 * (numerator % denominator);
    if twice_remainder > denominator || (twice_remainder == denominator && quotient % 2 == 1) {
        quotient + 1
    } else {
        quotient
    }
}

fn transfer_fee(ctx: &ServerContext, input: Input) -> Result<Output, BusinessError> {
    let amount = parse_minor_units(&input.amount)
        .filter(|minor| *minor > 0)
        .ok_or_else(|| BusinessError::new("AMOUNT_INVALID", "bank.amountInvalid"))?;

    let claims = match ctx.claims() {
        Ok(claims) => claims,
        Err(HostError::NotFound) => return Err(tier_unknown()),
        Err(other) => other.trap(),
    };
    let fee = match claims.get_str("tier") {
        Some("standard") => div_half_even(amount, RATE_DIVISOR).clamp(MIN_FEE, MAX_FEE),
        Some("premium") => 0,
        _ => return Err(tier_unknown()),
    };
    Ok(Output {
        fee: format_minor_units(fee),
    })
}

fn tier_unknown() -> BusinessError {
    BusinessError::new("TIER_UNKNOWN", "bank.tierUnknown")
}

plux_fn::function!(transfer_fee);

#[cfg(test)]
mod tests {
    use super::*;
    use plux_fn::Placement;
    use plux_fn::testing::{TestServer, replay};
    use serde_json::json;

    fn fee(tier: Option<serde_json::Value>, amount: &str) -> Result<String, String> {
        let server = TestServer::new();
        if let Some(tier) = tier {
            server.set_claims(json!({ "tier": tier }));
        }
        transfer_fee(
            &server.context(),
            Input {
                amount: amount.to_owned(),
            },
        )
        .map(|out| out.fee)
        .map_err(|e| e.code().to_owned())
    }

    fn standard(amount: &str) -> Result<String, String> {
        fee(Some(json!("standard")), amount)
    }

    #[test]
    fn is_a_server_function() {
        assert_eq!(__PLUX_PLACEMENT, Placement::Server);
    }

    #[test]
    fn every_stored_case_replays() {
        let cases = replay(include_str!("../function.json"), |case, input| {
            let server = TestServer::new();
            if let Some(claims) = case.get("claims") {
                server.set_claims(claims.clone());
            }
            __plux_entry_with(server.static_host(), input)
        });
        assert!(cases >= 6);
    }

    #[test]
    fn standard_fee_is_a_tenth_of_a_percent_between_floor_and_cap() {
        assert_eq!(standard("10").unwrap(), "0.50");
        assert_eq!(standard("500").unwrap(), "0.50");
        assert_eq!(standard("1000").unwrap(), "1.00");
        assert_eq!(standard("1234.56").unwrap(), "1.23");
        assert_eq!(standard("25000").unwrap(), "25.00");
        assert_eq!(standard("999999999999.99").unwrap(), "25.00");
    }

    #[test]
    fn the_fee_rounds_half_to_even() {
        assert_eq!(standard("1000.50").unwrap(), "1.00");
        assert_eq!(standard("1001.50").unwrap(), "1.00");
        assert_eq!(standard("1002.50").unwrap(), "1.00");
        assert_eq!(standard("1500.50").unwrap(), "1.50");
        assert_eq!(div_half_even(1500, 1000), 2);
        assert_eq!(div_half_even(2500, 1000), 2);
        assert_eq!(div_half_even(3500, 1000), 4);
    }

    #[test]
    fn premium_pays_nothing() {
        assert_eq!(fee(Some(json!("premium")), "50000").unwrap(), "0.00");
    }

    #[test]
    fn invalid_amounts_are_refused_before_claims_are_read() {
        for bad in [
            "0",
            "0.00",
            "-5",
            "",
            "abc",
            "1.",
            ".5",
            "1.234",
            "1,5",
            "1e3",
            " 1",
            "9999999999999",
        ] {
            assert_eq!(fee(None, bad), Err("AMOUNT_INVALID".to_owned()), "{bad:?}");
        }
    }

    #[test]
    fn a_missing_or_unknown_tier_is_tier_unknown() {
        assert_eq!(fee(None, "100"), Err("TIER_UNKNOWN".to_owned()));
        assert_eq!(
            fee(Some(json!("gold")), "100"),
            Err("TIER_UNKNOWN".to_owned())
        );
        assert_eq!(fee(Some(json!(7)), "100"), Err("TIER_UNKNOWN".to_owned()));
    }

    #[test]
    #[should_panic(expected = "capability denied")]
    fn a_denied_claims_capability_is_a_system_error() {
        let server = plux_fn::testing::MockHost::new(Placement::Device);
        let ctx = ServerContext::new(&server).unwrap();
        let _ = transfer_fee(
            &ctx,
            Input {
                amount: "100".to_owned(),
            },
        );
    }
}
