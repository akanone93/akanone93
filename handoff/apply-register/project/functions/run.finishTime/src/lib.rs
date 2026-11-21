// SPDX-FileCopyrightText: 2026 Apply Register
// SPDX-License-Identifier: Apache-2.0

//! `run.finishTime`: The finish time a pace gives over a distance.

use plux_fn::{BusinessError, DeviceContext};
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};

/// The distance and the pace.
#[derive(Debug, Deserialize, JsonSchema)]
#[serde(rename_all = "camelCase")]
pub struct Input {
    /// Kilometres as decimal text with at most three places.
    pub distance_km: String,
    /// Seconds per kilometre.
    pub pace_sec_per_km: u32,
}

/// The finish time.
#[derive(Debug, Serialize, JsonSchema)]
pub struct Output {
    /// Whole minutes.
    pub minutes: u32,
    /// hh:mm:ss.
    pub label: String,
}

/// Metres in a decimal text of kilometres with at most three places.
fn metres(text: &str) -> Option<u64> {
    let (whole, frac) = text.trim().split_once('.').unwrap_or((text.trim(), ""));
    let digits = |s: &str| s.bytes().all(|b| b.is_ascii_digit());
    if whole.is_empty() || whole.len() > 4 || frac.len() > 3 || !digits(whole) || !digits(frac) {
        return None;
    }
    let w: u64 = whole.parse().ok()?;
    let f: u64 = if frac.is_empty() { 0 } else { format!("{frac:0<3}").parse().ok()? };
    Some(w * 1000 + f)
}

fn handler(_ctx: &DeviceContext, i: Input) -> Result<Output, BusinessError> {
    if !(150..=1800).contains(&i.pace_sec_per_km) {
        return Err(BusinessError::new("PACE_UNREALISTIC", "run.paceUnrealistic").with_param("min", 150));
    }
    let m = metres(&i.distance_km)
        .filter(|m| (1..=1_000_000).contains(m))
        .ok_or_else(|| BusinessError::new("DISTANCE_INVALID", "run.distanceInvalid"))?;
    // Rounded to the nearest second.
    let s = (m * u64::from(i.pace_sec_per_km) + 500) / 1000;
    let label = format!("{:02}:{:02}:{:02}", s / 3600, s / 60 % 60, s % 60);
    Ok(Output { minutes: u32::try_from(s / 60).unwrap_or(u32::MAX), label })
}

plux_fn::function!(handler);

#[cfg(test)]
mod tests {
    use super::*;
    use plux_fn::testing::{TestDevice, replay};

    #[test]
    fn a_half_marathon_at_five_minutes_a_kilometre() {
        let host = TestDevice::new();
        let out = handler(&host.context(), Input { distance_km: "21.0975".into(), pace_sec_per_km: 300 });
        assert!(out.is_err(), "four places are refused");
        let out = handler(&host.context(), Input { distance_km: "21.097".into(), pace_sec_per_km: 300 }).unwrap();
        assert_eq!(out.label, "01:45:29");
        assert_eq!(out.minutes, 105);
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
