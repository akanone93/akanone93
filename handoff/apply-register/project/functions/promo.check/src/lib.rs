// SPDX-FileCopyrightText: 2026 Apply Register
// SPDX-License-Identifier: Apache-2.0

//! `promo.check`: Checks a DevHarbor promotion code: EARLY20 until sales close, TEAM5 for five tickets or more.

use plux_fn::{BusinessError, ServerContext};
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};

/// The code and the order it is for.
#[derive(Debug, Deserialize, JsonSchema)]
#[serde(rename_all = "camelCase")]
pub struct Input {
    /// As typed; case does not matter.
    pub code: String,
    /// Tickets in the order.
    pub quantity: u32,
}

/// The discount the code gives.
#[derive(Debug, Serialize, JsonSchema)]
#[serde(rename_all = "camelCase")]
pub struct Output {
    /// The code, upper case.
    pub code: String,
    /// Percent off.
    pub discount_percent: u32,
}

fn handler(ctx: &ServerContext, i: Input) -> Result<Output, BusinessError> {
    let code = i.code.trim().to_uppercase();
    match code.as_str() {
        "EARLY20" => {
            // When early sales close is the server's rule (Unix seconds in PROMO_EARLY_CLOSES), never the
            // caller's: without it the code has expired, so a missing setting never extends a discount.
            let closes_at = ctx
                .env("PROMO_EARLY_CLOSES")
                .ok()
                .flatten()
                .and_then(|v| v.trim().parse::<i64>().ok())
                .unwrap_or(i64::MIN);
            let now = ctx.now().unwrap_or_else(|e| e.trap()) / 1_000_000_000;
            if now >= closes_at {
                return Err(BusinessError::new("PROMO_EXPIRED", "promo.expired"));
            }
            Ok(Output { code, discount_percent: 20 })
        }
        "TEAM5" => {
            if i.quantity < 5 {
                return Err(BusinessError::new("PROMO_CONDITION", "promo.condition").with_param("min", 5));
            }
            Ok(Output { code, discount_percent: 15 })
        }
        _ => Err(BusinessError::new("PROMO_INVALID", "promo.invalid")),
    }
}

plux_fn::function!(handler);

#[cfg(test)]
mod tests {
    use super::*;
    use plux_fn::testing::{TestServer, replay};

    #[test]
    fn early_codes_expire_with_sales() {
        let host = TestServer::new();
        host.host().set_now(2_000_000_000 * 1_000_000_000);
        let early = || handler(&host.context(), Input { code: "early20".into(), quantity: 1 });
        assert_eq!(early().unwrap_err().code(), "PROMO_EXPIRED", "no closing time is expired");
        host.host().set_env("PROMO_EARLY_CLOSES", "2000000001");
        assert_eq!(early().unwrap().discount_percent, 20);
        host.host().set_env("PROMO_EARLY_CLOSES", "2000000000");
        assert_eq!(early().unwrap_err().code(), "PROMO_EXPIRED");
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
