// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import 'dart:convert';
import 'dart:io';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:plux_flutter/plux_flutter.dart';
import 'package:plux_player/plux_player.dart';
import 'package:video_player/video_player.dart' as vp;

/// A controller that plays nothing: it records what the widget asks of it
/// and lets the test move the value, so no platform player is needed.
final class FakeController extends vp.VideoPlayerController {
  FakeController(Uri url, {this.failToInitialize = false})
    : super.networkUrl(url);

  final bool failToInitialize;
  final List<String> calls = [];
  bool looping = false;

  @override
  Future<void> initialize() async {
    if (failToInitialize) throw StateError('no network');
    value = value.copyWith(
      isInitialized: true,
      duration: const Duration(minutes: 2, seconds: 5),
    );
  }

  @override
  Future<void> play() async {
    calls.add('play');
    value = value.copyWith(isPlaying: true);
  }

  @override
  Future<void> pause() async {
    calls.add('pause');
    value = value.copyWith(isPlaying: false);
  }

  @override
  Future<void> seekTo(Duration position) async {
    calls.add('seek ${position.inSeconds}');
    value = value.copyWith(position: position);
  }

  @override
  Future<void> setLooping(bool looping) async => this.looping = looping;

  void finish() => value = value.copyWith(
    isPlaying: false,
    isCompleted: true,
    position: value.duration,
  );

  void caption(String text) => value = value.copyWith(
    caption: vp.Caption(
      number: 1,
      start: Duration.zero,
      end: const Duration(seconds: 5),
      text: text,
    ),
  );
}

final class FakeNode implements NodeContext {
  FakeNode(this.context, this.props, {this.events = const {}});

  @override
  final BuildContext context;
  final Map<int, Object?> props;
  final Set<int> events;

  @override
  T? decode<T>(int id, T? Function(Decoding d, Object? v) decoder) {
    final v = props[id];
    return v == null ? null : decoder(this, v);
  }

  @override
  Never missing(String name) => throw DecodeError('$name is missing');

  @override
  bool handles(int id) => events.contains(id);

  @override
  void fire(int id, [Object? payload]) {}

  final List<String> asked = [];

  @override
  Future<Uri?> mediaSource(String url) async {
    asked.add(url);
    return Uri.parse(url).host == 'example.com' ? Uri.parse(url) : null;
  }

  @override
  dynamic noSuchMethod(Invocation invocation) => super.noSuchMethod(invocation);
}

Map<String, Object?> _descriptor(String type) => jsonDecode(
  File('../../schema/widgets/layer2/$type.json').readAsStringSync(),
) as Map<String, Object?>;

const _vtt = 'WEBVTT\n\n00:00.000 --> 00:05.000\nHello there\n';

