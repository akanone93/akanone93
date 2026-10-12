<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

# Secure UI Guide

The widgets, pages and behaviours that keep secrets on screen from leaking: hardened inputs,
secure pages, masking, the inactivity lock, the biometric prompt and ID capture. These are
`SEC-090` to `SEC-094` and `SEC-092`; the design is [ADR-0059](../adr/0059-secure-ui.md).
A document that uses them requires the runtime feature `security.secureui.v1` (runtime
0.4.0), so older runtimes refuse the release.

## 1. Widgets

| Widget | Use it for | Notable props and events |
|---|---|---|
| `SecureTextField` | Passwords, answers, card data | `value` (two-way), `label`, `obscure` (default `true`), `maxLength`; `onChanged`, `onSubmitted` |
| `SecurePinPad` | PIN entry | `length` (4 to 12, default 4), `randomizeLayout`, `haptics`; `onCompleted` |
| `OtpInput` | One-time codes | `length` (4 to 12, default 6), `alphabet` (`numeric`, `alphanumeric`), `autofill`; `onCompleted` |
| `BiometricButton` | A step-up before an action | `label`, `reason`; `onSuccess`, `onFailure` |
| `KycCameraCapture` (package `plux_media`) | ID document and selfie capture | `mode` (`document`, `selfie`); `onCaptured` with a reference to the image |

What each does:

- **`SecureTextField`** is a text field with suggestions, autocorrect, keyboard learning,
  the clipboard and the selection menu turned off, obscured by default. It uses the
  platform's incognito keyboard flags (Android) and secure text entry (iOS).
- **`SecurePinPad`** is Plux's own keypad, not the system keyboard. `randomizeLayout`
  reshuffles the digits each time it appears, so smudges and shoulder surfing reveal less.
  There is no key-press animation; feedback is haptic only.
- **`OtpInput`** accepts manual entry and paste (checked against the length and alphabet), and
  autofills from the platform: the SMS User Consent API on Android (no SMS permission), the
  one-time-code content type on iOS. `onCompleted` fires when `length` characters are in.
- **`BiometricButton`** runs `biometricAuth` and fires `onSuccess` or `onFailure`.
- **`KycCameraCapture`** shows a framing guide, takes the photo, and measures it off the UI
  isolate. A photo that is `blurry`, `dark` or `bright` is explained to the user and cannot be
  used; they retake it. Register `PluxMedia()` in `devicePackages`, and the app must approve
  the `camera` capability.

The values these widgets emit (`onChanged`, `onSubmitted`, `onCompleted`) are **sensitive**.
A run they start records no values in its trace, and they never reach logs or telemetry.

```json
{"type": "SecurePinPad", "props": {"length": 6, "randomizeLayout": true},
 "events": {"onCompleted": {"steps": [
   {"id": "keep", "action": "setState",
    "input": {"path": "page.unlock.pin", "value": {"$expr": "event"}}}]}}}
```

## 2. Secure pages

Mark a page secure, and set the level it needs:

```json
"security": {"secure": true, "requiresAssurance": "AL2"}
```

While a secure page shows:

- **Android** sets `FLAG_SECURE`: no screenshots, no screen recording, a blank app switcher
  card; and the window drops touches while another window covers it (tapjacking, `SEC-093`).
- **iOS** cannot prevent a screenshot. The runtime covers the window while the screen is
  captured or mirrored and in the app switcher.
- The **inactivity lock** attaches to the page (§4).

Pages count each other: the protection stays on across secure pages and goes off after the last.
`screenshotBlockingDefault` makes every page behave as secure for screen protection (default on
under `maximum`); a page cannot opt out of it. If the platform cannot protect the window the
runtime reports it once and does not crash.

## 3. Masking

Mark a form field or a type field `sensitive` (`SCH-012`, `SEC-092`). The compiler then:

- keeps the value out of analytics and traces, and refuses `sensitive` with `persisted`;
- marks the text and inputs bound to it as **masked**.

A masked text shows bullets (`••••••••`, the same length for every value) until the user taps
the unmask button; a masked input is built obscured with suggestions, learning and the
selection menu off. Nothing of the value reaches the semantics tree while masked, and the value is
masked again when the app leaves the foreground. `Text`, `SelectableText`, `TextField`,
`TextFormField` and `CupertinoTextField` mask.

```json
{"name": "password", "type": "string", "sensitive": true}
```

## 4. The inactivity lock

With `inactivityLock` on (`strict`: 300 s, `maximum`: 120 s, `inactivityLockTimeout`), a secure
page covers its content with a lock screen after the idle time. The content stays mounted, so
what the user typed survives the unlock. Unlocking asks for a biometric or the device
credential.

- Pointer and key events and edits in the secure widgets count as activity.
- The lock runs only while a secure page shows. It also engages when the app returns to the
  foreground after the idle time passed; a quick return does not lock.
- A touch on the lock screen does not unlock it. One prompt is open at a time.
- A device with no screen lock stays locked and says what to set up.

## 5. `biometricAuth`

An action under the `biometrics` capability. It takes `reason` (shown by the system prompt)
and `allowDeviceCredential` (default `true`), and branches `success` or `failure`:

```json
{"id": "confirm", "action": "biometricAuth",
 "input": {"reason": "Confirm to view your statement"},
 "onSuccess": "open", "onFailure": "denied"}
```

A cancelled or failed prompt takes `failure`. A device that cannot ask (no hardware, nothing
enrolled, no screen lock) fails the step with `PLX-5403`; an unknown platform answer is never
treated as success. Biometric data never leaves the platform. For payments use the
[SCA](sca.md) flow, which signs, rather than `biometricAuth`, which only confirms.

## 6. Settings

| Setting | Effect |
|---|---|
| `screenshotBlockingDefault` | Every page protects the screen |
| `inactivityLock`, `inactivityLockTimeout` | The lock on secure pages |
| `raspScreenCaptureResponse`, `raspOverlayResponse` | What the runtime does when it sees mirroring, recording, or a covered touch |

Overrides only tighten them ([configuration](configuration.md)).

## 7. Accessibility

Secure widgets keep their roles and labels (`SecureTextField` and `OtpInput` are text fields,
`SecurePinPad` and `BiometricButton` buttons). Nothing of a masked value reaches screen readers. The confirmation sheet of [SCA](sca.md) reads each row as a unit.
