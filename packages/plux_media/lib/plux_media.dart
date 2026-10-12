// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// Media actions for Plux plugins (RT-060, SEC-080): `pickImage`,
/// `capturePhoto` and `pickFile`, and the camera and quality checks behind
/// the `KycCameraCapture` widget (SEC-090).
///
/// Add [PluxMedia] to `PluxConfig.devicePackages`. A plugin that uses the
/// actions declares the `photos`, `camera` or `files` device API in its
/// capabilities and the app document approves it; without this package the
/// actions fail with `PLX-5401`. The host declares the platform usage
/// descriptions (`NSPhotoLibraryUsageDescription`,
/// `NSCameraUsageDescription`) and the camera permission itself.
library;

export 'src/kyc.dart' show KycQuality, KycThresholds;
export 'src/media.dart' show FileBackend, ImageBackend, PickedEntry, PluxMedia;
