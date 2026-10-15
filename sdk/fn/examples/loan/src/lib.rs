// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

//! `loan.calculateSchedule`: a device function that amortises a loan.
//!
//! Decimals travel as strings and are computed with `rust_decimal`, so the
//! result is exact and identical on every device. Payments are rounded
//! half-even to two places; the last instalment absorbs the rounding so the
//! balance ends at exactly zero.

use plux_fn::{BusinessError, DeviceContext};
use rust_decimal::{Decimal, MathematicalOps, RoundingStrategy};
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};

/// Smallest principal accepted.
const MIN_PRINCIPAL: Decimal = Decimal::from_parts(1000, 0, 0, false, 0);
/// Largest principal accepted; keeps every intermediate inside `Decimal`.
const MAX_PRINCIPAL: Decimal = Decimal::from_parts(0xd4a5_1000, 0xe8, 0, false, 0);
/// Longest term in months.
const MAX_MONTHS: u32 = 360;

/// Loan terms.
#[derive(Debug, Deserialize, JsonSchema)]
#[serde(rename_all = "camelCase")]
pub struct Input {
    /// Amount borrowed, a decimal string, at least 1000.
    #[serde(with = "rust_decimal::serde::str")]
    #[schemars(with = "String")]
    pub principal: Decimal,
    /// Yearly interest rate as a fraction from 0 to 1, a decimal string.
    #[serde(with = "rust_decimal::serde::str")]
    #[schemars(with = "String")]
    pub annual_rate: Decimal,
    /// Term in months, 1 to 360.
    #[schemars(range(min = 1, max = 360))]
    pub months: u32,
}

/// One instalment.
#[derive(Debug, Serialize, JsonSchema)]
#[serde(rename_all = "camelCase")]
pub struct Instalment {
    /// Month number from 1.
    pub month: u32,
    /// Amount paid this month.
    #[serde(with = "rust_decimal::serde::str")]
    #[schemars(with = "String")]
    pub payment: Decimal,
    /// Part of the payment that repays principal.
    #[serde(with = "rust_decimal::serde::str")]
    #[schemars(with = "String")]
    pub principal: Decimal,
    /// Part of the payment that is interest.
    #[serde(with = "rust_decimal::serde::str")]
    #[schemars(with = "String")]
    pub interest: Decimal,
    /// Principal still owed after this payment.
    #[serde(with = "rust_decimal::serde::str")]
    #[schemars(with = "String")]
    pub balance: Decimal,
}

/// The amortisation schedule.
#[derive(Debug, Serialize, JsonSchema)]
#[serde(rename_all = "camelCase")]
pub struct Output {
    /// The regular monthly payment.
    #[serde(with = "rust_decimal::serde::str")]
    #[schemars(with = "String")]
    pub monthly_payment: Decimal,
    /// One entry per month.
    pub schedule: Vec<Instalment>,
}

fn money(value: Decimal) -> Decimal {
    let mut rounded = value.round_dp_with_strategy(2, RoundingStrategy::MidpointNearestEven);
    rounded.rescale(2);
    rounded
}

fn range_error(code: &str, key: &str) -> BusinessError {
    BusinessError::new(code, key)
}

fn calculate_schedule(_ctx: &DeviceContext, input: Input) -> Result<Output, BusinessError> {
    if input.principal < MIN_PRINCIPAL {
        return Err(range_error("PRINCIPAL_TOO_SMALL", "loan.principalTooSmall")
            .with_param("min", MIN_PRINCIPAL.to_string()));
    }
    // The upper bound keeps every intermediate value inside rust_decimal's
    // range, so a large principal is refused rather than trapping.
    if input.principal > MAX_PRINCIPAL {
        return Err(range_error("PRINCIPAL_TOO_LARGE", "loan.principalTooLarge")
            .with_param("max", MAX_PRINCIPAL.to_string()));
    }
    if input.annual_rate < Decimal::ZERO || input.annual_rate > Decimal::ONE {
        return Err(range_error("RATE_OUT_OF_RANGE", "loan.rateOutOfRange")
            .with_param("min", "0")
            .with_param("max", "1"));
    }
    if input.months == 0 || input.months > MAX_MONTHS {
        return Err(range_error("MONTHS_OUT_OF_RANGE", "loan.monthsOutOfRange")
            .with_param("min", 1)
            .with_param("max", MAX_MONTHS));
    }

    let monthly_rate = input.annual_rate / Decimal::from(12);
    let months = Decimal::from(input.months);
    let payment = if monthly_rate.is_zero() {
        money(input.principal / months)
    } else {
        let growth = (Decimal::ONE + monthly_rate).powi(i64::from(input.months));
        money(input.principal * monthly_rate * growth / (growth - Decimal::ONE))
    };

    let mut balance = input.principal;
    let mut schedule = Vec::with_capacity(input.months as usize);
    for month in 1..=input.months {
        let interest = money(balance * monthly_rate);
        let (paid, principal) = if month == input.months {
            (balance + interest, balance)
        } else {
            (payment, payment - interest)
        };
        balance -= principal;
        schedule.push(Instalment {
            month,
            payment: money(paid),
            principal: money(principal),
            interest,
            balance: money(balance),
        });
    }
    Ok(Output {
        monthly_payment: payment,
        schedule,
    })
}

