// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// `VideoPlayer` and `AudioPlayer` (ADR-0071) on `video_player`, which plays
/// audio-only sources through the same platform players (ExoPlayer, AVPlayer).
/// Controls are plain Material buttons with labels, so screen readers and
/// switch access reach them. A player never starts by itself unless
/// `autoplay` is set, and not even then under reduce motion.
library;

import 'dart:async';
import 'dart:io';

import 'package:flutter/material.dart';
import 'package:video_player/video_player.dart' as vp;

/// Makes the controller that plays [url]; [captions] is the parsed captions
/// track, if any. The default plays over the network.
typedef PluxPlayerControllerFactory = vp.VideoPlayerController Function(
  Uri url,
  Future<vp.ClosedCaptionFile>? captions,
);

vp.VideoPlayerController _network(
  Uri url,
  Future<vp.ClosedCaptionFile>? captions,
) => url.scheme == 'file'
    ? vp.VideoPlayerController.file(
        File.fromUri(url),
        closedCaptionFile: captions,
      )
    : vp.VideoPlayerController.networkUrl(url, closedCaptionFile: captions);

/// Whether [url] may be played: only https addresses, so that nobody on the
/// network can change what plays.
bool playableUrl(String url) {
  final uri = Uri.tryParse(url);
  return uri != null && uri.scheme == 'https' && uri.host.isNotEmpty;
}

/// `m:ss`, or `h:mm:ss` from an hour on.
String formatPosition(Duration d) {
  final s = d.inSeconds;
  return s >= 3600
      ? '${s ~/ 3600}:${_two(s ~/ 60 % 60)}:${_two(s % 60)}'
      : '${s ~/ 60}:${_two(s % 60)}';
}

String _two(int n) => n.toString().padLeft(2, '0');

/// A video from an https address with play and pause, a position slider and
/// optional WebVTT captions.
final class PluxVideoPlayer extends StatelessWidget {
  /// Creates the player of [url].
  const PluxVideoPlayer({
    required this.url,
    this.captions,
    this.autoplay = false,
    this.loop = false,
    this.aspectRatio = 16 / 9,
    this.semanticLabel,
    this.playLabel = 'Play',
    this.pauseLabel = 'Pause',
    this.captionsLabel = 'Captions',
    this.positionLabel = 'Position',
    this.onEnded,
    this.onError,
    this.controllerFactory = _network,
    this.resolve,
    super.key,
  });

  /// The https address of the video.
  final String url;

  /// Captions as WebVTT text.
  final String? captions;

  /// Whether it starts by itself; ignored under reduce motion.
  final bool autoplay;

  /// Whether it starts again when it ends.
  final bool loop;

  /// Width over height.
  final double aspectRatio;

  /// What the video is, spoken for the player.
  final String? semanticLabel;

  /// The play button's label.
  final String playLabel;

  /// The pause button's label.
  final String pauseLabel;

  /// The captions switch's label.
  final String captionsLabel;

  /// The position slider's label.
  final String positionLabel;

  /// Called when playback ends.
  final VoidCallback? onEnded;

  /// Called when the video cannot be played.
  final VoidCallback? onError;

  /// Makes the controller; tests put a fake here.
  final PluxPlayerControllerFactory controllerFactory;

  /// Where to read the media at the url from, when the host decides (a
  /// pinned download, SEC-041); null for the url itself. A null result
  /// means the media may not play.
  final Future<Uri?> Function(String url)? resolve;

  @override
  Widget build(BuildContext context) => _MediaPlayer(
    video: true,
    url: url,
    captions: captions,
    autoplay: autoplay,
    loop: loop,
    aspectRatio: aspectRatio,
    label: semanticLabel,
    playLabel: playLabel,
    pauseLabel: pauseLabel,
    captionsLabel: captionsLabel,
    positionLabel: positionLabel,
    onEnded: onEnded,
    onError: onError,
    controllerFactory: controllerFactory,
    resolve: resolve,
  );
}

/// Audio from an https address with play and pause and a position slider.
final class PluxAudioPlayer extends StatelessWidget {
  /// Creates the player of [url].
  const PluxAudioPlayer({
    required this.url,
    this.title,
    this.autoplay = false,
    this.loop = false,
    this.playLabel = 'Play',
    this.pauseLabel = 'Pause',
    this.positionLabel = 'Position',
    this.onEnded,
    this.onError,
    this.controllerFactory = _network,
    this.resolve,
    super.key,
  });

