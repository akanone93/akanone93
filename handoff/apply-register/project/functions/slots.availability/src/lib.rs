// SPDX-FileCopyrightText: 2026 Apply Register
// SPDX-License-Identifier: Apache-2.0

//! `slots.availability`: The clinic's free half-hour slots on a day, 09:00 to 16:30; closed on Sundays.

use plux_fn::{BusinessError, ServerContext};
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};

/// A day as (year, month, day), from yyyy-mm-dd; checks the calendar.
fn parse_day(text: &str) -> Option<(i32, u32, u32)> {
    let mut parts = text.split('-');
    let (y, m, d) = (parts.next()?, parts.next()?, parts.next()?);
    if parts.next().is_some() || y.len() != 4 || m.len() != 2 || d.len() != 2 {
        return None;
    }
    let (y, m, d): (i32, u32, u32) = (y.parse().ok()?, m.parse().ok()?, d.parse().ok()?);
    let leap = (y % 4 == 0 && y % 100 != 0) || y % 400 == 0;
    let days = [31, if leap { 29 } else { 28 }, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31];
    if (2000..=2100).contains(&y) && (1..=12).contains(&m) && d >= 1 && d <= days[(m - 1) as usize] {
        Some((y, m, d))
    } else {
        None
    }
}

/// 0 for Sunday to 6 for Saturday (Sakamoto).
fn weekday((y, m, d): (i32, u32, u32)) -> u32 {
    const T: [i32; 12] = [0, 3, 2, 5, 0, 3, 5, 1, 4, 6, 2, 4];
    let y = if m < 3 { y - 1 } else { y };
    let w = (y + y / 4 - y / 100 + y / 400 + T[(m - 1) as usize] + i32::try_from(d).unwrap_or(0)) % 7;
    u32::try_from(w).unwrap_or(0)
}

/// The day's slots, 09:00 to 16:30.
fn times() -> Vec<String> {
    (0..16).map(|k| format!("{:02}:{:02}", 9 + k / 2, (k % 2) * 30)).collect()
}

/// Days since 1970-01-01 of a calendar day (days_from_civil).
fn days_from_civil((y, m, d): (i32, u32, u32)) -> i64 {
    let y = i64::from(if m <= 2 { y - 1 } else { y });
    let era = y.div_euclid(400);
    let yoe = y - era * 400;
    let mp = i64::from(if m > 2 { m - 3 } else { m + 9 });
    let doy = (153 * mp + 2) / 5 + i64::from(d) - 1;
    era * 146_097 + yoe * 365 + yoe / 4 - yoe / 100 + doy - 719_468
}

/// How many days ahead a slot can be seen or booked.
const WINDOW_DAYS: i64 = 30;

/// Refuses a day before the server's today (UTC) or beyond the booking window, so no one books the past or
/// fills the calendar years ahead.
fn check_window(ctx: &ServerContext, day: (i32, u32, u32)) -> Result<(), BusinessError> {
    let today = ctx.now().unwrap_or_else(|e| e.trap()).div_euclid(86_400_000_000_000);
    let d = days_from_civil(day);
    if d < today {
        return Err(BusinessError::new("DATE_PAST", "slots.datePast"));
    }
    if d > today + WINDOW_DAYS {
        return Err(BusinessError::new("DATE_TOO_FAR", "slots.dateTooFar").with_param("days", WINDOW_DAYS));
    }
    Ok(())
}

/// Europe/Berlin's UTC offset on a day of clinic hours (09:00-17:00, which no change of clock crosses): summer
/// time from the last Sunday of March to the last Sunday of October.
fn berlin_offset((y, m, d): (i32, u32, u32)) -> &'static str {
    let last_sunday = |month: u32| {
        let last = (y, month, 31);
        days_from_civil(last) - i64::from(weekday(last))
    };
    let day = days_from_civil((y, m, d));
    if day >= last_sunday(3) && day < last_sunday(10) { "+02:00" } else { "+01:00" }
}

