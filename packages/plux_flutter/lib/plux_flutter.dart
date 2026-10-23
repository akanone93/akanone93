// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The Plux runtime for Flutter.
///
/// The runtime syncs every plugin of an app at start-up, verifies signed
/// bundles before loading them, and renders them as native widgets (spec
/// §12). Start it with [Plux.initialize], show pages with [PluxView] or
/// [Plux.open], and follow updates on [Plux.syncEvents].
library;

export 'src/actions/trace.dart'
    show PluxActionTrace, PluxStepTrace, PluxTraceStatus;
export 'src/assets/icon_fonts.dart' show PluxIconSource;
export 'src/core/app_state.dart' show PluxState;
export 'src/core/config.dart'
    show
        ActivationPolicy,
        PluxAuthDelegate,
        PluxConfig,
        PluxConsent,
        PluxErrorHandler,
        PluxFallbackBuilder,
        PluxPublicKey,
        PluxThemeSource,
        PluxUpdateScreenBuilder,
        PluxUser,
        StartupPolicy;
export 'src/core/fallback.dart' show PluxDefaultFallback;
export 'src/core/host_events.dart' show PluxHostEvent;
export 'src/core/plux.dart' show Plux, PluxScope, PluxSyncTile;
export 'src/core/plux_view.dart' show PluxView, PluxViewEvent, PluxViewSizing;
export 'src/core/runtime.dart' show PluxStartup;
export 'src/core/update_screen.dart' show PluxDefaultUpdateScreen;
export 'src/db/adapter.dart'
    show DbMigrationOutcome, DbOperations, PluxDatabaseAdapter, kvTypeOf;
export 'src/db/memory_adapter.dart' show MemoryDatabaseAdapter;
export 'src/db/query.dart'
    show
        DbAnd,
        DbCompare,
        DbFilter,
        DbNot,
        DbOp,
        DbOr,
        DbQuery,
        DbSort,
        compareValues;
export 'src/db/schema.dart'
    show DbCollectionSchema, DbField, DbFieldKind, DbMigrationPlan, DbMigrator;
export 'src/device/types.dart'
    show
        PluxCodeScanner,
        PluxDeviceException,
        PluxDeviceFailure,
        PluxDevicePackage,
        PluxKycCapture,
        PluxKycPhoto,
        PluxLocationProvider,
        PluxMediaPicker,
        PluxPickedFile,
        PluxPosition,
        PluxScanResult;
export 'src/devtools_api/diagnostics.dart'
    show PluxDiagnostic, PluxDiagnostics, PluxPluginInfo, PluxReleaseInfo;
export 'src/errors/plux_exception.dart' show PluxErrorCode, PluxException;
export 'src/functions/engine.dart'
    show
        PluxFunctionBusinessError,
        PluxFunctionCancel,
        PluxFunctionContext,
        PluxFunctionEngine,
        PluxFunctionKv,
        PluxFunctionLimits,
        PluxFunctionOutput,
        PluxFunctionResult;
export 'src/l10n/l10n.dart'
    show
        CurrencyDisplay,
        FractionDigits,
        L10nException,
        MoneyOptions,
        NumberOptions,
        PluxCalendarDate,
        PluxDateFormatting,
        PluxDateOptions,
        PluxDecimal,
        PluxLocaleData,
        PluxNumberFormatting;
export 'src/l10n/runtime_strings.dart' show RuntimeStrings;
export 'src/l10n/scope.dart' show PluxLocaleScope;
export 'src/native_catalogue/registration.dart'
    show
        PluxNativeAction,
        PluxNativeRoute,
        PluxNativeSlot,
        PluxRouterAdapter,
        PluxSlot;
export 'src/navigation/delegate.dart'
    show
        PluxNavigationDelegate,
        PluxNavigatorDelegate,
        PluxPageRoute,
        PluxPresentation,
        PluxRouteSpec,
        PluxTransition;
export 'src/navigation/plux_page.dart' show PluxPage;
export 'src/navigation/router.dart' show PluxNotFoundPage;
export 'src/navigation/shell.dart' show PluxShell, PluxShellScope, PluxShellTab;
export 'src/platform/platform_services.dart'
    show PlatformSecretStore, platformStorageDirectory;
export 'src/pxl/builtins.dart' show isIbanNumber;
export 'src/pxl/decimal.dart' show Decimal;
export 'src/pxl/phone.dart'
    show isPhoneRegion, isValidPhone, phoneCallingCode, phoneRegions;
export 'src/pxl/values.dart' show Money, PxlDate, PxlDateTime, civilFromDays;
export 'src/reconstruction/reconstruction.dart'
    show
        PluxReconstruction,
        PluxReconstructionBundle,
        PluxReconstructionMoment,
        PluxReconstructionSignature;
export 'src/reconstruction/view.dart'
    show PluxReconstructionBanner, PluxReconstructionView;
export 'src/render/builders/builders.dart' show ItemCollection;
export 'src/render/decoders.dart'
    show
        decodeAlignment,
        decodeEdgeInsets,
        decodeEdgeInsetsResolved,
        decodeIconData,
        decodeIconWidget,
        decodeImageSource,
        decodeScrollPhysics,
        decodeWidgetStateColor;
export 'src/render/decoding.dart'
    show
        DecodeError,
        Decoding,
        Fields,
        PluxObject,
        asBool,
        asColor,
        asDouble,
        asDoubles,
        asInt,
        asList,
        asString,
        asStrings;
export 'src/render/generated/render.g.dart'
    show
        decodeAxis,
        decodeBorderSide,
        decodeBoxDecoration,
        decodeClip,
        decodeCupertinoDatePickerMode,
        decodeDragStartBehavior,
        decodeHitTestBehavior,
        decodeNavigationRailLabelType,
        decodeOffset,
        decodePanAxis,
        decodeScrollViewKeyboardDismissBehavior,
        decodeTextBaseline,
        decodeTextDirection,
        decodeTextStyle,
        nameCardBrand,
        nameColumnWidthKind,
        nameCountdownFormat,
        nameListStatus,
        nameMaskKind,
        nameMoneyDisplay,
        nameSignDisplay,
        nameStepState,
        nameStepperType,
        nameTableCellVerticalAlignment,
        nameTimelineStepState,
        nameTransactionStatus;
export 'src/render/keep_scope.dart' show keepScope;
export 'src/render/node_context.dart' show NodeBuilder, NodeContext;
export 'src/render/plux_icon.dart' show PluxIcon;
export 'src/render/widget_package.dart' show PluxWidgetPackage;
export 'src/runtime_info.dart';
export 'src/schema/limits.g.dart' show PluxLimit, PluxLimitUnit;
export 'src/security/key_wrap.dart'
    show HardwareWrappedSecretStore, KeyWrapper, PlatformKeyWrapper;
export 'src/security/rasp.dart' show PluxSecurityHost, PluxSecurityModule;
export 'src/security/security_config.dart'
    show
        SecurityProfile,
        SecuritySetting,
        SecuritySettingType,
        SecuritySettings;
export 'src/store/kv_store.dart'
    show SecretStore, StoreException, StoreFailure, installationKey;
export 'src/sync/background_sync_config.dart' show BackgroundSyncConfig;
export 'src/sync/sync_event.dart'
    show
        SyncActivated,
        SyncChecking,
        SyncDownloading,
        SyncEvent,
        SyncFailed,
        SyncOutcome,
        SyncResult,
        SyncRolledBack,
        SyncRun,
        SyncStaged,
        SyncUpToDate;