  /// The https address of the audio.
  final String url;

  /// The title shown, and spoken for the player.
  final String? title;

  /// Whether it starts by itself; ignored under reduce motion.
  final bool autoplay;

  /// Whether it starts again when it ends.
  final bool loop;

  /// The play button's label.
  final String playLabel;

  /// The pause button's label.
  final String pauseLabel;

  /// The position slider's label.
  final String positionLabel;

  /// Called when playback ends.
  final VoidCallback? onEnded;

  /// Called when the audio cannot be played.
  final VoidCallback? onError;

  /// Makes the controller; tests put a fake here.
  final PluxPlayerControllerFactory controllerFactory;

  /// Where to read the media at the url from, when the host decides (a
  /// pinned download, SEC-041); null for the url itself. A null result
  /// means the media may not play.
  final Future<Uri?> Function(String url)? resolve;

  @override
  Widget build(BuildContext context) => _MediaPlayer(
    video: false,
    url: url,
    autoplay: autoplay,
    loop: loop,
    label: title,
    playLabel: playLabel,
    pauseLabel: pauseLabel,
    captionsLabel: '',
    positionLabel: positionLabel,
    onEnded: onEnded,
    onError: onError,
    controllerFactory: controllerFactory,
    resolve: resolve,
  );
}

final class _MediaPlayer extends StatefulWidget {
  const _MediaPlayer({
    required this.video,
    required this.url,
    required this.autoplay,
    required this.loop,
    required this.label,
    required this.playLabel,
    required this.pauseLabel,
    required this.captionsLabel,
    required this.positionLabel,
    required this.onEnded,
    required this.onError,
    required this.controllerFactory,
    this.resolve,
    this.captions,
    this.aspectRatio = 16 / 9,
  });

  final bool video;
  final String url;
  final String? captions;
  final bool autoplay;
  final bool loop;
  final double aspectRatio;
  final String? label;
  final String playLabel;
  final String pauseLabel;
  final String captionsLabel;
  final String positionLabel;
  final VoidCallback? onEnded;
  final VoidCallback? onError;
  final PluxPlayerControllerFactory controllerFactory;

  /// Where to read the media at the url from, when the host decides (a
  /// pinned download, SEC-041); null for the url itself. A null result
  /// means the media may not play.
  final Future<Uri?> Function(String url)? resolve;

  @override
  State<_MediaPlayer> createState() => _MediaPlayerState();
}

