// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The Layer 2 components of P5, group A (WGT-020, Appendix C.9, ADR-0071):
/// `ConsentCheckbox`, `SearchField`, `FilterBar`, `SortSheet`,
/// `InfiniteList`, `WizardForm`, `OnboardingPager`, `SettingsList`,
/// `ProfileHeader` and `StatusTimeline`. They are built from Material
/// widgets, the page's theme and the page's form; every one follows the
/// text direction and the text scale, and names its role to screen readers
/// (A11Y-001). Their IDs come from the generated registry (BND-011).
library;

import 'dart:async';

import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:plux_flutter/src/forms/form_state.dart';
import 'package:plux_flutter/src/l10n/runtime_strings.dart';
import 'package:plux_flutter/src/render/builders/builders.dart';
import 'package:plux_flutter/src/render/decoders.dart';
import 'package:plux_flutter/src/render/decoding.dart';
import 'package:plux_flutter/src/render/generated/render.g.dart';
import 'package:plux_flutter/src/render/node_context.dart';
import 'package:plux_flutter/src/render/scope.dart';
import 'package:plux_flutter/src/schema/registry.g.dart';

/// The IDs of one widget of the registry.
final class _Ids {
  _Ids(String type) : _d = widgetDescriptors.firstWhere((d) => d.type == type);

  final WidgetDescriptor _d;

  int get id => _d.id;
  int p(String name) => _d.props[name]!;
  int e(String name) => _d.events[name]!;
  int s(String name) => _d.slots[name]!;
}

final _consent = _Ids('ConsentCheckbox');
final _search = _Ids('SearchField');
final _filter = _Ids('FilterBar');
final _sort = _Ids('SortSheet');
final _infinite = _Ids('InfiniteList');
final _wizard = _Ids('WizardForm');
final _pager = _Ids('OnboardingPager');
final _settings = _Ids('SettingsList');
final _profile = _Ids('ProfileHeader');
final _timeline = _Ids('StatusTimeline');

/// The builders of group A, by permanent widget ID.
final Map<int, NodeBuilder> componentsABuilders = {
  _consent.id: buildConsentCheckbox,
  _search.id: buildSearchField,
  _filter.id: buildFilterBar,
  _sort.id: buildSortSheet,
  _infinite.id: buildInfiniteList,
  _wizard.id: buildWizardForm,
  _pager.id: buildOnboardingPager,
  _settings.id: buildSettingsList,
  _profile.id: buildProfileHeader,
  _timeline.id: buildStatusTimeline,
};

// ── Shared pieces ───────────────────────────────────────────────────────────

bool _same(Object? a, Object? b) =>
    a is List && b is List ? listEquals(a, b) : a == b;

/// A value the component keeps locally: it starts at [initial] and follows
/// it when the page changes it.
final class _Value<T> extends StatefulWidget {
  const _Value({required this.initial, required this.build});

  final T initial;
  final Widget Function(T value, ValueChanged<T> set) build;

  @override
  State<_Value<T>> createState() => _ValueState<T>();
}

final class _ValueState<T> extends State<_Value<T>> {
  late T _value = widget.initial;

  @override
  void didUpdateWidget(_Value<T> old) {
    super.didUpdateWidget(old);
    if (!_same(old.initial, widget.initial)) _value = widget.initial;
  }

  @override
  Widget build(BuildContext context) =>
      widget.build(_value, (v) => setState(() => _value = v));
}

/// A choice of a filter bar or a sort sheet (`ChoiceItem`).
@immutable
final class _Choice {
  const _Choice(this.id, this.label, this.selected);

  final String id;
  final String label;
  final bool selected;

  @override
  bool operator ==(Object other) =>
      other is _Choice &&
      other.id == id &&
      other.label == label &&
      other.selected == selected;

  @override
  int get hashCode => Object.hash(id, label, selected);
}

_Choice? _decodeChoice(Decoding d, Object? v) {
  final f = Fields.of(d, v);
  if (f == null) return null;
  final id = asString(d, f.get(1, 'id'));
  final label = asString(d, f.get(2, 'label'));
  if (id == null || label == null) return null;
  return _Choice(id, label, asBool(d, f.get(3, 'selected')) ?? false);
}

Duration _animation(BuildContext context) =>
    MediaQuery.disableAnimationsOf(context)
    ? Duration.zero
    : const Duration(milliseconds: 200);

