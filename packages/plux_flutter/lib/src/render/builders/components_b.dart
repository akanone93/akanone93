// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// Layer 2 commerce, campaign, rating and media components of P8
/// (WGT-020, Appendix C.9, ADR-0071): `LiveEtaCard`, `ProductCard`,
/// `QuantityStepper`, `Carousel`, `PromoBanner`, `CountdownTimer`,
/// `RatingBar`, `RatingInput`, `ImageGallery` and `FileUploader`. Each is
/// built from Layer 1 widgets and the page's theme. Their screen-reader
/// scripts are in docs/accessibility/layer2-screen-reader.md.
///
/// Prices arrive as formatted strings; nothing here formats money.
library;

import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:flutter/material.dart';
import 'package:plux_flutter/src/actions/action_error.dart';
import 'package:plux_flutter/src/assets/icon_fonts.dart';
import 'package:plux_flutter/src/data/services.dart' show DataScope;
import 'package:plux_flutter/src/device/types.dart';
import 'package:plux_flutter/src/errors/plux_exception.dart';
import 'package:plux_flutter/src/l10n/runtime_strings.dart';
import 'package:plux_flutter/src/pxl/types.dart' show toJson;
import 'package:plux_flutter/src/pxl/values.dart';
import 'package:plux_flutter/src/render/decoders.dart';
import 'package:plux_flutter/src/render/decoding.dart';
import 'package:plux_flutter/src/render/generated/render.g.dart';
import 'package:plux_flutter/src/render/node_context.dart';
import 'package:plux_flutter/src/render/plux_icon.dart';
import 'package:plux_flutter/src/render/scope.dart';
import 'package:plux_flutter/src/schema/registry.g.dart';

/// A descriptor with name lookups for its permanent IDs.
final class _D {
  _D(String type) : d = widgetDescriptors.firstWhere((w) => w.type == type);

  final WidgetDescriptor d;

  int get id => d.id;
  int p(String name) => d.props[name]!;
  int e(String name) => d.events[name]!;
}

final _eta = _D('LiveEtaCard');
final _product = _D('ProductCard');
final _stepper = _D('QuantityStepper');
final _carousel = _D('Carousel');
final _promo = _D('PromoBanner');
final _countdown = _D('CountdownTimer');
final _ratingBar = _D('RatingBar');
final _ratingInput = _D('RatingInput');
final _gallery = _D('ImageGallery');
final _uploader = _D('FileUploader');

/// The builders of this group, by permanent widget ID.
final Map<int, NodeBuilder> componentsBBuilders = {
  _eta.id: buildLiveEtaCard,
  _product.id: buildProductCard,
  _stepper.id: buildQuantityStepper,
  _carousel.id: buildCarousel,
  _promo.id: buildPromoBanner,
  _countdown.id: buildCountdownTimer,
  _ratingBar.id: buildRatingBar,
  _ratingInput.id: buildRatingInput,
  _gallery.id: buildImageGallery,
  _uploader.id: buildFileUploader,
};

/// The time source of countdowns. Production pages use the device clock;
/// tests and previews put one of these above the page to drive time.
final class CountdownClock extends InheritedWidget {
  /// Provides [now] to the countdowns below.
  const CountdownClock({super.key, required this.now, required super.child});

  /// The current time.
  final DateTime Function() now;

  /// The current time as seen from [context].
  static DateTime nowOf(BuildContext context) {
    final clock = context.getInheritedWidgetOfExactType<CountdownClock>();
    return clock?.now() ?? DateTime.now();
  }

  @override
  bool updateShouldNotify(CountdownClock old) => now != old.now;
}

DateTime? _dateTime(Object? v) => v is PxlDateTime
    ? DateTime.fromMillisecondsSinceEpoch(v.millis, isUtc: true)
    : null;

Widget _picture(NodeContext c, ImageProvider<Object> image) => Image(
  image: image,
  fit: BoxFit.cover,
  excludeFromSemantics: true,
  errorBuilder: (context, error, _) {
    c.imageFailed(error);
    return ColoredBox(
      color: Theme.of(context).colorScheme.surfaceContainerHighest,
      child: const Center(child: Icon(Icons.broken_image_outlined)),
    );
  },
);

// ── LiveEtaCard ────────────────────────────────────────────────────────────

/// `LiveEtaCard`: title, minutes left and status. The card is a live region,
/// so a screen reader announces each new estimate or status.
Widget buildLiveEtaCard(NodeContext c) => _LiveEta(
  title: c.decode(_eta.p('title'), asString),
  minutes: c.decode(_eta.p('etaMinutes'), asInt),
  status: c.decode(_eta.p('status'), asString),
  unit: c.decode(_eta.p('minutesLabel'), asString) ?? 'min',
  icon: c.decode(_eta.p('icon'), decodeIconData),
);

final class _LiveEta extends StatelessWidget {
  const _LiveEta({
    required this.title,
    required this.minutes,
    required this.status,
    required this.unit,
    required this.icon,
  });

