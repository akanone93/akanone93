// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The debugging overlays of a dev session (`DEV-034`): a frame chart, the
/// rebuild count of each node, the interactive nodes below the minimum
/// target size (`A11Y-004`) and the text with too little contrast
/// (`A11Y-003`), drawn above the app by [DevToolsLayer]. Only builds with
/// `kPluxDevtools` create any of it.
library;

import 'package:flutter/rendering.dart';
import 'package:flutter/scheduler.dart';
import 'package:flutter/widgets.dart';
import 'package:plux_flutter/src/devtools_api/dev_inspect.dart';
import 'package:plux_flutter/src/devtools_api/dev_nodes.dart';
import 'package:plux_flutter/src/devtools_api/dev_session.dart';
import 'package:plux_flutter/src/render/min_target.dart';

/// The lowest contrast ratio of normal text (WCAG AA).
const double minTextContrast = 4.5;

/// The lowest contrast ratio of large text (WCAG AA).
const double minLargeTextContrast = 3;

/// The contrast ratio of [foreground] over [background], from 1 to 21.
double contrastRatio(Color foreground, Color background) {
  final a = foreground.computeLuminance();
  final b = background.computeLuminance();
  final hi = a > b ? a : b;
  final lo = a > b ? b : a;
  return (hi + 0.05) / (lo + 0.05);
}

/// Carries out `toggleOverlay`.
final class DevOverlaysHandler implements DevCommandHandler {
  /// Creates the handler.
  const DevOverlaysHandler({required this.host});

  /// The state of the tools.
  final DevNodeHost host;

  @override
  Future<Map<String, Object?>> handle(Map<String, Object?> command) async {
    final body = devCommandBody(command, 'toggleOverlay');
    if (body == null) {
      return devFailure(devStatusFailed, 'toggleOverlay is not an object');
    }
    final overlay = DevOverlayKind.fromProto(body['overlay']);
    if (overlay == null) {
      return devFailure(devStatusFailed, 'toggleOverlay names no overlay');
    }
    host.setOverlay(overlay, enabled: body['enabled'] == true);
    return {'status': devStatusOk};
  }
}

/// The interactive nodes on screen smaller than [minTargetSize] in either
/// direction.
List<DevNodeInfo> smallTargets(DevNodeHost host) => [
  for (final m in host.markers)
    if (m.info.interactive &&
        m.hasSize &&
        (m.size.width < minTargetSize || m.size.height < minTargetSize))
      m.info,
];

/// The nodes on screen holding text whose colour has less contrast against
/// its background than WCAG AA asks for. The background is the nearest
/// opaque colour behind the text; text with no known colour or background
/// is not judged.
List<DevNodeInfo> lowContrast(DevNodeHost host) => [
  for (final m in host.markers)
    if (_hasLowContrast(m)) m.info,
];

bool _hasLowContrast(RenderObject root) {
  var low = false;
  void visit(RenderObject o) {
    if (low) return;
    if (o is RenderParagraph) {
      final text = o.text;
      final style = text.style;
      final color = style?.color;
      final background = _backgroundBehind(o);
      if (color != null && background != null) {
        final size = style?.fontSize ?? 14;
        final bold = (style?.fontWeight?.value ?? 400) >= 700;
        final large = size >= 24 || (size >= 18.66 && bold);
        final shown = Color.alphaBlend(color, background);
        low =
            contrastRatio(shown, background) <
            (large ? minLargeTextContrast : minTextContrast);
      }
      return;
    }
    o.visitChildren(visit);
  }

  root.visitChildren(visit);
  return low;
}

Color? _backgroundBehind(RenderObject from) {
  for (RenderObject? o = from.parent; o != null; o = o.parent) {
    final color = _fill(o);
    if (color != null && color.a == 1) return color;
  }
  return null;
}

Color? _fill(RenderObject o) {
  if (o is RenderDecoratedBox) {
    final d = o.decoration;
    return d is BoxDecoration ? d.color : null;
  }
  for (final p in o.toDiagnosticsNode().getProperties()) {
    final v = p.value;
    if (p.name == 'color' && v is Color) return v;
  }
  return null;
}