// ── ConsentCheckbox ─────────────────────────────────────────────────────────

/// `ConsentCheckbox`: a checkbox with the consent text and, when
/// `linkLabel` is set, a link that fires `onLinkTap`. Checking it fires
/// `onChanged(true)` and then `onAccepted(documentVersion)`.
Widget buildConsentCheckbox(NodeContext c) {
  final label =
      c.decode(_consent.p('label'), asString) ??
      c.missing('ConsentCheckbox.label');
  final version =
      c.decode(_consent.p('documentVersion'), asString) ??
      c.missing('ConsentCheckbox.documentVersion');
  final linkLabel = c.decode(_consent.p('linkLabel'), asString);
  return _Value<bool>(
    initial: c.decode(_consent.p('value'), asBool) ?? false,
    build: (checked, set) {
      void toggle() {
        final next = !checked;
        set(next);
        c.fire(_consent.e('onChanged'), next);
        if (next) c.fire(_consent.e('onAccepted'), version);
      }

      return Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        mainAxisSize: MainAxisSize.min,
        children: [
          Semantics(
            container: true,
            checked: checked,
            label: label,
            onTap: toggle,
            excludeSemantics: true,
            child: InkWell(
              onTap: toggle,
              child: ConstrainedBox(
                constraints: const BoxConstraints(minHeight: 48),
                child: Row(
                  crossAxisAlignment: CrossAxisAlignment.center,
                  children: [
                    Checkbox(value: checked, onChanged: (_) => toggle()),
                    Expanded(child: Text(label)),
                  ],
                ),
              ),
            ),
          ),
          if (linkLabel != null)
            Padding(
              padding: const EdgeInsetsDirectional.only(start: 48),
              child: Semantics(
                link: true,
                child: TextButton(
                  onPressed: c.handles(_consent.e('onLinkTap'))
                      ? () => c.fire(_consent.e('onLinkTap'))
                      : null,
                  child: Text(linkLabel),
                ),
              ),
            ),
        ],
      );
    },
  );
}

// ── SearchField ─────────────────────────────────────────────────────────────

/// `SearchField`: a text field that fires `onSearch` once typing has paused
/// for `debounceMs`, with `suggestions` below it while it has text.
Widget buildSearchField(NodeContext c) {
  final onSearch = _search.e('onSearch');
  final onPick = _search.e('onSuggestionSelected');
  return _SearchField(
    value: c.decode(_search.p('value'), asString) ?? '',
    hint: c.decode(_search.p('hint'), asString),
    debounce: Duration(
      milliseconds: (c.decode(_search.p('debounceMs'), asInt) ?? 300).clamp(
        0,
        5000,
      ),
    ),
    suggestions: c.decode(_search.p('suggestions'), asStrings) ?? const [],
    onSearch: (q) => c.fire(onSearch, q),
    onPick: (s) => c.fire(onPick, s),
  );
}

final class _SearchField extends StatefulWidget {
  const _SearchField({
    required this.value,
    required this.hint,
    required this.debounce,
    required this.suggestions,
    required this.onSearch,
    required this.onPick,
  });

  final String value;
  final String? hint;
  final Duration debounce;
  final List<String> suggestions;
  final ValueChanged<String> onSearch;
  final ValueChanged<String> onPick;

  @override
  State<_SearchField> createState() => _SearchFieldState();
}

final class _SearchFieldState extends State<_SearchField> {
  late final TextEditingController _controller = TextEditingController(
    text: widget.value,
  )..addListener(_changed);
  Timer? _timer;
  String _last = '';
  bool _picked = false;

  @override
  void initState() {
    super.initState();
    _last = widget.value;
  }

  @override
  void didUpdateWidget(_SearchField old) {
    super.didUpdateWidget(old);
    if (old.value != widget.value && _controller.text != widget.value) {
      _last = widget.value;
      _controller.text = widget.value;
    }
  }

  void _changed() {
    final text = _controller.text;
    if (text == _last) return;
    _last = text;
    _picked = false;
    _timer?.cancel();
    _timer = Timer(widget.debounce, () => widget.onSearch(text));
    setState(() {});
  }

  void _pick(String s) {
    _timer?.cancel();
    _last = s;
    _picked = true;
    _controller.value = TextEditingValue(
      text: s,
      selection: TextSelection.collapsed(offset: s.length),
    );
    widget.onPick(s);
    setState(() {});
  }

