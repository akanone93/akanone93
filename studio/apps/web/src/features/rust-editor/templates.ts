// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

export interface StarterTemplate {
  /** The function name the template suggests, in the dotted form of FN-006. */
  id: "loan.schedule" | "fee.calculator" | "eligibility.check" | "currency.convert";
  title: string;
  /** `src/lib.rs` of the new function crate. */
  source: string;
}

/** Money is integer minor units, as in the SDK's examples, so no template needs a decimal crate. */
const prelude = `use plux_fn::{BusinessError, ServerContext};
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};
`;

/** The four starters of FN-033: a loan schedule, a fee calculator, an eligibility check and a currency conversion. */
export const starterTemplates: readonly StarterTemplate[] = [
  {
    id: "loan.schedule",
    title: "Loan schedule",
    source: `${prelude}
/// A fixed-rate loan, in minor units.
#[derive(Debug, Deserialize, JsonSchema)]
pub struct Input {
    pub principal: i64,
    /// Yearly rate in basis points.
    pub rate_bp: i64,
    pub months: u32,
}

#[derive(Debug, Serialize, JsonSchema)]
pub struct Output {
    /// Interest of each month, in minor units.
    pub interest: Vec<i64>,
}

fn schedule(_ctx: &ServerContext, input: Input) -> Result<Output, BusinessError> {
    let monthly = input.principal.saturating_mul(input.rate_bp) / (12 * 10_000);
    Ok(Output { interest: vec![monthly; input.months as usize] })
}

plux_fn::function!(schedule);
`,
  },
  {
    id: "fee.calculator",
    title: "Fee calculator",
    source: `${prelude}
#[derive(Debug, Deserialize, JsonSchema)]
pub struct Input {
    /// Amount in minor units.
    pub amount: i64,
}

#[derive(Debug, Serialize, JsonSchema)]
pub struct Output {
    pub fee: i64,
}

/// One part in a thousand, at least 50 and at most 2 500 minor units.
fn fee(_ctx: &ServerContext, input: Input) -> Result<Output, BusinessError> {
    Ok(Output { fee: (input.amount / 1000).clamp(50, 2500) })
}

plux_fn::function!(fee);
`,
  },
  {
    id: "eligibility.check",
    title: "Eligibility check",
    source: `${prelude}
#[derive(Debug, Deserialize, JsonSchema)]
pub struct Input {
    pub age: u32,
    /// Yearly income in minor units.
    pub income: i64,
}

#[derive(Debug, Serialize, JsonSchema)]
pub struct Output {
    pub eligible: bool,
}

fn check(_ctx: &ServerContext, input: Input) -> Result<Output, BusinessError> {
    Ok(Output { eligible: input.age >= 18 && input.income >= 2_000_000 })
}

plux_fn::function!(check);
`,
  },
  {
    id: "currency.convert",
    title: "Currency conversion",
    source: `${prelude}
#[derive(Debug, Deserialize, JsonSchema)]
pub struct Input {
    /// Amount in minor units of the source currency.
    pub amount: i64,
    /// Rate in millionths of the target currency per unit of the source.
    pub rate_micro: i64,
}

#[derive(Debug, Serialize, JsonSchema)]
pub struct Output {
    pub converted: i64,
}

fn convert(_ctx: &ServerContext, input: Input) -> Result<Output, BusinessError> {
    Ok(Output { converted: input.amount.saturating_mul(input.rate_micro) / 1_000_000 })
}

plux_fn::function!(convert);
`,
  },
];