/// Draws the tools of [host] above [child], and catches the taps of
/// inspect mode.
final class DevToolsLayer extends StatefulWidget {
  /// Creates the layer.
  const DevToolsLayer({super.key, required this.host, required this.child});

  /// The state of the tools.
  final DevNodeHost host;

  /// The app.
  final Widget child;

  /// The key of the frame chart while it is on.
  static const Key frameChartKey = Key('plux-dev-frame-chart');

  /// The key of the rebuild counters while they are on.
  static const Key rebuildCountersKey = Key('plux-dev-rebuild-counters');

  /// The key of the touch target highlight while it is on.
  static const Key touchTargetsKey = Key('plux-dev-touch-targets');

  /// The key of the contrast highlight while it is on.
  static const Key contrastKey = Key('plux-dev-contrast');

  /// The key of the outline of the selected node while there is one.
  static const Key selectionKey = Key('plux-dev-selection');

  /// The key of the inspect barrier while inspect mode is on.
  static const Key barrierKey = Key('plux-dev-inspect-barrier');

  @override
  State<DevToolsLayer> createState() => _DevToolsLayerState();
}

final class _DevToolsLayerState extends State<DevToolsLayer> {
  final GlobalKey _app = GlobalKey();
  final ValueNotifier<int> _tick = ValueNotifier(0);
  bool _ticking = false;

  @override
  void initState() {
    super.initState();
    widget.host.addListener(_changed);
    SchedulerBinding.instance.addTimingsCallback(_timings);
    _changed();
  }

  @override
  void didUpdateWidget(DevToolsLayer old) {
    super.didUpdateWidget(old);
    if (!identical(old.host, widget.host)) {
      old.host.removeListener(_changed);
      widget.host.addListener(_changed);
    }
  }

  @override
  void dispose() {
    widget.host.removeListener(_changed);
    SchedulerBinding.instance.removeTimingsCallback(_timings);
    _tick.dispose();
    super.dispose();
  }

  void _timings(List<FrameTiming> timings) {
    for (final t in timings) {
      widget.host.recordFrame(DevFrame(t.buildDuration, t.rasterDuration));
    }
  }

  void _changed() {
    if (!mounted) return;
    setState(() {});
    _follow();
  }

  /// Repaints the highlights after every frame while one is on, since the
  /// nodes move and rebuild without the host's knowing.
  void _follow() {
    final host = widget.host;
    final live =
        host.selected != null ||
        host.overlays.any((o) => o != DevOverlayKind.frameChart);
    if (!live || _ticking) return;
    _ticking = true;
    SchedulerBinding.instance.addPostFrameCallback((_) {
      _ticking = false;
      if (!mounted) return;
      _tick.value++;
      _follow();
    });
    SchedulerBinding.instance.scheduleFrame();
  }

  @override
  Widget build(BuildContext context) {
    final host = widget.host;
    final on = host.overlays;
    return Directionality(
      textDirection: TextDirection.ltr,
      child: Stack(
        fit: StackFit.expand,
        children: [
          KeyedSubtree(key: _app, child: widget.child),
          if (host.inspecting)
            Positioned.fill(
              key: DevToolsLayer.barrierKey,
              child: DevInspectBarrier(host: host, appKey: _app),
            ),
          if (host.selected != null)
            _paint(
              DevToolsLayer.selectionKey,
              (c, size, box) => _outlines(c, box, [
                ?host.find(host.selected!),
              ], const Color(0xFF2962FF)),
            ),
          if (on.contains(DevOverlayKind.touchTargets))
            _paint(
              DevToolsLayer.touchTargetsKey,
              (c, size, box) => _outlines(c, box, [
                for (final m in host.markers)
                  if (smallTargets(host).contains(m.info)) m,
              ], const Color(0xFFD50000)),
            ),
          if (on.contains(DevOverlayKind.contrast))
            _paint(
              DevToolsLayer.contrastKey,
              (c, size, box) => _outlines(c, box, [
                for (final m in host.markers)
                  if (lowContrast(host).contains(m.info)) m,
              ], const Color(0xFFFF6D00)),
            ),
          if (on.contains(DevOverlayKind.rebuildCounters))
            _paint(DevToolsLayer.rebuildCountersKey, _counters),
          if (on.contains(DevOverlayKind.frameChart))
            Positioned(
              key: DevToolsLayer.frameChartKey,
              left: 8,
              right: 8,
              top: 32,
              height: 64,
              child: IgnorePointer(
                child: CustomPaint(painter: _FrameChart(host.frames)),
              ),
            ),
        ],
      ),
    );
  }