  @override
  void dispose() {
    _timer?.cancel();
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final label =
        widget.hint ?? MaterialLocalizations.of(context).searchFieldLabel;
    final showSuggestions =
        !_picked &&
        _controller.text.isNotEmpty &&
        widget.suggestions.isNotEmpty;
    return Column(
      mainAxisSize: MainAxisSize.min,
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Semantics(
          textField: true,
          label: label,
          child: TextField(
            controller: _controller,
            textInputAction: TextInputAction.search,
            onSubmitted: (q) {
              _timer?.cancel();
              widget.onSearch(q);
            },
            decoration: InputDecoration(
              hintText: label,
              prefixIcon: const ExcludeSemantics(child: Icon(Icons.search)),
              border: const OutlineInputBorder(),
            ),
          ),
        ),
        if (showSuggestions)
          Semantics(
            container: true,
            child: Column(
              mainAxisSize: MainAxisSize.min,
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                for (final s in widget.suggestions)
                  Semantics(
                    button: true,
                    label: s,
                    excludeSemantics: true,
                    onTap: () => _pick(s),
                    child: InkWell(
                      onTap: () => _pick(s),
                      child: ConstrainedBox(
                        constraints: const BoxConstraints(minHeight: 48),
                        child: Align(
                          alignment: AlignmentDirectional.centerStart,
                          child: Padding(
                            padding: const EdgeInsetsDirectional.symmetric(
                              horizontal: 16,
                              vertical: 8,
                            ),
                            child: Text(s),
                          ),
                        ),
                      ),
                    ),
                  ),
              ],
            ),
          ),
      ],
    );
  }
}

// ── FilterBar ───────────────────────────────────────────────────────────────

/// `FilterBar`: filter chips in a horizontally scrolling row. Tapping one
/// fires `onToggle` with the chip's `id`, `label` and new `selected`.
Widget buildFilterBar(NodeContext c) {
  final chips =
      c.decode(_filter.p('filters'), (d, v) => asList(d, v, _decodeChoice)) ??
      c.missing('FilterBar.filters');
  return _Value<List<_Choice>>(
    initial: chips,
    build: (current, set) => SingleChildScrollView(
      scrollDirection: Axis.horizontal,
      padding: const EdgeInsetsDirectional.symmetric(horizontal: 16),
      child: Row(
        children: [
          for (final (i, chip) in current.indexed)
            Padding(
              padding: const EdgeInsetsDirectional.only(end: 8),
              child: FilterChip(
                label: Text(chip.label),
                selected: chip.selected,
                onSelected: (on) {
                  set([
                    for (final (j, x) in current.indexed)
                      if (j == i) _Choice(x.id, x.label, on) else x,
                  ]);
                  c.fire(_filter.e('onToggle'), {
                    'id': chip.id,
                    'label': chip.label,
                    'selected': on,
                  });
                },
              ),
            ),
        ],
      ),
    ),
  );
}

// ── SortSheet ───────────────────────────────────────────────────────────────

/// `SortSheet`: radio rows for the sort orders; choosing one fires
/// `onSelected` with its `id`.
Widget buildSortSheet(NodeContext c) {
  final options =
      c.decode(_sort.p('options'), (d, v) => asList(d, v, _decodeChoice)) ??
      c.missing('SortSheet.options');
  final title = c.decode(_sort.p('title'), asString);
  return _Value<String?>(
    initial: c.decode(_sort.p('selected'), asString),
    build: (selected, set) => Builder(
      builder: (context) {
        final theme = Theme.of(context);
        final scheme = theme.colorScheme;
        return Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            if (title != null)
              Semantics(
                header: true,
                child: Padding(
                  padding: const EdgeInsetsDirectional.fromSTEB(16, 16, 16, 8),
                  child: Text(title, style: theme.textTheme.titleMedium),
                ),
              ),
            for (final o in options)
              Semantics(
                inMutuallyExclusiveGroup: true,
                checked: o.id == selected,
                label: o.label,
                onTap: () {
                  set(o.id);
                  c.fire(_sort.e('onSelected'), o.id);
                },
                excludeSemantics: true,
                child: InkWell(
                  onTap: () {
                    set(o.id);
                    c.fire(_sort.e('onSelected'), o.id);
                  },
                  child: ConstrainedBox(
                    constraints: const BoxConstraints(minHeight: 48),
                    child: Padding(
                      padding: const EdgeInsetsDirectional.symmetric(
                        horizontal: 16,
                        vertical: 8,
                      ),
                      child: Row(
                        children: [
                          Icon(
                            o.id == selected
                                ? Icons.radio_button_checked
                                : Icons.radio_button_unchecked,
                            color: o.id == selected
                                ? scheme.primary
                                : scheme.onSurfaceVariant,
                          ),
                          const SizedBox(width: 16),
                          Expanded(child: Text(o.label)),
                        ],
                      ),
                    ),
                  ),
                ),
              ),
          ],
        );
      },
    ),
  );
}