  final String? title;
  final int? minutes;
  final String? status;
  final String unit;
  final PluxIconSource? icon;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final scheme = theme.colorScheme;
    final title = this.title, minutes = this.minutes, status = this.status;
    final icon = this.icon;
    final label = [
      ?title,
      if (minutes != null) '$minutes $unit',
      ?status,
    ].join(', ');
    return Semantics(
      container: true,
      liveRegion: true,
      label: label,
      child: ExcludeSemantics(
        child: Card(
          margin: EdgeInsets.zero,
          child: Padding(
            padding: const EdgeInsetsDirectional.all(16),
            child: Row(
              children: [
                if (icon != null) ...[
                  PluxIcon(icon, size: 32, color: scheme.primary),
                  const SizedBox(width: 16),
                ],
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      if (title != null)
                        Text(title, style: theme.textTheme.labelLarge),
                      if (minutes != null)
                        Text(
                          '$minutes $unit',
                          style: theme.textTheme.headlineSmall,
                        ),
                      if (status != null)
                        Text(
                          status,
                          style: theme.textTheme.bodyMedium?.copyWith(
                            color: scheme.onSurfaceVariant,
                          ),
                        ),
                    ],
                  ),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}

// ── ProductCard ────────────────────────────────────────────────────────────

/// `ProductCard`: image, badge, title, price (and the struck-through
/// original price), tappable as a whole, with an add-to-cart button when
/// `onAddToCart` is handled.
Widget buildProductCard(NodeContext c) {
  final image = c.decode(_product.p('image'), decodeImageSource);
  final onTap = _product.e('onTap'), onAdd = _product.e('onAddToCart');
  return _ProductCard(
    image: image == null ? null : _picture(c, image),
    title:
        c.decode(_product.p('title'), asString) ??
        c.missing('ProductCard.title'),
    price:
        c.decode(_product.p('price'), asString) ??
        c.missing('ProductCard.price'),
    originalPrice: c.decode(_product.p('originalPrice'), asString),
    badge: c.decode(_product.p('badge'), asString),
    addLabel:
        c.decode(_product.p('addToCartLabel'), asString) ??
        RuntimeStrings.of(c.context)['plux.product.addToCart'],
    onTap: c.handles(onTap) ? () => c.fire(onTap) : null,
    onAdd: c.handles(onAdd) ? () => c.fire(onAdd) : null,
  );
}

final class _ProductCard extends StatelessWidget {
  const _ProductCard({
    required this.image,
    required this.title,
    required this.price,
    required this.originalPrice,
    required this.badge,
    required this.addLabel,
    required this.onTap,
    required this.onAdd,
  });

  final Widget? image;
  final String title;
  final String price;
  final String? originalPrice;
  final String? badge;
  final String addLabel;
  final VoidCallback? onTap;
  final VoidCallback? onAdd;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final scheme = theme.colorScheme;
    final badge = this.badge, original = originalPrice, image = this.image;
    return Card(
      margin: EdgeInsets.zero,
      clipBehavior: Clip.antiAlias,
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        mainAxisSize: MainAxisSize.min,
        children: [
          Semantics(
            container: true,
            button: onTap != null,
            onTap: onTap,
            child: InkWell(
              excludeFromSemantics: true,
              onTap: onTap,
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                mainAxisSize: MainAxisSize.min,
                children: [
                  AspectRatio(
                    aspectRatio: 4 / 3,
                    child: Stack(
                      fit: StackFit.expand,
                      children: [
                        image ??
                            ColoredBox(color: scheme.surfaceContainerHighest),
                        if (badge != null)
                          PositionedDirectional(
                            top: 8,
                            start: 8,
                            child: DecoratedBox(
                              decoration: BoxDecoration(
                                color: scheme.tertiaryContainer,
                                borderRadius: BorderRadius.circular(12),
                              ),
                              child: Padding(
                                padding: const EdgeInsetsDirectional.symmetric(
                                  horizontal: 8,
                                  vertical: 4,
                                ),
                                child: Text(
                                  badge,
                                  style: theme.textTheme.labelSmall?.copyWith(
                                    color: scheme.onTertiaryContainer,
                                  ),
                                ),
                              ),
                            ),
                          ),
                      ],
                    ),
                  ),
                  Padding(
                    padding: const EdgeInsetsDirectional.fromSTEB(
                      12,
                      12,
                      12,
                      8,
                    ),
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text(
                          title,
                          style: theme.textTheme.titleSmall,
                          maxLines: 2,
                          overflow: TextOverflow.ellipsis,
                        ),
                        const SizedBox(height: 4),
                        Wrap(
                          spacing: 8,
                          crossAxisAlignment: WrapCrossAlignment.center,
                          children: [
                            Text(price, style: theme.textTheme.titleMedium),
                            if (original != null)
                              Text(
                                original,
                                style: theme.textTheme.bodySmall?.copyWith(
                                  color: scheme.onSurfaceVariant,
                                  decoration: TextDecoration.lineThrough,
                                ),
                              ),
                          ],
                        ),
                      ],
                    ),
                  ),
                ],
              ),
            ),
          ),
          if (onAdd != null)
            Padding(
              padding: const EdgeInsetsDirectional.fromSTEB(12, 0, 12, 12),
              child: FilledButton.tonal(
                onPressed: onAdd,
                child: Text(addLabel),
              ),
            ),
        ],
      ),
    );
  }
}