fn booked_key(clinic: &str, date: &str) -> String {
    format!("booked:{clinic}:{date}")
}

fn check_clinic(clinic: &str) -> Result<(), BusinessError> {
    if (2..=40).contains(&clinic.len()) && clinic.bytes().all(|b| b.is_ascii_lowercase() || b.is_ascii_digit() || b == b'-') {
        Ok(())
    } else {
        Err(BusinessError::new("CLINIC_INVALID", "slots.clinicInvalid"))
    }
}

/// The clinic and the day.
#[derive(Debug, Deserialize, JsonSchema)]
pub struct Input {
    /// The clinic's slug.
    pub clinic: String,
    /// yyyy-mm-dd.
    pub date: String,
}

/// One slot.
#[derive(Debug, Serialize, JsonSchema)]
#[serde(rename_all = "camelCase")]
pub struct Slot {
    /// hh:mm.
    pub time: String,
    /// Whether it can be booked.
    pub free: bool,
    /// When it starts, RFC 3339 with the clinic's offset, for a calendar.
    pub starts_at: String,
}

/// The day's slots.
#[derive(Debug, Serialize, JsonSchema)]
pub struct Output {
    /// In order.
    pub slots: Vec<Slot>,
}

fn handler(ctx: &ServerContext, i: Input) -> Result<Output, BusinessError> {
    let day = parse_day(&i.date).ok_or_else(|| BusinessError::new("DATE_INVALID", "slots.dateInvalid"))?;
    check_clinic(&i.clinic)?;
    if weekday(day) == 0 {
        return Err(BusinessError::new("CLOSED", "slots.closed"));
    }
    check_window(ctx, day)?;
    let booked: Vec<String> = ctx.kv().get_json(&booked_key(&i.clinic, &i.date)).unwrap_or_else(|e| e.trap()).unwrap_or_default();
    let offset = berlin_offset(day);
    let slots = times()
        .into_iter()
        .map(|t| Slot { free: !booked.contains(&t), starts_at: format!("{}T{t}:00{offset}", i.date), time: t })
        .collect();
    Ok(Output { slots })
}

plux_fn::function!(handler);

#[cfg(test)]
mod tests {
    use super::*;
    use plux_fn::testing::{TestServer, replay};

    #[test]
    fn a_booked_slot_is_not_free_and_sunday_is_closed() {
        let host = TestServer::new();
        host.host().set_now(1_763_337_600 * 1_000_000_000); // 2025-11-17, a Monday
        host.host().set_kv("booked:willow:2025-11-17", br#"["09:30"]"#);
        let day = |date: &str| handler(&host.context(), Input { clinic: "willow".into(), date: date.into() });
        let out = day("2025-11-17").unwrap();
        assert_eq!(out.slots.len(), 16);
        assert!(!out.slots[1].free && out.slots[0].free);
        assert_eq!(out.slots[0].starts_at, "2025-11-17T09:00:00+01:00");
        assert_eq!(day("2025-11-23").unwrap_err().code(), "CLOSED");
        assert_eq!(day("2025-11-14").unwrap_err().code(), "DATE_PAST");
        assert_eq!(day("2025-12-18").unwrap_err().code(), "DATE_TOO_FAR");
        assert_eq!(day("2025-12-17").unwrap().slots.len(), 16, "the window's last day");
    }

    #[test]
    fn summer_slots_carry_summer_time() {
        let host = TestServer::new();
        host.host().set_now(1_782_864_000 * 1_000_000_000); // 2026-07-01
        let out = handler(&host.context(), Input { clinic: "willow".into(), date: "2026-07-06".into() }).unwrap();
        assert_eq!(out.slots[0].starts_at, "2026-07-06T09:00:00+02:00");
        assert_eq!(berlin_offset((2026, 3, 27)), "+01:00");
        assert_eq!(berlin_offset((2026, 3, 30)), "+02:00");
        assert_eq!(berlin_offset((2026, 10, 23)), "+02:00");
        assert_eq!(berlin_offset((2026, 10, 26)), "+01:00");
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
