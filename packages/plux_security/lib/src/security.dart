// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// `PluxSecurity`, the module the host installs (SEC-070, SEC-071,
/// ADR-0057): it scans at start and on a timer, decides the response to
/// each detection by policy, and drives the runtime's side of it.
library;

import 'dart:async';
import 'dart:developer' as developer;

import 'package:flutter/foundation.dart';
import 'package:plux_flutter/plux_flutter.dart';
import 'package:plux_security/src/detection.dart';
import 'package:plux_security/src/detector.dart';
import 'package:plux_security/src/dev_mode.dart';
import 'package:plux_security/src/response.dart';

/// What is in force now: the detections and the response to each.
@immutable
final class PluxSecurityStatus {
  /// Creates a status.
  PluxSecurityStatus(
    Map<PluxDetectionKind, PluxDetection> detections,
    Map<PluxDetectionKind, PluxRaspResponse> responses,
  ) : detections = Map.unmodifiable(detections),
      responses = Map.unmodifiable(responses);

  /// Nothing detected.
  static final PluxSecurityStatus clear = PluxSecurityStatus(
    const {},
    const {},
  );

  /// The detections in force, by kind.
  final Map<PluxDetectionKind, PluxDetection> detections;

  /// The response to each detection in force.
  final Map<PluxDetectionKind, PluxRaspResponse> responses;

  /// The strictest response in force; [PluxRaspResponse.report] when nothing
  /// is detected.
  PluxRaspResponse get response =>
      responses.values.fold(PluxRaspResponse.report, (a, b) => a.stricter(b));

  /// Whether the blocking screen must show.
  bool get blocked => response == PluxRaspResponse.block;

  /// The detections whose response asks for a message ([PluxRaspResponse.warn]
  /// or stricter).
  List<PluxDetection> get warnings => [
    for (final MapEntry(:key, :value) in detections.entries)
      if (responses[key]!.index >= PluxRaspResponse.warn.index) value,
  ];

  /// The ceiling on the assurance level this status imposes: 0 when
  /// blocked, 2 when degraded, 3 (none) otherwise.
  int get assuranceCeiling => switch (response) {
    PluxRaspResponse.block => 0,
    PluxRaspResponse.degrade => 2,
    _ => 3,
  };

  @override
  bool operator ==(Object other) =>
      other is PluxSecurityStatus &&
      mapEquals(other.responses, responses) &&
      other.detections.length == detections.length &&
      detections.entries.every(
        (e) =>
            other.detections[e.key] != null &&
            listEquals(other.detections[e.key]!.indicators, e.value.indicators),
      );

  @override
  int get hashCode => Object.hash(
    Object.hashAll(responses.entries.map((e) => Object.hash(e.key, e.value))),
    detections.length,
  );
}

/// Runtime application self-protection (SEC-070, SEC-071, ADR-0057).
///
/// Add it to `PluxConfig.securityModules`:
///
/// ```dart
/// final security = PluxSecurity(
///   expectations: PluxRaspExpectations(signingCertSha256: [releaseCertDigest]),
/// );
/// await Plux.initialize(PluxConfig(
///   appId: 'acme-mobile',
///   endpoint: Uri.parse('https://plux.acme.example'),
///   securityModules: [security],
/// ));
/// // MaterialApp(builder: (context, child) =>
/// //     PluxSecurityGate(security: security, child: child!));
/// ```
///
/// Every detection is reported to the server as the `rasp_detection`
/// telemetry event, where it lowers the device's assurance level, and a
/// detection that suggests a modified environment makes the device attest
/// again (SEC-006). The response on the device follows the security
/// settings, which [policy] can only tighten; so does the scan interval. RASP raises the cost of an attack; it does not
/// stop a determined attacker on a device they control.
final class PluxSecurity implements PluxSecurityModule {
  /// Creates the module. The first scan runs when the runtime attaches it,
  /// the next ones every [scanInterval].
  PluxSecurity({
    this._detector = const PlatformRaspDetector(),
    PluxRaspExpectations? expectations,
    this.policy = const PluxRaspPolicy(),
    this.scanInterval,
    this.mode = PluxSecurityMode.production,
    this.environment = productionEnvironment,
    @visibleForTesting bool devtools = kPluxDevtools,
  }) : expectations = expectations ?? PluxRaspExpectations(),
       developmentRefusal = refuseDevelopmentMode(
         mode,
         devtools: kPluxDevtools && devtools,
         environment: environment,
       ) {
    final warning = developmentRefusal;
    if (warning != null) {
      developer.log(warning, name: 'plux_security', level: 900);
    }
  }

  final PluxRaspDetector _detector;

  /// What the app expects of itself.
  final PluxRaspExpectations expectations;

  /// The response to each kind of detection.
  final PluxRaspPolicy policy;

  /// How often the device is scanned again, when the app wants it more
  /// often than the `raspScanInterval` setting asks; the setting applies
  /// otherwise (SEC-182).
  final Duration? scanInterval;

  /// How detections are treated; [PluxSecurityMode.developmentReportOnly]
  /// only takes effect when [developmentMode] is true.
  final PluxSecurityMode mode;

  /// The environment key of the runtime's `PluxConfig.environment`, which
  /// the development mode checks; `production` unless the host says
  /// otherwise.
  final String environment;

  /// Why the requested [mode] was refused, or null when it runs. A refused
  /// development mode is logged as a warning and the module runs as
  /// [PluxSecurityMode.production].
  final String? developmentRefusal;

