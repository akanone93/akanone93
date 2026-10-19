// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// Video and audio widgets for Plux pages: `VideoPlayer` and `AudioPlayer`
/// on `video_player` (ADR-0071). Register [PluxPlayer] in
/// `PluxConfig.widgetPackages`.
library;

export 'src/package.dart' show PluxPlayer, PluxPlayerIds;
export 'src/players.dart'
    show
        PluxAudioPlayer,
        PluxPlayerControllerFactory,
        PluxVideoPlayer,
        formatPosition,
        playableUrl;