// ── QuantityStepper ────────────────────────────────────────────────────────

/// `QuantityStepper`: the quantity between a decrease and an increase
/// button, kept locally and following the bound `value`. Screen readers
/// adjust it with increase and decrease actions, as a slider.
Widget buildQuantityStepper(NodeContext c) {
  final min = c.decode(_stepper.p('min'), asInt) ?? 0;
  final max = c.decode(_stepper.p('max'), asInt) ?? 99;
  final step = (c.decode(_stepper.p('step'), asInt) ?? 1).clamp(1, 1 << 30);
  final value = c.decode(_stepper.p('value'), asInt) ?? 1;
  final decrease =
      c.decode(_stepper.p('decreaseLabel'), asString) ??
      RuntimeStrings.of(c.context)['plux.stepper.decrease'];
  final increase =
      c.decode(_stepper.p('increaseLabel'), asString) ??
      RuntimeStrings.of(c.context)['plux.stepper.increase'];
  final hi = max < min ? min : max;
  return c.controlled<int>(_stepper.e('onChanged'), value.clamp(min, hi), (
    v,
    onChanged,
  ) {
    final down = (v - step).clamp(min, hi);
    final up = (v + step).clamp(min, hi);
    final canDown = onChanged != null && v > min;
    final canUp = onChanged != null && v < hi;
    return Semantics(
      container: true,
      value: '$v',
      increasedValue: '$up',
      decreasedValue: '$down',
      onIncrease: canUp ? () => onChanged(up) : null,
      onDecrease: canDown ? () => onChanged(down) : null,
      excludeSemantics: true,
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          IconButton.filledTonal(
            tooltip: decrease,
            onPressed: canDown ? () => onChanged(down) : null,
            icon: const Icon(Icons.remove),
          ),
          ConstrainedBox(
            constraints: const BoxConstraints(minWidth: 40),
            child: Text(
              '$v',
              textAlign: TextAlign.center,
              style: Theme.of(c.context).textTheme.titleMedium,
            ),
          ),
          IconButton.filledTonal(
            tooltip: increase,
            onPressed: canUp ? () => onChanged(up) : null,
            icon: const Icon(Icons.add),
          ),
        ],
      ),
    );
  });
}

// ── Carousel ───────────────────────────────────────────────────────────────

/// `Carousel`: its children as swipeable pages with indicators. With
/// `autoplay` it advances every `intervalMs`, except while the device
/// reduces motion or a finger is down.
Widget buildCarousel(NodeContext c) {
  final onPage = _carousel.e('onPageChanged');
  return _CarouselView(
    pages: c.children(),
    autoplay: c.decode(_carousel.p('autoplay'), asBool) ?? false,
    interval: Duration(
      milliseconds: (c.decode(_carousel.p('intervalMs'), asInt) ?? 5000).clamp(
        500,
        1 << 30,
      ),
    ),
    indicators: c.decode(_carousel.p('indicators'), asBool) ?? true,
    label: c.decode(_carousel.p('semanticLabel'), asString),
    onPageChanged: c.handles(onPage) ? (i) => c.fire(onPage, i) : null,
  );
}

final class _CarouselView extends StatefulWidget {
  const _CarouselView({
    required this.pages,
    required this.autoplay,
    required this.interval,
    required this.indicators,
    required this.label,
    required this.onPageChanged,
  });

  final List<Widget> pages;
  final bool autoplay;
  final Duration interval;
  final bool indicators;
  final String? label;
  final ValueChanged<int>? onPageChanged;

  @override
  State<_CarouselView> createState() => _CarouselViewState();
}