  Widget _paint(
    Key key,
    void Function(Canvas canvas, Size size, RenderBox layer) draw,
  ) => Positioned.fill(
    key: key,
    child: IgnorePointer(
      child: Builder(
        builder: (context) => CustomPaint(
          painter: _Painter(_tick, (canvas, size) {
            final box = context.findRenderObject();
            if (box is RenderBox) draw(canvas, size, box);
          }),
        ),
      ),
    ),
  );

  void _outlines(
    Canvas canvas,
    RenderBox layer,
    List<RenderDevNode> markers,
    Color color,
  ) {
    final paint = Paint()
      ..style = PaintingStyle.stroke
      ..strokeWidth = 2
      ..color = color;
    for (final m in markers) {
      final rect = _rectOf(m, layer);
      if (rect != null) canvas.drawRect(rect, paint);
    }
  }

  void _counters(Canvas canvas, Size size, RenderBox layer) {
    for (final m in widget.host.markers) {
      final rect = _rectOf(m, layer);
      if (rect == null) continue;
      final label = TextPainter(
        text: TextSpan(
          text: '${widget.host.rebuilds(m.info.path)}',
          style: const TextStyle(
            color: Color(0xFFFFFFFF),
            backgroundColor: Color(0xCC000000),
            fontSize: 10,
          ),
        ),
        textDirection: TextDirection.ltr,
      )..layout();
      label.paint(canvas, rect.topLeft);
      label.dispose();
    }
  }
}

Rect? _rectOf(RenderDevNode m, RenderBox layer) {
  if (!m.attached || !m.hasSize || !layer.attached) return null;
  return MatrixUtils.transformRect(
    m.getTransformTo(layer),
    Offset.zero & m.size,
  );
}

final class _Painter extends CustomPainter {
  _Painter(Listenable repaint, this._draw) : super(repaint: repaint);

  final void Function(Canvas canvas, Size size) _draw;

  @override
  void paint(Canvas canvas, Size size) => _draw(canvas, size);

  @override
  bool shouldRepaint(_Painter old) => true;
}

final class _FrameChart extends CustomPainter {
  _FrameChart(this.frames);

  final List<DevFrame> frames;

  @override
  void paint(Canvas canvas, Size size) {
    canvas.drawRect(
      Offset.zero & size,
      Paint()..color = const Color(0x99000000),
    );
    const budget = 1000 / 60;
    final scale = size.height / (budget * 3);
    final width = size.width / DevNodeHost.frameCapacity;
    for (var i = 0; i < frames.length; i++) {
      final f = frames[i];
      final build = f.build.inMicroseconds / 1000 * scale;
      final raster = f.raster.inMicroseconds / 1000 * scale;
      final x = i * width;
      final late = f.total.inMicroseconds / 1000 > budget;
      canvas
        ..drawRect(
          Rect.fromLTWH(x, size.height - build, width - 1, build),
          Paint()..color = const Color(0xFF42A5F5),
        )
        ..drawRect(
          Rect.fromLTWH(x, size.height - build - raster, width - 1, raster),
          Paint()
            ..color = late ? const Color(0xFFEF5350) : const Color(0xFF66BB6A),
        );
    }
    final line = size.height - budget * scale;
    canvas.drawLine(
      Offset(0, line),
      Offset(size.width, line),
      Paint()
        ..color = const Color(0xFFFFFFFF)
        ..strokeWidth = 1,
    );
  }

  @override
  bool shouldRepaint(_FrameChart old) => true;
}