plux_fn::function!(calculate_schedule);

#[cfg(test)]
mod tests {
    use super::*;
    use plux_fn::Placement;
    use plux_fn::testing::{TestDevice, replay};
    use std::str::FromStr;

    fn terms(principal: &str, rate: &str, months: u32) -> Input {
        Input {
            principal: Decimal::from_str(principal).unwrap(),
            annual_rate: Decimal::from_str(rate).unwrap(),
            months,
        }
    }

    #[test]
    fn is_a_device_function() {
        assert_eq!(__PLUX_PLACEMENT, Placement::Device);
    }

    #[test]
    fn every_stored_case_replays() {
        let device = TestDevice::new();
        let cases = replay(include_str!("../function.json"), |_, input| {
            __plux_entry_with(device.static_host(), input)
        });
        assert!(cases >= 3);
    }

    #[test]
    fn the_balance_ends_at_exactly_zero_and_principal_adds_up() {
        let device = TestDevice::new();
        for (principal, rate, months) in [
            ("10000", "0.05", 12),
            ("250000.55", "0.0425", 360),
            ("1000", "1", 360),
            ("1234.56", "0.07", 7),
            ("1000000000000", "1", 360),
        ] {
            let out =
                calculate_schedule(&device.context(), terms(principal, rate, months)).unwrap();
            let last = out.schedule.last().unwrap();
            assert_eq!(last.balance, Decimal::ZERO, "{principal} {rate} {months}");
            assert_eq!(last.balance.scale(), 2);
            let repaid: Decimal = out.schedule.iter().map(|i| i.principal).sum();
            assert_eq!(repaid, Decimal::from_str(principal).unwrap());
            assert_eq!(out.schedule.len(), months as usize);
        }
    }

    #[test]
    fn a_zero_rate_splits_the_principal_evenly() {
        let device = TestDevice::new();
        let out = calculate_schedule(&device.context(), terms("1000", "0", 3)).unwrap();
        let payments: Vec<String> = out.schedule.iter().map(|i| i.payment.to_string()).collect();
        assert_eq!(payments, ["333.33", "333.33", "333.34"]);
        assert!(out.schedule.iter().all(|i| i.interest.is_zero()));
    }

    #[test]
    fn out_of_range_terms_are_business_errors() {
        let device = TestDevice::new();
        let ctx = device.context();
        let code = |input| {
            calculate_schedule(&ctx, input)
                .unwrap_err()
                .code()
                .to_owned()
        };
        assert_eq!(code(terms("999.99", "0.05", 12)), "PRINCIPAL_TOO_SMALL");
        assert_eq!(
            code(terms("1000000000001", "0.05", 12)),
            "PRINCIPAL_TOO_LARGE"
        );
        assert_eq!(code(terms("1000", "-0.01", 12)), "RATE_OUT_OF_RANGE");
        assert_eq!(code(terms("1000", "1.01", 12)), "RATE_OUT_OF_RANGE");
        assert_eq!(code(terms("1000", "0.05", 0)), "MONTHS_OUT_OF_RANGE");
        assert_eq!(code(terms("1000", "0.05", 361)), "MONTHS_OUT_OF_RANGE");
        assert!(calculate_schedule(&ctx, terms("1000", "1", 1)).is_ok());
        assert!(calculate_schedule(&ctx, terms("1000", "0", 360)).is_ok());
    }

    #[test]
    fn the_schemas_describe_the_camel_case_fields() {
        let schemas: serde_json::Value = serde_json::from_str(&__plux_schemas()).unwrap();
        assert_eq!(
            schemas["input"]["properties"]["annualRate"]["type"],
            "string"
        );
        assert_eq!(schemas["input"]["properties"]["months"]["maximum"], 360);
        assert!(schemas["output"]["properties"]["monthlyPayment"].is_object());
    }
}