final class _CarouselViewState extends State<_CarouselView> {
  final PageController _controller = PageController();
  Timer? _timer;
  int _page = 0;

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    _schedule();
  }

  @override
  void didUpdateWidget(_CarouselView old) {
    super.didUpdateWidget(old);
    _schedule();
  }

  void _stop() {
    _timer?.cancel();
    _timer = null;
  }

  void _schedule() {
    _stop();
    if (!widget.autoplay ||
        widget.pages.length < 2 ||
        MediaQuery.disableAnimationsOf(context)) {
      return;
    }
    _timer = Timer.periodic(widget.interval, (_) {
      if (!_controller.hasClients || widget.pages.isEmpty) return;
      _controller.animateToPage(
        (_page + 1) % widget.pages.length,
        duration: const Duration(milliseconds: 300),
        curve: Curves.easeInOut,
      );
    });
  }

  @override
  void dispose() {
    _stop();
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    final still = MediaQuery.disableAnimationsOf(context);
    final count = widget.pages.length;
    final current = count == 0 ? 0 : _page.clamp(0, count - 1);
    return Semantics(
      container: true,
      label: widget.label,
      child: LayoutBuilder(
        builder: (context, box) {
          final height = box.hasBoundedHeight
              ? box.maxHeight
              : box.maxWidth.isFinite
              ? box.maxWidth * 9 / 16
              : 200.0;
          return SizedBox(
            height: height,
            child: Stack(
              children: [
                Listener(
                  onPointerDown: (_) => _stop(),
                  onPointerUp: (_) => _schedule(),
                  onPointerCancel: (_) => _schedule(),
                  child: PageView(
                    controller: _controller,
                    onPageChanged: (i) {
                      setState(() => _page = i);
                      widget.onPageChanged?.call(i);
                    },
                    children: widget.pages,
                  ),
                ),
                if (widget.indicators && count > 1)
                  PositionedDirectional(
                    start: 0,
                    end: 0,
                    bottom: 8,
                    child: ExcludeSemantics(
                      child: Row(
                        mainAxisAlignment: MainAxisAlignment.center,
                        children: [
                          for (var i = 0; i < count; i++)
                            AnimatedContainer(
                              duration: still
                                  ? Duration.zero
                                  : const Duration(milliseconds: 200),
                              margin: const EdgeInsets.symmetric(horizontal: 3),
                              width: i == current ? 16 : 8,
                              height: 8,
                              decoration: BoxDecoration(
                                borderRadius: BorderRadius.circular(4),
                                color: i == current
                                    ? scheme.primary
                                    : scheme.outlineVariant,
                              ),
                            ),
                        ],
                      ),
                    ),
                  ),
              ],
            ),
          );
        },
      ),
    );
  }
}

// ── CountdownTimer and PromoBanner ─────────────────────────────────────────

/// `CountdownTimer`: the time left until `endsAt`, ticking once a second.
Widget buildCountdownTimer(NodeContext c) {
  final onFinished = _countdown.e('onFinished');
  return CountdownView(
    endsAt:
        _dateTime(c.prop(_countdown.p('endsAt'))) ??
        c.missing('CountdownTimer.endsAt'),
    format: c.decode(_countdown.p('format'), nameCountdownFormat) ?? 'hms',
    onFinished: c.handles(onFinished) ? () => c.fire(onFinished) : null,
  );
}

/// The remaining time written in [format]: `hms`, `dhms` or `compact`.
String formatCountdown(Duration remaining, String format) {
  final total = remaining.isNegative
      ? 0
      : (remaining.inMilliseconds + 999) ~/ 1000;
  final d = total ~/ 86400, m = total % 3600 ~/ 60, s = total % 60;
  String two(int n) => n.toString().padLeft(2, '0');
  return switch (format) {
    'dhms' => '${d}d ${two(total % 86400 ~/ 3600)}:${two(m)}:${two(s)}',
    'compact' =>
      d > 0
          ? '${d}d ${total % 86400 ~/ 3600}h'
          : total >= 3600
          ? '${total ~/ 3600}h ${m}m'
          : m > 0
          ? '${m}m ${s}s'
          : '${s}s',
    _ => '${two(total ~/ 3600)}:${two(m)}:${two(s)}',
  };
}

/// A countdown to [endsAt] that rebuilds once a second and calls
/// [onFinished] once when it reaches zero. The timer is cancelled with the
/// widget. Time comes from [CountdownClock].
final class CountdownView extends StatefulWidget {
  /// Creates a countdown.
  const CountdownView({
    super.key,
    required this.endsAt,
    this.format = 'hms',
    this.onFinished,
    this.style,
  });

  /// The instant the countdown ends.
  final DateTime endsAt;

  /// `hms`, `dhms` or `compact`.
  final String format;

  /// Called once at zero.
  final VoidCallback? onFinished;

  /// The text style; the theme's `titleMedium` by default.
  final TextStyle? style;

  @override
  State<CountdownView> createState() => _CountdownViewState();
}

final class _CountdownViewState extends State<CountdownView> {
  Timer? _timer;
  bool _fired = false;

  @override
  void initState() {
    super.initState();
    _timer = Timer.periodic(const Duration(seconds: 1), (_) => _tick());
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (mounted) _checkFinished();
    });
  }

  @override
  void didUpdateWidget(CountdownView old) {
    super.didUpdateWidget(old);
    if (old.endsAt != widget.endsAt) {
      _fired = false;
      _timer?.cancel();
      _timer = Timer.periodic(const Duration(seconds: 1), (_) => _tick());
    }
  }

  Duration get _remaining =>
      widget.endsAt.difference(CountdownClock.nowOf(context));

  void _tick() {
    if (!mounted) return;
    setState(() {});
    _checkFinished();
  }

  void _checkFinished() {
    if (_fired || !_remaining.isNegative && _remaining != Duration.zero) return;
    _fired = true;
    _timer?.cancel();
    _timer = null;
    widget.onFinished?.call();
  }

  @override
  void dispose() {
    _timer?.cancel();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final text = formatCountdown(_remaining, widget.format);
    return Semantics(
      label: text,
      excludeSemantics: true,
      child: Text(
        text,
        textDirection: TextDirection.ltr,
        style: (widget.style ?? Theme.of(context).textTheme.titleMedium)
            ?.copyWith(fontFeatures: const [FontFeature.tabularFigures()]),
      ),
    );
  }
}