  /// Whether the report-only development mode runs (DEV-001): requested,
  /// in a build with `kPluxDevtools` and in a non-production environment.
  bool get developmentMode =>
      kPluxDevtools &&
      mode == PluxSecurityMode.developmentReportOnly &&
      developmentRefusal == null;

  /// Where the development mode reports detections as `securityDetection`
  /// dev events; the Dev app sets it once its dev session is up.
  DevEventSink? devEventSink;

  final ValueNotifier<PluxSecurityStatus> _status = ValueNotifier(
    PluxSecurityStatus.clear,
  );

  /// What is in force now; changes when a scan finds or clears something
  /// and when the security settings change.
  ValueListenable<PluxSecurityStatus> get status => _status;

  PluxSecurityHost? _host;
  Timer? _timer;
  Duration? _every;
  Future<void>? _scanning;
  var _ceiling = 3;
  final Map<PluxDetectionKind, PluxDetection> _active = {};

  @override
  String get name => 'plux_security';

  @override
  void attach(PluxSecurityHost host) {
    detach();
    _host = host;
    host.settings.addListener(_settingsChanged);
    unawaited(scanNow());
    _schedule();
  }

  /// The interval in force: the setting's, or the app's when it is shorter.
  Duration _interval(SecuritySettings settings) {
    final fromSettings = Duration(
      seconds: settings.number(SecuritySetting.raspScanInterval),
    );
    final own = scanInterval;
    return own != null && own < fromSettings ? own : fromSettings;
  }

  void _schedule() {
    final host = _host;
    if (host == null) return;
    final every = _interval(host.settings.value);
    if (every == _every && _timer != null) return;
    _timer?.cancel();
    _every = every;
    _timer = Timer.periodic(every, (_) => unawaited(scanNow()));
  }

  @override
  void detach() {
    _timer?.cancel();
    _timer = null;
    _every = null;
    _host?.settings.removeListener(_settingsChanged);
    _host = null;
  }

  /// Scans now, outside the timer, such as before a sensitive step. A scan
  /// already running is joined. Completes when the scan's findings are
  /// applied; a detector that fails leaves the previous findings in force.
  Future<void> scanNow() => _scanning ??= _scan().whenComplete(() {
    _scanning = null;
  });

  Future<void> _scan() async {
    final List<PluxDetection> found;
    try {
      found = await _detector.scan(expectations);
    } on Object catch (e) {
      // A failing check is not a finding; it is logged, never thrown into
      // the host app.
      developer.log('scan failed: ${e.runtimeType}', name: 'plux_security');
      return;
    }
    if (_host == null) return;
    final fresh = <PluxDetectionKind>[];
    final next = <PluxDetectionKind, PluxDetection>{
      for (final d in _active.values)
        if (d.kind.sticky) d.kind: d,
    };
    for (final d in found) {
      if (!_active.containsKey(d.kind)) fresh.add(d.kind);
      next[d.kind] = d;
    }
    _active
      ..clear()
      ..addAll(next);
    _publish(fresh);
  }

  void _settingsChanged() {
    _schedule();
    _publish(const []);
  }

  /// Recomputes the status, reports the [fresh] detections and applies the
  /// assurance ceiling.
  void _publish(List<PluxDetectionKind> fresh, {bool simulated = false}) {
    final host = _host;
    if (host == null) return;
    if (developmentMode) {
      _publishReportOnly(fresh, simulated);
      return;
    }
    final settings = host.settings.value;
    final responses = {
      for (final k in _active.keys) k: policy.responseFor(k, settings),
    };
    final status = PluxSecurityStatus(_active, responses);
    for (final kind in fresh) {
      final d = _active[kind]!;
      host.reportDetection(
        kind: kind.wireName,
        severity: kind.severity.name,
        response: responses[kind]!.name,
        indicators: d.indicators,
      );
    }
    if (status.assuranceCeiling != _ceiling) {
      _ceiling = status.assuranceCeiling;
      host.limitAssurance(_ceiling);
    }
    // A modified environment makes the device attest again, after the
    // reports above are on their way (SEC-006). Overlays and screen capture
    // say nothing the platform's attestation could confirm.
    if (fresh.any((k) => k.sticky)) unawaited(host.requestReattestation());
    if (status != _status.value) _status.value = status;
  }

  /// The development mode's side of [_publish] (DEV-001, ADR-0057 Revision
  /// 2026-10-22): the detections are in the status and in the dev stream,
  /// and every response is [PluxRaspResponse.report]. Nothing is reported
  /// to the server, no ceiling is set and the device does not attest again,
  /// so the assurance level the server sees is unchanged.
  void _publishReportOnly(List<PluxDetectionKind> fresh, bool simulated) {
    final sink = devEventSink;
    for (final kind in fresh) {
      sink?.emit({
        'securityDetection': {
          'kind': devKindName(kind),
          if (simulated) 'simulated': true,
          'response': PluxRaspResponse.report.name,
        },
      });
    }
    final status = PluxSecurityStatus(_active, {
      for (final k in _active.keys) k: PluxRaspResponse.report,
    });
    if (status != _status.value) _status.value = status;
  }

  /// Injects a detection of [kind] through the pipeline the real detectors
  /// use, as the `simulateSecurityDetection` dev command does (DEV-001). It
  /// is reported to [devEventSink] as simulated and follows the kind's
  /// stickiness like a real finding. Returns false, and does nothing, when
  /// the development mode does not run or the module is not attached.
  bool simulateDetection(PluxDetectionKind kind) {
    if (!developmentMode || _host == null) return false;
    _active[kind] = PluxDetection(kind, const ['simulated']);
    _publish([kind], simulated: true);
    return true;
  }
}