void main() {
  late List<FakeController> made;
  late List<Uri> urls;
  setUp(() {
    made = [];
    urls = [];
  });

  vp.VideoPlayerController factory(
    Uri url,
    Future<vp.ClosedCaptionFile>? captions,
  ) {
    urls.add(url);
    final c = FakeController(url);
    made.add(c);
    return c;
  }

  Widget app(Widget child, {bool reduceMotion = false}) => MaterialApp(
    builder: (context, c) => MediaQuery(
      data: MediaQuery.of(context).copyWith(disableAnimations: reduceMotion),
      child: c!,
    ),
    home: Scaffold(body: child),
  );

  test('the package IDs are those of the descriptors [WGT-020] [RT-060]', () {
    for (final (type, id) in [
      ('VideoPlayer', PluxPlayerIds.videoPlayer),
      ('AudioPlayer', PluxPlayerIds.audioPlayer),
    ]) {
      final d = _descriptor(type);
      expect(d['id'], id, reason: type);
      expect(d['package'], const PluxPlayer().name);
    }
    expect(const PluxPlayer().builders.keys, unorderedEquals([203, 204]));
  });

  test('only https addresses play [SEC-054]', () {
    expect(playableUrl('https://example.com/a.mp4'), isTrue);
    expect(playableUrl('http://example.com/a.mp4'), isFalse);
    expect(playableUrl('file:///etc/passwd'), isFalse);
    expect(playableUrl('/a.mp4'), isFalse);
    expect(formatPosition(const Duration(seconds: 65)), '1:05');
    expect(formatPosition(const Duration(hours: 1, seconds: 5)), '1:00:05');
  });

  testWidgets('it does not start by itself; play and pause are labelled '
      'buttons [WGT-020] [A11Y-001]', (tester) async {
    final handle = tester.ensureSemantics();
    await tester.pumpWidget(
      app(
        PluxVideoPlayer(
          url: 'https://example.com/a.mp4',
          semanticLabel: 'Welcome film',
          controllerFactory: factory,
        ),
      ),
    );
    await tester.pump();
    expect(made.single.calls, isEmpty);
    expect(
      tester
          .getSemantics(find.byType(IconButton).first)
          .getSemanticsData()
          .tooltip,
      'Play',
    );
    expect(find.bySemanticsLabel(RegExp('Welcome film')), findsOneWidget);
    expect(find.bySemanticsLabel(RegExp('Position')), findsOneWidget);
    await tester.tap(find.byTooltip('Play'));
    await tester.pump();
    expect(made.single.calls, ['play']);
    expect(find.byTooltip('Pause'), findsOneWidget);
    await tester.tap(find.byTooltip('Pause'));
    await tester.pump();
    expect(made.single.calls, ['play', 'pause']);
    handle.dispose();
  });

  testWidgets('autoplay starts, except under reduce motion [WGT-020] '
      '[ANI-007]', (tester) async {
    await tester.pumpWidget(
      app(
        PluxVideoPlayer(
          url: 'https://example.com/a.mp4',
          autoplay: true,
          loop: true,
          controllerFactory: factory,
        ),
      ),
    );
    await tester.pump();
    expect(made.last.calls, ['play']);
    expect(made.last.looping, isTrue);

    await tester.pumpWidget(const SizedBox());
    await tester.pumpWidget(
      app(
        PluxAudioPlayer(
          url: 'https://example.com/a.mp3',
          autoplay: true,
          controllerFactory: factory,
        ),
        reduceMotion: true,
      ),
    );
    await tester.pump();
    expect(made.last.calls, isEmpty);
  });

  testWidgets('seeking moves the position; ending fires onEnded once '
      '[WGT-020]', (tester) async {
    var ended = 0;
    await tester.pumpWidget(
      app(
        PluxAudioPlayer(
          url: 'https://example.com/a.mp3',
          title: 'Podcast',
          onEnded: () => ended++,
          controllerFactory: factory,
        ),
      ),
    );
    await tester.pump();
    expect(find.text('Podcast'), findsOneWidget);
    await tester.tapAt(tester.getCenter(find.byType(Slider)));
    await tester.pump();
    expect(made.single.calls.single, startsWith('seek '));
    made.single.finish();
    await tester.pump();
    made.single.finish();
    await tester.pump();
    expect(ended, 1);
  });

  testWidgets('captions show and can be switched off [WGT-020]', (
    tester,
  ) async {
    await tester.pumpWidget(
      app(
        PluxVideoPlayer(
          url: 'https://example.com/a.mp4',
          captions: _vtt,
          controllerFactory: factory,
        ),
      ),
    );
    await tester.pump();
    made.single.caption('Hello there');
    await tester.pump();
    expect(find.text('Hello there'), findsOneWidget);
    await tester.tap(find.byTooltip('Captions'));
    await tester.pump();
    expect(find.text('Hello there'), findsNothing);
  });

  testWidgets('a bad address or a failing player reports onError, never '
      'plays [WGT-020] [SEC-054]', (tester) async {
    var errors = 0;
    await tester.pumpWidget(
      app(
        PluxVideoPlayer(
          url: 'http://example.com/a.mp4',
          onError: () => errors++,
          controllerFactory: factory,
        ),
      ),
    );
    await tester.pump();
    expect(errors, 1);
    expect(urls, isEmpty);
    expect(find.byIcon(Icons.error_outline), findsOneWidget);

    await tester.pumpWidget(const SizedBox());
    await tester.pumpWidget(
      app(
        PluxAudioPlayer(
          url: 'https://example.com/a.mp3',
          onError: () => errors++,
          controllerFactory: (url, _) =>
              FakeController(url, failToInitialize: true),
        ),
      ),
    );
    await tester.pump();
    expect(errors, 2);
    expect(
      tester.widget<IconButton>(find.byType(IconButton)).onPressed,
      isNull,
    );
  });

  testWidgets('the builders read props by ID [WGT-020]', (tester) async {
    late Widget video;
    late Widget audio;
    await tester.pumpWidget(
      Builder(
        builder: (context) {
          const pkg = PluxPlayer();
          video = pkg.builders[203]!(
            FakeNode(
              context,
              {1: 'https://example.com/a.mp4', 3: true, 5: 1.0},
              events: {1},
            ),
          );
          audio = pkg.builders[204]!(
            FakeNode(context, {1: 'https://example.com/a.mp3', 2: 'Song'}),
          );
          return const SizedBox();
        },
      ),
    );
    final v = video as PluxVideoPlayer;
    expect(v.autoplay, isTrue);
    expect(v.aspectRatio, 1.0);
    expect(v.onEnded, isNotNull);
    expect(v.onError, isNull);
    expect((audio as PluxAudioPlayer).title, 'Song');
    expect((audio as PluxAudioPlayer).autoplay, isFalse);
  });

  testWidgets('a source the node refuses never plays; one it allows does '
      '[WGT-020] [SEC-080] [SEC-041] [AST-002]', (tester) async {
    var errors = 0;
    late FakeNode node;
    await tester.pumpWidget(
      Builder(
        builder: (context) {
          node = FakeNode(context, {1: 'https://evil.example.net/a.mp4'});
          return app(
            PluxVideoPlayer(
              url: 'https://evil.example.net/a.mp4',
              onError: () => errors++,
              controllerFactory: factory,
              resolve: node.mediaSource,
            ),
          );
        },
      ),
    );
    await tester.pump();
    await tester.pump();
    expect(errors, 1);
    expect(urls, isEmpty);
    expect(node.asked, ['https://evil.example.net/a.mp4']);

    await tester.pumpWidget(const SizedBox());
    await tester.pumpWidget(
      app(
        PluxVideoPlayer(
          url: 'https://example.com/a.mp4',
          controllerFactory: factory,
          resolve: node.mediaSource,
        ),
      ),
    );
    await tester.pump();
    await tester.pump();
    expect(urls, [Uri.parse('https://example.com/a.mp4')]);
  });

  testWidgets('the builders hand the node as the source resolver '
      '[WGT-020] [SEC-041]', (tester) async {
    late Widget video;
    await tester.pumpWidget(
      Builder(
        builder: (context) {
          video = const PluxPlayer().builders[203]!(
            FakeNode(context, {1: 'https://example.com/a.mp4'}),
          );
          return const SizedBox();
        },
      ),
    );
    expect((video as PluxVideoPlayer).resolve, isNotNull);
  });
}