// ── InfiniteList ────────────────────────────────────────────────────────────

/// `InfiniteList`: the items of a data source, lazily, with the `empty`,
/// `loading` and `error` slots. While more items load or fail to load, the
/// items stay and the `loading` or `error` slot follows them; the last
/// item fires `onEndReached` once while `hasMore` (WGT-012).
Widget buildInfiniteList(NodeContext c) {
  final l = ItemCollection(
    c,
    items: _infinite.p('items'),
    status: _infinite.p('status'),
    hasMore: _infinite.p('hasMore'),
    onEndReached: _infinite.e('onEndReached'),
    itemSlot: _infinite.s('item'),
    empty: _infinite.s('empty'),
    loading: _infinite.s('loading'),
    error: _infinite.s('error'),
  );
  final status = nameListStatus(c, c.prop(_infinite.p('status'))) ?? 'ready';
  if (l.values.isEmpty) {
    return l.replacement ?? const SizedBox.shrink();
  }
  final footer = switch (status) {
    'loading' => c.slot(_infinite.s('loading')),
    'error' => c.slot(_infinite.s('error')),
    _ => null,
  };
  return ListView.builder(
    padding: c.decode(_infinite.p('padding'), decodeEdgeInsets),
    itemCount: l.values.length + (footer == null ? 0 : 1),
    itemBuilder: (context, i) => i < l.values.length
        ? l.item(context, i)
        : Semantics(liveRegion: true, child: footer),
  );
}

// ── WizardForm ──────────────────────────────────────────────────────────────

/// `WizardForm`: one child per step with a progress indicator and
/// Back, Next and Submit buttons. Next and Submit validate the fields the
/// step holds (`stepFields`) through the enclosing `FormScope`.
Widget buildWizardForm(NodeContext c) {
  final titles = c.decode(_wizard.p('titles'), asStrings) ?? const <String>[];
  final steps = c.children();
  if (steps.isEmpty) return const SizedBox.shrink();
  return _Value<int>(
    initial: (c.decode(_wizard.p('currentStep'), asInt) ?? 0).clamp(
      0,
      steps.length - 1,
    ),
    build: (step, set) => _Wizard(
      c: c,
      step: step,
      steps: steps,
      titles: titles,
      fields: c.decode(_wizard.p('stepFields'), asStrings) ?? const [],
      back:
          c.decode(_wizard.p('backLabel'), asString) ??
          RuntimeStrings.of(c.context)['plux.wizard.back'],
      next:
          c.decode(_wizard.p('nextLabel'), asString) ??
          RuntimeStrings.of(c.context)['plux.wizard.next'],
      submit:
          c.decode(_wizard.p('submitLabel'), asString) ??
          RuntimeStrings.of(c.context)['plux.wizard.submit'],
      setStep: set,
    ),
  );
}

final class _Wizard extends ConsumerStatefulWidget {
  const _Wizard({
    required this.c,
    required this.step,
    required this.steps,
    required this.titles,
    required this.fields,
    required this.back,
    required this.next,
    required this.submit,
    required this.setStep,
  });

  final NodeContext c;
  final int step;
  final List<Widget> steps;
  final List<String> titles;
  final List<String> fields;
  final String back, next, submit;
  final ValueChanged<int> setStep;

  @override
  ConsumerState<_Wizard> createState() => _WizardState();
}

final class _WizardState extends ConsumerState<_Wizard> {
  bool _busy = false;

  /// The form below the nearest `FormScope`, or null outside one.
  FormController? _form() {
    final scope = RenderScope.of(context);
    final alias = scope.formAlias;
    if (alias == null) return null;
    final parts = alias.split('.');
    final instance = parts.first == 'component'
        ? scope.componentState
        : scope.state;
    if (instance == null) return null;
    return ref.read(pageStateProvider(instance).notifier).form(parts.last);
  }