/// `PromoBanner`: title, subtitle, call to action and, with `endsAt`, a
/// countdown, over an image. The whole banner is one button.
Widget buildPromoBanner(NodeContext c) {
  final image = c.decode(_promo.p('image'), decodeImageSource);
  final onTap = _promo.e('onTap');
  return _Promo(
    title:
        c.decode(_promo.p('title'), asString) ?? c.missing('PromoBanner.title'),
    subtitle: c.decode(_promo.p('subtitle'), asString),
    image: image == null ? null : _picture(c, image),
    cta: c.decode(_promo.p('ctaLabel'), asString),
    endsAt: _dateTime(c.prop(_promo.p('endsAt'))),
    onTap: c.handles(onTap) ? () => c.fire(onTap) : null,
  );
}

final class _Promo extends StatelessWidget {
  const _Promo({
    required this.title,
    required this.subtitle,
    required this.image,
    required this.cta,
    required this.endsAt,
    required this.onTap,
  });

  final String title;
  final String? subtitle;
  final Widget? image;
  final String? cta;
  final DateTime? endsAt;
  final VoidCallback? onTap;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final scheme = theme.colorScheme;
    final image = this.image, subtitle = this.subtitle, cta = this.cta;
    final endsAt = this.endsAt;
    // Text sits on a scrim over a picture, else on the container colour.
    final onColor = image == null ? scheme.onPrimaryContainer : Colors.white;
    return Semantics(
      container: true,
      button: onTap != null,
      onTap: onTap,
      child: Material(
        color: scheme.primaryContainer,
        borderRadius: BorderRadius.circular(16),
        clipBehavior: Clip.antiAlias,
        child: InkWell(
          excludeFromSemantics: true,
          onTap: onTap,
          child: Stack(
            children: [
              if (image != null) ...[
                Positioned.fill(child: image),
                const Positioned.fill(
                  child: ColoredBox(color: Color(0x99000000)),
                ),
              ],
              Padding(
                padding: const EdgeInsetsDirectional.all(16),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    Text(
                      title,
                      style: theme.textTheme.titleLarge?.copyWith(
                        color: onColor,
                      ),
                    ),
                    if (subtitle != null)
                      Text(
                        subtitle,
                        style: theme.textTheme.bodyMedium?.copyWith(
                          color: onColor,
                        ),
                      ),
                    if (endsAt != null) ...[
                      const SizedBox(height: 8),
                      CountdownView(
                        endsAt: endsAt,
                        style: theme.textTheme.titleMedium?.copyWith(
                          color: onColor,
                        ),
                      ),
                    ],
                    if (cta != null) ...[
                      const SizedBox(height: 12),
                      DecoratedBox(
                        decoration: BoxDecoration(
                          color: scheme.primary,
                          borderRadius: BorderRadius.circular(20),
                        ),
                        child: Padding(
                          padding: const EdgeInsetsDirectional.symmetric(
                            horizontal: 16,
                            vertical: 8,
                          ),
                          child: Text(
                            cta,
                            style: theme.textTheme.labelLarge?.copyWith(
                              color: scheme.onPrimary,
                            ),
                          ),
                        ),
                      ),
                    ],
                  ],
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

// ── RatingBar and RatingInput ──────────────────────────────────────────────

/// `RatingBar`: read-only stars, rounded to halves. Screen readers hear the
/// label and a value such as `3.5 of 5`.
Widget buildRatingBar(NodeContext c) {
  final max = (c.decode(_ratingBar.p('max'), asInt) ?? 5).clamp(1, 10);
  final value =
      (c.decode(_ratingBar.p('value'), asDouble) ??
              c.missing('RatingBar.value'))
          .clamp(0.0, max.toDouble());
  final of =
      c.decode(_ratingBar.p('ofLabel'), asString) ??
      RuntimeStrings.of(c.context)['plux.rating.of'];
  final shown = value == value.roundToDouble()
      ? '${value.round()}'
      : value.toStringAsFixed(1);
  final half = (value * 2).round() / 2;
  final scheme = Theme.of(c.context).colorScheme;
  return Semantics(
    container: true,
    label: c.decode(_ratingBar.p('semanticLabel'), asString),
    value: '$shown $of $max',
    excludeSemantics: true,
    child: Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        for (var i = 1; i <= max; i++)
          Icon(
            half >= i
                ? Icons.star
                : half >= i - 0.5
                ? Icons.star_half
                : Icons.star_border,
            color: scheme.primary,
            size: 20,
          ),
      ],
    ),
  );
}

/// `RatingInput`: stars the user taps; each star is a button labelled with
/// its number, the chosen one marked selected.
Widget buildRatingInput(NodeContext c) {
  final max = (c.decode(_ratingInput.p('max'), asInt) ?? 5).clamp(1, 10);
  final value = (c.decode(_ratingInput.p('value'), asInt) ?? 0).clamp(0, max);
  final noun = c.decode(_ratingInput.p('starLabel'), asString) ?? 'star';
  final scheme = Theme.of(c.context).colorScheme;
  return c.controlled<int>(_ratingInput.e('onChanged'), value, (v, onChanged) {
    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        for (var i = 1; i <= max; i++)
          Semantics(
            button: true,
            selected: i == v,
            enabled: onChanged != null,
            label: '$i $noun',
            onTap: onChanged == null ? null : () => onChanged(i),
            excludeSemantics: true,
            child: IconButton(
              onPressed: onChanged == null ? null : () => onChanged(i),
              icon: Icon(
                i <= v ? Icons.star : Icons.star_border,
                color: scheme.primary,
              ),
            ),
          ),
      ],
    );
  });
}

// ── ImageGallery ───────────────────────────────────────────────────────────

/// `ImageGallery`: swipeable images, each pinch-zoomable, with indicators.
Widget buildImageGallery(NodeContext c) {
  final images =
      c.decode(
        _gallery.p('images'),
        (d, v) => asList(d, v, decodeImageSource),
      ) ??
      c.missing('ImageGallery.images');
  final onPage = _gallery.e('onPageChanged');
  return _GalleryView(
    images: [for (final i in images) _picture(c, i)],
    initialPage: c.decode(_gallery.p('initialPage'), asInt) ?? 0,
    indicators: c.decode(_gallery.p('indicators'), asBool) ?? true,
    label: c.decode(_gallery.p('semanticLabel'), asString),
    onPageChanged: c.handles(onPage) ? (i) => c.fire(onPage, i) : null,
  );
}

final class _GalleryView extends StatefulWidget {
  const _GalleryView({
    required this.images,
    required this.initialPage,
    required this.indicators,
    required this.label,
    required this.onPageChanged,
  });

  final List<Widget> images;
  final int initialPage;
  final bool indicators;
  final String? label;
  final ValueChanged<int>? onPageChanged;

  @override
  State<_GalleryView> createState() => _GalleryViewState();
}

final class _GalleryViewState extends State<_GalleryView> {
  late final PageController _controller = PageController(
    initialPage: widget.images.isEmpty
        ? 0
        : widget.initialPage.clamp(0, widget.images.length - 1),
  );
  late int _page = _controller.initialPage;
  bool _zoomed = false;

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    final count = widget.images.length;
    return Semantics(
      container: true,
      label: widget.label,
      child: LayoutBuilder(
        builder: (context, box) {
          final height = box.hasBoundedHeight
              ? box.maxHeight
              : box.maxWidth.isFinite
              ? box.maxWidth * 3 / 4
              : 240.0;
          return SizedBox(
            height: height,
            child: Stack(
              children: [
                PageView.builder(
                  controller: _controller,
                  physics: _zoomed
                      ? const NeverScrollableScrollPhysics()
                      : null,
                  itemCount: count,
                  onPageChanged: (i) {
                    setState(() => _page = i);
                    widget.onPageChanged?.call(i);
                  },
                  itemBuilder: (context, i) => _Zoomable(
                    onZoom: (z) {
                      if (z != _zoomed) setState(() => _zoomed = z);
                    },
                    child: widget.images[i],
                  ),
                ),
                if (widget.indicators && count > 1)
                  PositionedDirectional(
                    start: 0,
                    end: 0,
                    bottom: 8,
                    child: ExcludeSemantics(
                      child: Row(
                        mainAxisAlignment: MainAxisAlignment.center,
                        children: [
                          for (var i = 0; i < count; i++)
                            Container(
                              margin: const EdgeInsets.symmetric(horizontal: 3),
                              width: 8,
                              height: 8,
                              decoration: BoxDecoration(
                                shape: BoxShape.circle,
                                color: i == _page
                                    ? scheme.primary
                                    : scheme.outlineVariant,
                              ),
                            ),
                        ],
                      ),
                    ),
                  ),
              ],
            ),
          );
        },
      ),
    );
  }
}

/// An image that pinch-zooms. Panning is on only while zoomed in, and
/// scaling only with two fingers, so a one-finger swipe reaches the page
/// view.
final class _Zoomable extends StatefulWidget {
  const _Zoomable({required this.child, required this.onZoom});

