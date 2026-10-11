// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// What the secure UI shares across a runtime (ADR-0059): the platform
/// calls, the screen protection that secure pages hold while they show,
/// and the inactivity lock; and the widgets that put them around a page.
library;

import 'dart:async';

import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';
import 'package:plux_flutter/src/actions/clock.dart';
import 'package:plux_flutter/src/device/guard.dart';
import 'package:plux_flutter/src/errors/plux_exception.dart';
import 'package:plux_flutter/src/secure/inactivity_lock.dart';
import 'package:plux_flutter/src/secure/platform.dart';
import 'package:plux_flutter/src/secure/secure_screen.dart';
import 'package:plux_flutter/src/security/security_config.dart';

/// The platform calls the secure UI makes; tests substitute fakes.
final class SecurePlatforms {
  /// Creates the platforms; each defaults to the runtime's platform channel.
  const SecurePlatforms({
    this.screen = const ChannelSecureScreenPlatform(),
    this.biometrics = const ChannelBiometricPlatform(),
  });

  /// Protects the screen while secure pages show.
  final SecureScreenPlatform screen;

  /// Asks for a biometric or the device credential.
  final BiometricPlatform biometrics;
}

/// The secure UI of one runtime: the screen protection, the inactivity
/// lock and the biometric prompt that secure pages and widgets share.
final class SecureUi {
  /// Creates the secure UI over the security [settings] (SEC-182).
  /// [report] receives platform failures; [clock] times the lock.
  SecureUi({
    required this.settings,
    required void Function(PluxException error) report,
    SecurePlatforms platforms = const SecurePlatforms(),
    ActionClock clock = const ActionClock(),
  }) : biometrics = platforms.biometrics,
       screen = SecureScreen(platforms.screen, report: report),
       lock = InactivityLock(
         settings: settings,
         clock: clock,
         biometrics: platforms.biometrics,
       );

  /// The security settings in force.
  final ValueListenable<SecuritySettings> settings;

  /// The biometric prompt.
  final BiometricPlatform biometrics;

  /// The screen protection.
  final SecureScreen screen;

  /// The inactivity lock.
  final InactivityLock lock;

  /// Stops the lock's timer and listeners.
  void dispose() => lock.dispose();
}

/// Gives the pages below it the runtime's [SecureUi] and the plugin they
/// belong to, which the secure widgets find with [of].
final class SecureUiScope extends InheritedWidget {
  /// Creates the scope for the pages of [plugin].
  const SecureUiScope({
    super.key,
    required this.ui,
    required this.plugin,
    required this.guard,
    required super.child,
    this.services = const {},
  });

  /// The runtime's secure UI.
  final SecureUi ui;

  /// The plugin whose page shows.
  final String plugin;

  /// Checks the plugin's device operations, or null where the device
  /// actions are not wired.
  final DeviceGuard? guard;

  /// The services the device actions find (the picked files, the packages'
  /// implementations), by type.
  final Map<Type, Object> services;

  /// The service of type [T] the runtime or a registered package provides.
  T? service<T extends Object>() => services[T] as T?;

  /// The scope above [context], or null outside a page.
  static SecureUiScope? maybeOf(BuildContext context) =>
      context.dependOnInheritedWidgetOfExactType<SecureUiScope>();

  @override
  bool updateShouldNotify(SecureUiScope old) =>
      ui != old.ui ||
      plugin != old.plugin ||
      guard != old.guard ||
      services != old.services;
}

/// Puts a page behind the protection of the screen and the inactivity lock
/// (SEC-091, SEC-093, SEC-094).
///
/// While it shows, a page marked [secure] holds the platform's screen
/// protection, and so does any page when the setting
/// `screenshotBlockingDefault` is on. A [secure] page also attaches to the
/// inactivity lock, and shows a lock screen in place of its content while
/// the lock is engaged; the content stays mounted, so that what the user
/// entered survives the unlock.
final class SecurePageGate extends StatefulWidget {
  /// Creates the gate.
  const SecurePageGate({
    super.key,
    required this.ui,
    required this.secure,
    required this.child,
  });

  /// The runtime's secure UI.
  final SecureUi ui;

  /// Whether the page is marked secure.
  final bool secure;