  /// Whether the fields of the shown step pass their validators. Other
  /// steps' fields are not shown as touched.
  Future<bool> _valid() async {
    final form = _form();
    if (form == null) return true;
    final mine = <String>{
      if (widget.step < widget.fields.length)
        for (final f in widget.fields[widget.step].split(','))
          if (f.trim().isNotEmpty) f.trim(),
    };
    if (mine.isEmpty) return true;
    final touchedBefore = form.host.read()?['touched'];
    await form.validate();
    final state = form.host.read();
    for (final f in form.decl.fields) {
      if (mine.contains(f.name)) continue;
      final was = touchedBefore is Map ? touchedBefore[f.name] == true : false;
      form.writeTouched(f.name, touched: was);
    }
    final errors = state?['errors'];
    return errors is! Map || mine.every((f) => errors[f] == null);
  }

  Future<void> _go(int delta) async {
    if (_busy) return;
    setState(() => _busy = true);
    try {
      final last = widget.step == widget.steps.length - 1;
      if (delta > 0 && !await _valid()) return;
      if (!mounted) return;
      if (delta > 0 && last) {
        widget.c.fire(_wizard.e('onSubmit'));
      } else {
        final to = (widget.step + delta).clamp(0, widget.steps.length - 1);
        widget.setStep(to);
        widget.c.fire(_wizard.e(delta > 0 ? 'onNext' : 'onBack'), to);
      }
    } on Object {
      // A form that cannot be read leaves the step where it is.
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final n = widget.steps.length;
    final step = widget.step;
    final title = step < widget.titles.length ? widget.titles[step] : null;
    final theme = Theme.of(context);
    final last = step == n - 1;
    return Column(
      mainAxisSize: MainAxisSize.min,
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Semantics(
          container: true,
          liveRegion: true,
          label: '${title ?? ''} ${step + 1} / $n'.trim(),
          excludeSemantics: true,
          child: Padding(
            padding: const EdgeInsetsDirectional.fromSTEB(16, 8, 16, 8),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                LinearProgressIndicator(value: (step + 1) / n),
                const SizedBox(height: 8),
                Text(
                  '${step + 1} / $n${title == null ? '' : '  $title'}',
                  style: theme.textTheme.titleMedium,
                ),
              ],
            ),
          ),
        ),
        KeyedSubtree(key: ValueKey(step), child: widget.steps[step]),
        Padding(
          padding: const EdgeInsetsDirectional.all(16),
          child: Row(
            children: [
              if (step > 0)
                OutlinedButton(
                  onPressed: _busy ? null : () => _go(-1),
                  child: Text(widget.back),
                ),
              const Spacer(),
              FilledButton(
                onPressed: _busy ? null : () => _go(1),
                child: Text(last ? widget.submit : widget.next),
              ),
            ],
          ),
        ),
      ],
    );
  }
}

// ── OnboardingPager ─────────────────────────────────────────────────────────

/// `OnboardingPager`: swipeable pages with indicators; the button advances
/// and, on the last page, fires `onDone`.
Widget buildOnboardingPager(NodeContext c) {
  final pages = c.children();
  if (pages.isEmpty) return const SizedBox.shrink();
  return _Pager(
    pages: pages,
    page: (c.decode(_pager.p('currentPage'), asInt) ?? 0).clamp(
      0,
      pages.length - 1,
    ),
    next:
        c.decode(_pager.p('nextLabel'), asString) ??
        RuntimeStrings.of(c.context)['plux.wizard.next'],
    done:
        c.decode(_pager.p('doneLabel'), asString) ??
        RuntimeStrings.of(c.context)['plux.pager.done'],
    onChanged: (i) => c.fire(_pager.e('onPageChanged'), i),
    onDone: () => c.fire(_pager.e('onDone')),
  );
}

final class _Pager extends StatefulWidget {
  const _Pager({
    required this.pages,
    required this.page,
    required this.next,
    required this.done,
    required this.onChanged,
    required this.onDone,
  });

  final List<Widget> pages;
  final int page;
  final String next, done;
  final ValueChanged<int> onChanged;
  final VoidCallback onDone;

  @override
  State<_Pager> createState() => _PagerState();
}

final class _PagerState extends State<_Pager> {
  late final PageController _controller = PageController(
    initialPage: widget.page,
  );
  late int _page = widget.page;