  final Widget child;
  final ValueChanged<bool> onZoom;

  @override
  State<_Zoomable> createState() => _ZoomableState();
}

final class _ZoomableState extends State<_Zoomable> {
  final TransformationController _transform = TransformationController();
  final Set<int> _pointers = {};
  bool _zoomed = false;

  @override
  void dispose() {
    _transform.dispose();
    super.dispose();
  }

  void _update() {
    final zoomed = _transform.value.getMaxScaleOnAxis() > 1.01;
    if (zoomed == _zoomed) return;
    _zoomed = zoomed;
    widget.onZoom(zoomed);
  }

  @override
  Widget build(BuildContext context) => Listener(
    onPointerDown: (e) => setState(() => _pointers.add(e.pointer)),
    onPointerUp: (e) => setState(() => _pointers.remove(e.pointer)),
    onPointerCancel: (e) => setState(() => _pointers.remove(e.pointer)),
    child: InteractiveViewer(
      transformationController: _transform,
      minScale: 1,
      maxScale: 4,
      panEnabled: _zoomed,
      scaleEnabled: _zoomed || _pointers.length >= 2,
      onInteractionEnd: (_) => _update(),
      child: SizedBox.expand(child: widget.child),
    ),
  );
}

// ── FileUploader ───────────────────────────────────────────────────────────

/// `FileUploader`: picks a file through the `plux_media` picker, previews
/// it and uploads it through the transfer operation `dataSource.operation`.
/// Without the picker package the control is disabled and PLX-5401 is
/// reported (RT-060).
Widget buildFileUploader(NodeContext c) {
  final source =
      c.decode(_uploader.p('dataSource'), asString) ??
      c.missing('FileUploader.dataSource');
  final operation =
      c.decode(_uploader.p('operation'), asString) ??
      c.missing('FileUploader.operation');
  final accept = c.decode(_uploader.p('accept'), asStrings) ?? const [];
  final scope = RenderScope.of(c.context);
  final step = scope.actions?.context;
  final picker = step?.service<PluxMediaPicker>();
  final device = step?.device;
  final data = step?.data;
  final onUploaded = _uploader.e('onUploaded');
  final onFailed = _uploader.e('onFailed');
  return FileUploaderView(
    label:
        c.decode(_uploader.p('label'), asString) ??
        RuntimeStrings.of(c.context)['plux.uploader.choose'],
    uploadLabel:
        c.decode(_uploader.p('uploadLabel'), asString) ??
        RuntimeStrings.of(c.context)['plux.uploader.upload'],
    maxBytes: c.decode(_uploader.p('maxBytes'), asInt),
    pick: picker == null || device == null
        ? null
        : () async {
            try {
              device.guard.admit(device.plugin, 'pickFile', {
                'mimeTypes': accept,
              });
            } on ActionError {
              // Blocked: the guard has reported it.
              return null;
            }
            final files = await picker.pickFiles(
              mimeTypes: accept,
              multiple: false,
              limit: 1,
            );
            return files.isEmpty ? null : files.first;
          },
    upload: data == null
        ? null
        : (file, progress) async {
            final param = data is DataScope
                ? data[source]?.spec.operations[operation]?.transfer?.fileParam
                : null;
            final out = await data.callOperation(
              '$source.$operation',
              {param ?? 'file': file.path},
              onProgress: (p) =>
                  progress(p.total > 0 ? p.sent / p.total : null),
            );
            return out == null ? null : jsonEncode(toJson(out));
          },
    onUnavailable: () => scope.report(
      PluxException(
        PluxErrorCode.devicePackageMissing,
        'FileUploader needs plux_media: add it to '
        'PluxConfig.devicePackages',
        details: {'plugin': scope.pluginKey},
      ),
      path: scope.path,
    ),
    onUploaded: c.handles(onUploaded) ? (r) => c.fire(onUploaded, r) : null,
    onFailed: c.handles(onFailed) ? (e) => c.fire(onFailed, e) : null,
  );
}

/// Where a [FileUploaderView] is.
enum _Phase { idle, picked, uploading, done, failed }

/// The file uploader's interface. [pick] is null where no picker is
/// installed, which disables the control after calling [onUnavailable]
/// once; [upload] reports progress as a fraction, null when unknown, and
/// returns the JSON-encoded response.
final class FileUploaderView extends StatefulWidget {
  /// Creates the view.
  const FileUploaderView({
    super.key,
    required this.label,
    required this.uploadLabel,
    required this.pick,
    required this.upload,
    this.maxBytes,
    this.onUnavailable,
    this.onUploaded,
    this.onFailed,
  });