  /// The page.
  final Widget child;

  @override
  State<SecurePageGate> createState() => _SecurePageGateState();
}

final class _SecurePageGateState extends State<SecurePageGate>
    with WidgetsBindingObserver {
  SecureScreenLease? _lease;

  bool get _protect =>
      widget.secure ||
      widget.ui.settings.value.flag(SecuritySetting.screenshotBlockingDefault);

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
    widget.ui.settings.addListener(_settingsChanged);
    if (widget.secure) widget.ui.lock.attach();
    _sync();
  }

  @override
  void didUpdateWidget(SecurePageGate old) {
    super.didUpdateWidget(old);
    if (old.ui != widget.ui) {
      old.ui.settings.removeListener(_settingsChanged);
      widget.ui.settings.addListener(_settingsChanged);
    }
    if (old.secure != widget.secure || old.ui != widget.ui) {
      if (old.secure) old.ui.lock.detach();
      if (widget.secure) widget.ui.lock.attach();
    }
    if (old.ui != widget.ui) {
      _lease?.release();
      _lease = null;
    }
    _sync();
  }

  void _settingsChanged() => _sync();

  /// Holds the screen protection exactly while the page needs it.
  void _sync() {
    if (_protect) {
      _lease ??= widget.ui.screen.acquire();
    } else {
      _lease?.release();
      _lease = null;
    }
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    if (state == AppLifecycleState.resumed) widget.ui.lock.resumed();
  }

  @override
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    widget.ui.settings.removeListener(_settingsChanged);
    _lease?.release();
    if (widget.secure) widget.ui.lock.detach();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    if (!widget.secure) return widget.child;
    return ListenableBuilder(
      listenable: widget.ui.lock,
      builder: (context, _) {
        final locked = widget.ui.lock.locked;
        return Stack(
          // The page keeps the constraints its parent gives it; the lock
          // screen fills the room the hidden page leaves.
          fit: locked ? StackFit.expand : StackFit.passthrough,
          children: [
            // Mounted but neither painted, hit-tested nor in the semantics
            // tree while locked.
            Offstage(offstage: locked, child: widget.child),
            if (locked)
              Positioned.fill(child: LockScreen(lock: widget.ui.lock)),
          ],
        );
      },
    );
  }
}

/// What a secure page shows in place of its content while the inactivity
/// lock is engaged: nothing of the page, and a button that asks for the
/// biometric or the device credential. The prompt opens when the screen
/// appears.
final class LockScreen extends StatefulWidget {
  /// Creates the screen for [lock].
  const LockScreen({super.key, required this.lock});

  /// The engaged lock.
  final InactivityLock lock;

  /// The reason the system prompt shows.
  static const String reason = 'Unlock to continue';

  @override
  State<LockScreen> createState() => _LockScreenState();
}

final class _LockScreenState extends State<LockScreen> {
  BiometricOutcome? _outcome;

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) => _ask());
  }

  void _ask() {
    if (!mounted) return;
    unawaited(
      widget.lock.unlock(reason: LockScreen.reason).then((outcome) {
        if (mounted) setState(() => _outcome = outcome);
      }),
    );
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final message = switch (_outcome) {
      BiometricOutcome.unavailable =>
        'Set up a screen lock on this device to continue.',
      BiometricOutcome.failed => 'That did not work. Try again.',
      _ => 'Confirm that it is you to continue.',
    };
    return Material(
      color: theme.colorScheme.surface,
      child: Semantics(
        container: true,
        liveRegion: true,
        child: Center(
          child: Padding(
            padding: const EdgeInsets.all(24),
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                Icon(
                  Icons.lock_outline,
                  size: 48,
                  color: theme.colorScheme.primary,
                ),
                const SizedBox(height: 16),
                Text('Locked', style: theme.textTheme.titleLarge),
                const SizedBox(height: 8),
                Text(
                  message,
                  style: theme.textTheme.bodyMedium,
                  textAlign: TextAlign.center,
                ),
                const SizedBox(height: 24),
                ListenableBuilder(
                  listenable: widget.lock,
                  builder: (context, _) => FilledButton(
                    onPressed: widget.lock.asking ? null : _ask,
                    child: const Text('Unlock'),
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