  @override
  void didUpdateWidget(_Pager old) {
    super.didUpdateWidget(old);
    if (old.page != widget.page && widget.page != _page) _show(widget.page);
  }

  void _show(int i) {
    _page = i;
    if (!_controller.hasClients) return;
    final d = _animation(context);
    if (d == Duration.zero) {
      _controller.jumpToPage(i);
    } else {
      unawaited(
        _controller.animateToPage(i, duration: d * 2, curve: Curves.ease),
      );
    }
  }

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final n = widget.pages.length;
    final scheme = Theme.of(context).colorScheme;
    final last = _page == n - 1;
    return LayoutBuilder(
      builder: (context, box) => Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          SizedBox(
            height: box.hasBoundedHeight ? box.maxHeight - 120 : 360,
            child: PageView(
              controller: _controller,
              onPageChanged: (i) {
                setState(() => _page = i);
                widget.onChanged(i);
              },
              children: widget.pages,
            ),
          ),
          Semantics(
            liveRegion: true,
            label: '${_page + 1} / $n',
            excludeSemantics: true,
            child: Row(
              mainAxisAlignment: MainAxisAlignment.center,
              children: [
                for (var i = 0; i < n; i++)
                  AnimatedContainer(
                    duration: _animation(context),
                    margin: const EdgeInsets.all(4),
                    width: i == _page ? 24 : 8,
                    height: 8,
                    decoration: BoxDecoration(
                      color: i == _page
                          ? scheme.primary
                          : scheme.outlineVariant,
                      borderRadius: BorderRadius.circular(4),
                    ),
                  ),
              ],
            ),
          ),
          Padding(
            padding: const EdgeInsetsDirectional.all(16),
            child: FilledButton(
              onPressed: () {
                if (last) {
                  widget.onDone();
                } else {
                  _show(_page + 1);
                  widget.onChanged(_page);
                }
              },
              child: Text(last ? widget.done : widget.next),
            ),
          ),
        ],
      ),
    );
  }
}

// ── SettingsList ────────────────────────────────────────────────────────────

/// `SettingsList`: the children grouped under section headers; each section
/// takes `count` children in order, and the rest follow without a header.
Widget buildSettingsList(NodeContext c) =>
    Builder(builder: (context) => _buildSettingsList(c, context));

Widget _buildSettingsList(NodeContext c, BuildContext context) {
  final sections =
      c.decode(
        _settings.p('sections'),
        (d, v) => asList(d, v, _decodeSection),
      ) ??
      c.missing('SettingsList.sections');
  final rows = c.children();
  final theme = Theme.of(context);
  var next = 0;
  final out = <Widget>[];
  for (final (title, count) in sections) {
    out.add(
      Semantics(
        header: true,
        child: Padding(
          padding: const EdgeInsetsDirectional.fromSTEB(16, 16, 16, 8),
          child: Text(
            title,
            style: theme.textTheme.titleSmall?.copyWith(
              color: theme.colorScheme.primary,
            ),
          ),
        ),
      ),
    );
    final end = (next + count).clamp(next, rows.length);
    out.addAll(rows.sublist(next, end));
    next = end;
  }
  out.addAll(rows.sublist(next));
  return Column(
    mainAxisSize: MainAxisSize.min,
    crossAxisAlignment: CrossAxisAlignment.stretch,
    children: out,
  );
}

(String, int)? _decodeSection(Decoding d, Object? v) {
  final f = Fields.of(d, v);
  if (f == null) return null;
  final title = asString(d, f.get(1, 'title'));
  final count = asInt(d, f.get(2, 'count'));
  return title == null || count == null ? null : (title, count < 0 ? 0 : count);
}

// ── ProfileHeader ───────────────────────────────────────────────────────────

/// `ProfileHeader`: avatar (the name's initial without an image), name,
/// subtitle and an actions row.
Widget buildProfileHeader(NodeContext c) =>
    Builder(builder: (context) => _buildProfileHeader(c, context));