final class _MediaPlayerState extends State<_MediaPlayer> {
  vp.VideoPlayerController? _controller;
  bool _failed = false;
  bool _captionsOn = true;
  bool _completed = false;
  bool _started = false;
  Uri? _downloaded;

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    if (!_started) {
      _started = true;
      unawaited(_open());
    }
  }

  @override
  void didUpdateWidget(_MediaPlayer old) {
    super.didUpdateWidget(old);
    if (old.url != widget.url || old.captions != widget.captions) {
      _close();
      unawaited(_open());
    } else if (old.loop != widget.loop) {
      unawaited(_controller?.setLooping(widget.loop));
    }
  }

  @override
  void dispose() {
    _close();
    super.dispose();
  }

  void _close() {
    final c = _controller;
    _controller = null;
    c?.removeListener(_changed);
    unawaited(c?.dispose());
    final file = _downloaded;
    _downloaded = null;
    if (file != null) {
      final dir = File.fromUri(file).parent;
      if (dir.existsSync()) dir.deleteSync(recursive: true);
    }
    _failed = false;
    _completed = false;
  }

  void _fail() {
    if (_failed) return;
    setState(() => _failed = true);
    widget.onError?.call();
  }

  Future<void> _open() async {
    if (!playableUrl(widget.url)) {
      // Reported after this frame: a build may not call a callback that
      // changes state.
      scheduleMicrotask(() {
        if (mounted) _fail();
      });
      return;
    }
    final resolve = widget.resolve;
    final source = resolve == null
        ? Uri.parse(widget.url)
        : await resolve(widget.url);
    if (!mounted) return;
    if (source == null) {
      _fail();
      return;
    }
    _downloaded = resolve != null && source.scheme == 'file' ? source : null;
    final text = widget.captions;
    final controller = widget.controllerFactory(
      source,
      widget.video && text != null && text.isNotEmpty
          ? Future.value(vp.WebVTTCaptionFile(text))
          : null,
    );
    _controller = controller;
    controller.addListener(_changed);
    try {
      await controller.initialize();
      if (!mounted || _controller != controller) return;
      await controller.setLooping(widget.loop);
      if (!mounted) return;
      // Reduce motion means nothing starts moving on its own.
      final reduce = MediaQuery.maybeDisableAnimationsOf(context) ?? false;
      if (widget.autoplay && !reduce) await controller.play();
      if (mounted) setState(() {});
    } on Object {
      if (mounted && _controller == controller) _fail();
    }
  }

  void _changed() {
    final c = _controller;
    if (c == null || !mounted) return;
    if (c.value.hasError) {
      _fail();
      return;
    }
    final done = c.value.isCompleted && !widget.loop;
    if (done && !_completed) {
      _completed = true;
      widget.onEnded?.call();
    } else if (!c.value.isCompleted) {
      _completed = false;
    }
    setState(() {});
  }

  void _toggle() {
    final c = _controller;
    if (c == null || !c.value.isInitialized) return;
    if (c.value.isPlaying) {
      unawaited(c.pause());
    } else {
      unawaited(c.play());
    }
  }

  @override
  Widget build(BuildContext context) {
    final c = _controller;
    final ready = c != null && c.value.isInitialized && !_failed;
    final theme = Theme.of(context);
    final playing = ready && c.value.isPlaying;
    final total = ready ? c.value.duration : Duration.zero;
    final at = ready ? c.value.position : Duration.zero;
    final ms = total.inMilliseconds;
    final hasCaptions =
        widget.video && (widget.captions?.isNotEmpty ?? false) && ready;

    final surface = !widget.video
        ? null
        : AspectRatio(
            aspectRatio: widget.aspectRatio,
            child: ColoredBox(
              color: Colors.black,
              child: ready
                  ? Stack(
                      alignment: Alignment.bottomCenter,
                      children: [
                        Positioned.fill(child: vp.VideoPlayer(c)),
                        if (hasCaptions && _captionsOn)
                          vp.ClosedCaption(text: c.value.caption.text),
                      ],
                    )
                  : _failed
                  ? const Center(
                      child: Icon(Icons.error_outline, color: Colors.white),
                    )
                  : null,
            ),
          );

    final controls = Row(
      children: [
        IconButton(
          tooltip: playing ? widget.pauseLabel : widget.playLabel,
          onPressed: ready ? _toggle : null,
          icon: Icon(playing ? Icons.pause : Icons.play_arrow),
        ),
        Expanded(
          child: Semantics(
            container: true,
            label: widget.positionLabel,
            value: '${formatPosition(at)} / ${formatPosition(total)}',
            child: ExcludeSemantics(
              child: Slider(
                value: ms == 0 ? 0 : at.inMilliseconds.clamp(0, ms).toDouble(),
                max: ms == 0 ? 1 : ms.toDouble(),
                onChanged: ready && ms > 0
                    ? (v) =>
                          unawaited(c.seekTo(Duration(milliseconds: v.round())))
                    : null,
              ),
            ),
          ),
        ),
        Text(formatPosition(at), style: theme.textTheme.labelSmall),
        if (hasCaptions)
          IconButton(
            tooltip: widget.captionsLabel,
            isSelected: _captionsOn,
            onPressed: () => setState(() => _captionsOn = !_captionsOn),
            icon: const Icon(Icons.closed_caption_off),
            selectedIcon: const Icon(Icons.closed_caption),
          ),
      ],
    );

    return Semantics(
      container: true,
      label: widget.label,
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        mainAxisSize: MainAxisSize.min,
        children: [
          ?surface,
          if (!widget.video && widget.label != null)
            Padding(
              padding: const EdgeInsets.symmetric(horizontal: 12),
              child: ExcludeSemantics(
                child: Text(widget.label!, style: theme.textTheme.titleSmall),
              ),
            ),
          controls,
        ],
      ),
    );
  }
}