  /// The pick button's label.
  final String label;

  /// The upload button's label.
  final String uploadLabel;

  /// The largest file in bytes, or null.
  final int? maxBytes;

  /// Lets the user pick a file; null when no picker is installed.
  final Future<PluxPickedFile?> Function()? pick;

  /// Uploads a file; null where no data layer runs.
  final Future<String?> Function(
    PluxPickedFile file,
    void Function(double? fraction) progress,
  )?
  upload;

  /// Called once when [pick] is null.
  final VoidCallback? onUnavailable;

  /// Called with `{name, mimeType, sizeBytes, response?}`.
  final ValueChanged<Map<String, Object?>>? onUploaded;

  /// Called with `{code, message}`.
  final ValueChanged<Map<String, Object?>>? onFailed;

  @override
  State<FileUploaderView> createState() => _FileUploaderViewState();
}

final class _FileUploaderViewState extends State<FileUploaderView> {
  _Phase _phase = _Phase.idle;
  PluxPickedFile? _file;
  double? _progress;
  String? _errorCode;

  @override
  void initState() {
    super.initState();
    if (widget.pick == null) {
      scheduleMicrotask(() => widget.onUnavailable?.call());
    }
  }

  void _fail(String code, String message) {
    if (!mounted) return;
    setState(() {
      _phase = _file == null ? _Phase.idle : _Phase.failed;
      _errorCode = code;
    });
    widget.onFailed?.call({'code': code, 'message': message});
  }