Widget _buildProfileHeader(NodeContext c, BuildContext context) {
  final name =
      c.decode(_profile.p('name'), asString) ?? c.missing('ProfileHeader.name');
  final subtitle = c.decode(_profile.p('subtitle'), asString);
  final image = c.decode(_profile.p('avatar'), decodeImageSource);
  final radius = c.decode(_profile.p('radius'), asDouble) ?? 32;
  final actions = c.slot(_profile.s('actions'));
  final theme = Theme.of(context);
  return Padding(
    padding: const EdgeInsets.all(16),
    child: Column(
      mainAxisSize: MainAxisSize.min,
      children: [
        ExcludeSemantics(
          child: CircleAvatar(
            radius: radius,
            backgroundImage: image,
            child: image == null && name.isNotEmpty
                ? Text(name.characters.first.toUpperCase())
                : null,
          ),
        ),
        const SizedBox(height: 12),
        Semantics(
          header: true,
          container: true,
          child: Text(
            name,
            style: theme.textTheme.headlineSmall,
            textAlign: TextAlign.center,
          ),
        ),
        if (subtitle != null)
          Text(
            subtitle,
            style: theme.textTheme.bodyMedium?.copyWith(
              color: theme.colorScheme.onSurfaceVariant,
            ),
            textAlign: TextAlign.center,
          ),
        if (actions != null) ...[
          const SizedBox(height: 16),
          Align(alignment: AlignmentDirectional.center, child: actions),
        ],
      ],
    ),
  );
}

// ── StatusTimeline ──────────────────────────────────────────────────────────

({String label, String? detail, String state})? _decodeStep(
  Decoding d,
  Object? v,
) {
  final f = Fields.of(d, v);
  final label = f == null ? null : asString(d, f.get(1, 'label'));
  if (f == null || label == null) return null;
  return (
    label: label,
    detail: asString(d, f.get(2, 'detail')),
    state: nameTimelineStepState(d, f.get(3, 'state')) ?? 'pending',
  );
}

/// `StatusTimeline`: the steps of a process from top to bottom; the current
/// step is announced when it changes.
Widget buildStatusTimeline(NodeContext c) =>
    Builder(builder: (context) => _buildStatusTimeline(c, context));

Widget _buildStatusTimeline(NodeContext c, BuildContext context) {
  final steps =
      c.decode(_timeline.p('steps'), (d, v) => asList(d, v, _decodeStep)) ??
      c.missing('StatusTimeline.steps');
  final theme = Theme.of(context);
  final scheme = theme.colorScheme;
  Widget marker(String state) {
    final (Color fill, Widget? icon) = switch (state) {
      'done' => (
        scheme.primary,
        Icon(Icons.check, size: 16, color: scheme.onPrimary),
      ),
      'failed' => (
        scheme.error,
        Icon(Icons.close, size: 16, color: scheme.onError),
      ),
      'current' => (
        scheme.primary,
        Icon(Icons.circle, size: 8, color: scheme.onPrimary),
      ),
      _ => (scheme.outlineVariant, null),
    };
    return ExcludeSemantics(
      child: Container(
        width: 24,
        height: 24,
        decoration: BoxDecoration(color: fill, shape: BoxShape.circle),
        child: icon,
      ),
    );
  }

  return Column(
    mainAxisSize: MainAxisSize.min,
    crossAxisAlignment: CrossAxisAlignment.stretch,
    children: [
      for (final (i, s) in steps.indexed)
        Semantics(
          container: true,
          liveRegion: s.state == 'current',
          label: [
            s.label,
            ?s.detail,
            RuntimeStrings.of(context)['plux.timeline.${s.state}'],
          ].join(', '),
          excludeSemantics: true,
          child: IntrinsicHeight(
            child: Row(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                SizedBox(
                  width: 40,
                  child: Column(
                    children: [
                      marker(s.state),
                      if (i < steps.length - 1)
                        Expanded(
                          child: Container(
                            width: 2,
                            color: s.state == 'done'
                                ? scheme.primary
                                : scheme.outlineVariant,
                          ),
                        ),
                    ],
                  ),
                ),
                Expanded(
                  child: Padding(
                    padding: const EdgeInsetsDirectional.only(bottom: 16),
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text(
                          s.label,
                          style: theme.textTheme.titleSmall?.copyWith(
                            fontWeight: s.state == 'current'
                                ? FontWeight.bold
                                : null,
                            color: s.state == 'failed' ? scheme.error : null,
                          ),
                        ),
                        if (s.detail != null)
                          Text(
                            s.detail!,
                            style: theme.textTheme.bodySmall?.copyWith(
                              color: scheme.onSurfaceVariant,
                            ),
                          ),
                      ],
                    ),
                  ),
                ),
              ],
            ),
          ),
        ),
    ],
  );
}