  (String, String) _describe(Object error) => switch (error) {
    ActionError(:final code, :final message) => (code.id, message),
    PluxDeviceException(:final failure, :final message) => (
      (failure == PluxDeviceFailure.denied
              ? PluxErrorCode.devicePermissionDenied
              : PluxErrorCode.deviceUnavailable)
          .id,
      message,
    ),
    _ => (PluxErrorCode.dataTransferFileFailed.id, 'the transfer failed'),
  };

  Future<void> _pick() async {
    final pick = widget.pick;
    if (pick == null || _phase == _Phase.uploading) return;
    try {
      final file = await pick();
      if (!mounted || file == null) return;
      final max = widget.maxBytes;
      if (max != null && file.sizeBytes > max) {
        _fail(
          PluxErrorCode.dataTransferFileFailed.id,
          'the file is larger than $max bytes',
        );
        return;
      }
      setState(() {
        _file = file;
        _phase = _Phase.picked;
        _progress = null;
        _errorCode = null;
      });
    } on Exception catch (e) {
      final (code, message) = _describe(e);
      _fail(code, message);
    }
  }

  Future<void> _upload() async {
    final file = _file, upload = widget.upload;
    if (file == null || upload == null || _phase == _Phase.uploading) return;
    setState(() {
      _phase = _Phase.uploading;
      _progress = 0;
      _errorCode = null;
    });
    try {
      final response = await upload(file, (f) {
        if (mounted) setState(() => _progress = f);
      });
      if (!mounted) return;
      setState(() => _phase = _Phase.done);
      widget.onUploaded?.call({
        'name': file.name,
        'mimeType': file.mimeType,
        'sizeBytes': file.sizeBytes,
        'response': ?response,
      });
    } on Exception catch (e) {
      final (code, message) = _describe(e);
      _fail(code, message);
    }
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final scheme = theme.colorScheme;
    final file = _file;
    final busy = _phase == _Phase.uploading;
    final pct = _progress == null ? null : (_progress! * 100).round();
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      mainAxisSize: MainAxisSize.min,
      children: [
        OutlinedButton.icon(
          onPressed: widget.pick == null || busy ? null : _pick,
          icon: const Icon(Icons.attach_file),
          label: Text(widget.label),
        ),
        if (file != null) ...[
          const SizedBox(height: 12),
          Row(
            children: [
              SizedBox(
                width: 56,
                height: 56,
                child: file.mimeType.startsWith('image/')
                    ? Image.file(
                        File(file.path),
                        fit: BoxFit.cover,
                        cacheWidth: 112,
                        excludeFromSemantics: true,
                        errorBuilder: (_, _, _) =>
                            const Icon(Icons.image_outlined),
                      )
                    : const Icon(Icons.insert_drive_file_outlined),
              ),
              const SizedBox(width: 12),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      file.name,
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: theme.textTheme.bodyLarge,
                    ),
                    Text(
                      _size(file.sizeBytes),
                      style: theme.textTheme.bodySmall?.copyWith(
                        color: scheme.onSurfaceVariant,
                      ),
                    ),
                  ],
                ),
              ),
              if (_phase == _Phase.done)
                Semantics(
                  liveRegion: true,
                  label: '${file.name}, 100%',
                  excludeSemantics: true,
                  child: Icon(Icons.check_circle, color: scheme.primary),
                ),
            ],
          ),
          if (busy) ...[
            const SizedBox(height: 12),
            Semantics(
              label: file.name,
              value: pct == null ? null : '$pct%',
              excludeSemantics: true,
              child: LinearProgressIndicator(value: _progress),
            ),
          ],
          if (_phase == _Phase.picked || _phase == _Phase.failed) ...[
            const SizedBox(height: 12),
            FilledButton(
              onPressed: widget.upload == null ? null : _upload,
              child: Text(widget.uploadLabel),
            ),
          ],
        ],
        if (_errorCode != null)
          Semantics(
            liveRegion: true,
            child: Padding(
              padding: const EdgeInsets.only(top: 8),
              child: Text(
                _errorCode!,
                style: theme.textTheme.bodySmall?.copyWith(color: scheme.error),
              ),
            ),
          ),
      ],
    );
  }

  static String _size(int bytes) => bytes >= 1 << 20
      ? '${(bytes / (1 << 20)).toStringAsFixed(1)} MB'
      : bytes >= 1 << 10
      ? '${(bytes / (1 << 10)).toStringAsFixed(1)} KB'
      : '$bytes B';
}
