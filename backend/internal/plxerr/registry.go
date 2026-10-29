// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package plxerr

import "sort"

// Registered codes. A code and its reason are permanent (ADR-0018): a retired
// code keeps its definition, marked deprecated, and is never reassigned.
const (
	// Schema and validation: structure (PLX-1000–1099).

	UnknownProperty          Code = 1001
	MissingProperty          Code = 1002
	WrongJSONType            Code = 1003
	InvalidEnumValue         Code = 1004
	InvalidFormat            Code = 1005
	OutOfRange               Code = 1006
	InvalidStructure         Code = 1007
	InvalidJSON              Code = 1008
	UnsupportedSchemaVersion Code = 1009
	MigrationFailed          Code = 1010
	DuplicateID              Code = 1011
	InvalidProjectLayout     Code = 1020
	RequestTooLarge          Code = 1021
	RevisionConflict         Code = 1030
	IdempotencyConflict      Code = 1031
	InvalidPageToken         Code = 1032

	// Schema and validation: references and semantics (PLX-1100–1199).

	DuplicateKey                 Code = 1101
	UnresolvedReference          Code = 1102
	DuplicateRouteName           Code = 1103
	UnknownWidgetType            Code = 1104
	UnknownProp                  Code = 1105
	PropTypeMismatch             Code = 1106
	MissingRequiredProp          Code = 1107
	UnknownEvent                 Code = 1108
	InvalidChildren              Code = 1109
	UnknownSlot                  Code = 1110
	MissingRequiredSlot          Code = 1111
	UnknownAction                Code = 1112
	InvalidActionGraph           Code = 1113
	ComponentVersionNotFound     Code = 1114
	UnknownType                  Code = 1115
	InvalidTypeExpression        Code = 1116
	ValueTypeMismatch            Code = 1117
	MissingMock                  Code = 1118
	RuntimeTooOld                Code = 1119
	RequiredFeaturesRaised       Code = 1120
	ConstraintViolation          Code = 1121
	DeprecatedMember             Code = 1122
	UnknownIcon                  Code = 1123
	CustomActionNamedLikeBuiltIn Code = 1124
	FlowCallCycle                Code = 1125
	FlowNotExported              Code = 1126
	InvalidTrigger               Code = 1127
	UndeclaredComponentEvent     Code = 1128
	EmitEventOutsideComponent    Code = 1129
	InvalidRetryPolicy           Code = 1130

	// Schema and validation: data sources (PLX-1170–1189, P5 R4).

	DataSourceConfigInvalid    Code = 1170
	DataSourceDomainUndeclared Code = 1171
	DataMappingInvalid         Code = 1172
	DataPaginationInvalid      Code = 1173
	DataSourceSecretHeader     Code = 1174
	DataStreamInvalid          Code = 1175
	DataOutboxInvalid          Code = 1176
	DataTransferInvalid        Code = 1177
	StateEntryReadOnly         Code = 1140
	StatePatchNotObject        Code = 1141
	StatePersistenceNotAllowed Code = 1142
	StateMigrationRequired     Code = 1143
	StateMigrationMismatch     Code = 1144
	StateMigrationInvalid      Code = 1145

	// Schema and validation: forms (PLX-1160–1169, P5 R3).

	FormValidatorNotApplicable Code = 1160
	FormValidatorOptions       Code = 1161
	FormPatternInvalid         Code = 1162
	FormPhoneRegionUnknown     Code = 1163
	FormAsyncValidatorInvalid  Code = 1164
	FormFieldInitialMissing    Code = 1165
	FormNameConflict           Code = 1166
	FormWriteInvalid           Code = 1167
	UnknownRoute               Code = 1201
	RouteParameterMissing      Code = 1203
	RouteParameterTypeInvalid  Code = 1204
	UnknownRouteParameter      Code = 1205
	RedirectLoop               Code = 1206

	// Schema and validation: local collections (PLX-1190–1199, PLX-1250–1259, P5 R6).

	CollectionVersionInvalid Code = 1190
	CollectionPlanRequired   Code = 1191
	CollectionPlanInvalid    Code = 1192
	CollectionKeyChanged     Code = 1193
	CollectionDestructive    Code = 1194
	DatabaseSourceInvalid    Code = 1195

	// Schema and validation: animation (PLX-1210–1229, P5 R7).

	AnimationInvalid          Code = 1210
	AnimationTargetInvalid    Code = 1211
	AnimationValueInvalid     Code = 1212
	TransitionInvalid         Code = 1213
	AnimationExpensive        Code = 1214
	AnimationOpacitySubtree   Code = 1215
	AnimationTooManyTimelines Code = 1216
	NodeAnimationInvalid      Code = 1217

	// Schema and validation: capabilities and device actions (PLX-1230–1239, P5 R8).

	CapabilityNotApproved      Code = 1230
	DeviceCapabilityUndeclared Code = 1231
	OpenURLDomainUndeclared    Code = 1232
	HostBuildLacksPackage      Code = 1233

	// Schema and validation: form scopes (PLX-1240–1249, P5).

	FormScopeInvalid Code = 1240

	// Schema and validation: local collections, key types (PLX-1250–1259, P5 R6).

	CollectionKeyTypeInvalid Code = 1250

	// Import: OpenAPI and GraphQL data sources (PLX-1260–1269, P5 R9).

	ImportDocumentInvalid      Code = 1260
	ImportConstructUnsupported Code = 1261
	ImportOperationInvalid     Code = 1262
	ImportOutputInvalid        Code = 1263

	// Plux Test: scenarios and runs (PLX-1270–1279, P5 R9).

	ScenarioFileInvalid      Code = 1270
	ScenarioSyntaxInvalid    Code = 1271
	ScenarioReferenceUnknown Code = 1272
	ScenarioUnsupported      Code = 1273
	ScenarioNameDuplicate    Code = 1274
	TestHarnessFailed        Code = 1275
	ScenarioFilesNone        Code = 1276

	// Schema and validation: limits and budgets (PLX-1300–1399).

	PageNodeBudget      Code = 1310
	PageDepthBudget     Code = 1311
	PageBuildCostBudget Code = 1312
	PageImageBudget     Code = 1313
	PageAnimationBudget Code = 1314
	LimitExceeded       Code = 1320
	LimitApproaching    Code = 1321

	// Schema and validation: accessibility, security and store policy
	// (PLX-1400–1599).

	AccessibleNameMissing              Code = 1401
	ImageAlternativeMissing            Code = 1402
	ContrastInsufficient               Code = 1403
	TouchTargetTooSmall                Code = 1404
	FixedHeightText                    Code = 1405
	FocusOrderInvalid                  Code = 1406
	HeadingLevelInvalid                Code = 1407
	LiveRegionStatic                   Code = 1408
	ContrastNotChecked                 Code = 1409
	A11yWaiverInvalid                  Code = 1410
	A11yWaiverPermissionRequired       Code = 1411
	SecretLikeValue                    Code = 1500
	SensitiveValueExposed              Code = 1501
	InsecureURL                        Code = 1502
	ExecutableContent                  Code = 1503
	DirectSourceAssuranceUnenforceable Code = 1504

	// Localisation and translation import (PLX-1600–1699, P8).

	LocaleUnsupported              Code = 1600
	MessageSyntaxInvalid           Code = 1601
	MessageArgumentUndeclared      Code = 1602
	MessageArgumentTypeMismatch    Code = 1603
	PluralCategoryInvalid          Code = 1604
	PluralOtherMissing             Code = 1605
	TranslationMissing             Code = 1606
	TranslationTooLong             Code = 1607
	TranslationKeyUnused           Code = 1608
	LiteralTextUntranslated        Code = 1609
	TranslationNotApproved         Code = 1610
	CalendarUnsupported            Code = 1611
	NumberingSystemUnsupported     Code = 1612
	LocaleOverrideInvalid          Code = 1613
	PseudoLocaleInProduction       Code = 1614
	CalendarOutOfTable             Code = 1615
	TranslationImportInvalid       Code = 1620
	TranslationImportKeyUnknown    Code = 1621
	TranslationImportSourceChanged Code = 1622
	TranslationImportDowngraded    Code = 1623

	// Compiler and PXL (PLX-2000–2999).

	PXLSyntaxError          Code = 2001
	PXLTypeMismatch         Code = 2002
	PXLUnknownIdentifier    Code = 2003
	PXLUnknownFunction      Code = 2004
	PXLWrongArgumentCount   Code = 2005
	PXLNullableAccess       Code = 2006
	PXLDecimalDivision      Code = 2007
	PXLCurrencyMismatch     Code = 2008
	PXLInvalidLiteral       Code = 2009
	PXLBudgetExceeded       Code = 2010
	PXLConstantError        Code = 2011
	PXLUnknownField         Code = 2012
	PXLFeatureRequired      Code = 2013
	PXLUnknownEnumMember    Code = 2014
	PXLInvalidMacro         Code = 2015
	PXLExpressionTooComplex Code = 2016
	PXLRegexInvalid         Code = 2020
	PXLRegexNotConstant     Code = 2021
	PXLRegexTooLarge        Code = 2022
	PXLUnknownPhoneRegion   Code = 2023
	InternalCompilerError   Code = 2201
	ContentIDCollision      Code = 2202

	// Release and sync (PLX-3000–3999).

	ManifestSignatureInvalid   Code = 3001
	ManifestExpired            Code = 3002
	RollbackRejected           Code = 3003
	UnsupportedRequiredFeature Code = 3010
	PatchHashMismatch          Code = 3011
	DeltaMalformed             Code = 3012
	BundleMalformed            Code = 3040
	SectionHashMismatch        Code = 3041
	SectionVerificationFailed  Code = 3042
	BundleEncryptedUnsupported Code = 3043
	TransportDecodingFailed    Code = 3044
	AssetHashMismatch          Code = 3045
	SyncFailed                 Code = 3050
	RevertedToLastKnownGood    Code = 3020
	DiskQuotaExceeded          Code = 3030

	// Publish gates: accessibility and golden tests (PLX-3060–3069, P8).

	AccessibilityBlocksPublish     Code = 3060
	GoldenRunMissing               Code = 3061
	GoldenDifferenceUnapproved     Code = 3062
	ReleaseProfileBelowEnvironment Code = 3063
	PseudoLocalesInProduction      Code = 3064

	// Runtime rendering and navigation (PLX-4000–4999).

	NodeBuildFailed           Code = 4001
	PropValueInvalid          Code = 4002
	UnknownWidget             Code = 4003
	ActionsNotAvailable       Code = 4010
	PluginDisabled            Code = 4020
	RouteNotFound             Code = 4100
	RouteParametersInvalid    Code = 4101
	NavigationRefused         Code = 4102
	DeepLinkUnmapped          Code = 4103
	NativeRouteNotRegistered  Code = 4200
	NativeSlotNotRegistered   Code = 4201
	NativeActionNotRegistered Code = 4202
	ExposedStateTypeMismatch  Code = 4203
	UserContextInvalid        Code = 4204
	HostCodeFailed            Code = 4205

	// Runtime animation (PLX-4300–4399, P5 R7).

	AnimationUnknown        Code = 4300
	AnimationCommandInvalid Code = 4301
	AnimationTimelineBroken Code = 4302
	AnimationAssetFailed    Code = 4303

	// Runtime localisation and widget packages (PLX-4400–4499, P8).

	LocaleNotInRelease   Code = 4400
	TranslationFallback  Code = 4401
	MessageFormatFailed  Code = 4402
	WidgetPackageMissing Code = 4403

	// Actions, data and local DB (PLX-5000–5999).

	ActionTimeout           Code = 5001
	ActionStepLimitExceeded Code = 5002
	ActionValueInvalid      Code = 5003
	ActionCustomError       Code = 5004
	ActionCancelled         Code = 5005
	ForEachLimitExceeded    Code = 5006
	FlowNotFound            Code = 5007
	ErrorHandlerFailed      Code = 5008
	ActionQueueFull         Code = 5009

	// Data sources (PLX-5100–5199, P5 R4).

	DataDomainBlocked     Code = 5100
	DataNetworkFailed     Code = 5101
	DataHTTPError         Code = 5102
	DataRequestTimeout    Code = 5103
	DataMappingFailed     Code = 5104
	DataSizeExceeded      Code = 5105
	DataGraphQLError      Code = 5106
	DataUnauthorised      Code = 5107
	DataSourceUnavailable Code = 5108
	DataCacheUnavailable  Code = 5109

	// Data sources: streams, the outbox and transfers (PLX-5110–5199, P5 R5).

	DataStreamFailed          Code = 5110
	DataStreamMessageTooLarge Code = 5111
	DataStreamLimit           Code = 5112
	DataOutboxFull            Code = 5120
	DataOutboxUnavailable     Code = 5121
	DataOutboxConflict        Code = 5122
	DataOutboxRejected        Code = 5123
	DataTransferTooLarge      Code = 5130
	DataTransferFileFailed    Code = 5131

	// Local database (PLX-5200–5299, P5 R6).

	DBMigrationFailed      Code = 5200
	DBUnavailable          Code = 5201
	DBEncryptionRequired   Code = 5202
	DBRecordInvalid        Code = 5203
	DBRecordNotFound       Code = 5204
	DBKeyConflict          Code = 5205
	DBQueryInvalid         Code = 5206
	DBLimitExceeded        Code = 5207
	DBStoreFailed          Code = 5208
	DBCollectionUnknown    Code = 5209
	DBValueTypeMismatch    Code = 5210
	StateWriteTypeMismatch Code = 5301
	StateWriteRefused      Code = 5302
	StateStoreUnavailable  Code = 5303
	StateStoreCorrupt      Code = 5304
	StateMigrationFailed   Code = 5305
	StateLimitExceeded     Code = 5306
	HostEventRefused       Code = 5307

	// Actions: forms (PLX-5350–5399, P5 R3).

	FormInvalid              Code = 5350
	FormNotInScope           Code = 5351
	FormAsyncValidatorFailed Code = 5352

	// Actions: device and feedback (PLX-5400–5499, P5 R8).

	DeviceCapabilityBlocked Code = 5400
	DevicePackageMissing    Code = 5401
	DevicePermissionDenied  Code = 5402
	DeviceUnavailable       Code = 5403
	OpenURLBlocked          Code = 5404
	ClipboardBlocked        Code = 5405

	// Actions: typed host events (PLX-5500–5549, P5).

	HostEventPayloadInvalid Code = 5500

	// Security (PLX-6000–6999).

	AttestationFailed            Code = 6001
	AssuranceInsufficient        Code = 6002
	KeyNotHardwareBacked         Code = 6003
	DevProviderInProduction      Code = 6004
	RegistrationChallengeInvalid Code = 6005
	DeviceRevoked                Code = 6006
	ReattestationRequired        Code = 6007
	LegacyRegistrationRefused    Code = 6008
	AttestationUnavailable       Code = 6009
	DPoPProofInvalid             Code = 6010
	DPoPReplay                   Code = 6011
	DPoPNonceRequired            Code = 6012
	TokenBindingMismatch         Code = 6013
	AccessTokenInvalid           Code = 6014
	ReplayCacheUnavailable       Code = 6015
	UserTokenInvalid             Code = 6016
	DeviceKeyUnavailable         Code = 6017
	CertificatePinMismatch       Code = 6020
	DevelopmentKeyInProduction   Code = 6021
	UpdateMetadataInvalid        Code = 6022
	OutboundRequestBlocked       Code = 6030
	AssetRejected                Code = 6031
	AssetUploadNotFound          Code = 6032
	AssetUploadOutOfOrder        Code = 6033
	AssetUploadChunkTooLarge     Code = 6034
	AssetUploadTooLarge          Code = 6035
	AssetUploadHashMismatch      Code = 6036
	AssetUploadSizeMismatch      Code = 6037
	SecurityConfigHashMismatch   Code = 6040
	SecurityConfigLoosensPreset  Code = 6041
	SecurityConfigOutOfBounds    Code = 6042
	ConfidentialKeyUnavailable   Code = 6050
	GatewayUpstreamUnknown       Code = 6060
	GatewayDestinationRefused    Code = 6061
	GatewayUpstreamFailed        Code = 6062
	DirectDataSourceRefused      Code = 6063
	GatewayRateLimited           Code = 6064
	BoundLocationSetByDevice     Code = 6065
	BoundValueChanged            Code = 6066
	LiveConfigStale              Code = 6067
	LiveConfigUnavailable        Code = 6068
	ConfigStoreUnavailable       Code = 6069
	SCASignatureInvalid          Code = 6070
	SCAChallengeInvalid          Code = 6071
	SCADynamicLinkingMismatch    Code = 6072
	TransactionRequiresGateway   Code = 6073
	FraudReceiptUnavailable      Code = 6080
	ImportedKeyInvalid           Code = 6081
	RASPDetection                Code = 6100

	// Functions (PLX-7000–7999).

	FunctionTimeout                  Code = 7001
	FunctionMemoryExceeded           Code = 7002
	FunctionCapabilityDenied         Code = 7003
	FunctionOutputTooLarge           Code = 7004
	FunctionCallDepthExceeded        Code = 7005
	FunctionHTTPCallsExceeded        Code = 7006
	FunctionTrapped                  Code = 7007
	FunctionInputInvalid             Code = 7008
	FunctionOutputInvalid            Code = 7009
	FunctionEgressRefused            Code = 7010
	IdempotencyKeyConflict           Code = 7011
	FunctionNotFound                 Code = 7012
	FunctionAliasNotFound            Code = 7013
	DeviceFunctionsDisabled          Code = 7014
	FunctionRateLimited              Code = 7015
	FunctionKVLimitExceeded          Code = 7016
	FunctionABIUnsupported           Code = 7017
	FunctionModuleInvalid            Code = 7018
	FunctionArtifactSignatureInvalid Code = 7019
	FunctionTestsFailed              Code = 7020
	FunctionCancelled                Code = 7021
	FunctionInputTooLarge            Code = 7022
	DeviceFunctionServerCapability   Code = 7030
	FunctionReferenceUnresolved      Code = 7031
	FunctionTypeMismatch             Code = 7032
	FunctionBusinessError            Code = 7100
	FunctionBuildFailed              Code = 7200
	FunctionDependencyNotVendored    Code = 7201
	FunctionBuildScriptRefused       Code = 7202
	FunctionBuildLimitExceeded       Code = 7203
	FunctionSchemaDerivationFailed   Code = 7204
	FunctionDeclarationMismatch      Code = 7205
	FunctionBuildNotReproducible     Code = 7206

	// Governance (PLX-8000–8999).

	MultiFactorRequired          Code = 8011
	AuthenticationRequired       Code = 8012
	EditingLockHeld              Code = 8020
	PermissionDenied             Code = 8030
	ResourceNotFound             Code = 8031
	ResourceExists               Code = 8032
	PreconditionFailed           Code = 8033
	RateLimited                  Code = 8040
	ReleaseInconsistent          Code = 8050
	WarningsNotAcknowledged      Code = 8051
	PluginNotPublished           Code = 8052
	AssetsNotReady               Code = 8053
	HostBuildIncompatible        Code = 8054
	ReleaseRequestState          Code = 8060
	ReleaseRequestEnded          Code = 8061
	VisualDiffRequired           Code = 8062
	LibraryPublicScanFailed      Code = 8063
	InternalServerError          Code = 8090
	UpstreamUnavailable          Code = 8091
	TranslationStateForbidden    Code = 8092
	TranslationDocumentForbidden Code = 8093

	// Studio, CLI and AI (PLX-9000–9999).

	CLIConfigurationInvalid Code = 9100
	ProjectNotFound         Code = 9101
	FileSystemError         Code = 9102

	// Dev sessions and pairing (PLX-9200–9229, P10).

	DevSessionExpired     Code = 9200
	PairingTokenExpired   Code = 9201
	PairingTokenUsed      Code = 9202
	DevSessionEnvironment Code = 9203
	DeviceNotPaired       Code = 9204
	SocketTicketInvalid   Code = 9205
	DevCommandNotAllowed  Code = 9206
	DevPatchRejected      Code = 9207
	DevSessionDeviceLimit Code = 9208
	DevEventTooLarge      Code = 9209

	// Device-side devtools (PLX-9230–9249, P10).

	DevBundleSignatureInvalid Code = 9230
	HotApplyRefused           Code = 9231
	DevCommandUnsupported     Code = 9232
	TraceFileUnreadable       Code = 9233

	// Golden test harness (PLX-9300–9399, P8).

	GoldenHarnessFailed   Code = 9300
	FlutterSDKUnavailable Code = 9301

	// Experiments, rollouts, governance, federation and licence (P9).

	PolicyDocumentInvalid     Code = 1700
	UnknownFlagType           Code = 1701
	VariantWithoutControl     Code = 1702
	VariantWeightsInvalid     Code = 1703
	SegmentRuleInvalid        Code = 1704
	ExperimentKeyUnknown      Code = 2300
	WinnerNotAVariant         Code = 2301
	VariantTreeExceedsBudget  Code = 2302
	ControlSignatureInvalid   Code = 3100
	ControlDocumentExpired    Code = 3101
	ControlRollbackRejected   Code = 3102
	MandatoryUpdatePending    Code = 3103
	ForcedRollbackApplied     Code = 3104
	ActionGraphSwitchedOff    Code = 3105
	DataSourceSwitchedOff     Code = 3106
	FunctionSwitchedOff       Code = 3107
	RolloutPausedByHealthGate Code = 3150
	RolloutRolledBack         Code = 3151
	TargetingRuleInvalid      Code = 3152
	ScheduleOverlapsRollout   Code = 3153

	// Background sync (PLX-3500–3509, P10).

	BackgroundSyncFailed            Code = 3500
	BackgroundSyncNotConfigured     Code = 3501
	BackgroundSyncConstraints       Code = 3502
	StagedReleaseRejected           Code = 3503
	StepUpRequired                  Code = 6200
	StepUpExpired                   Code = 6201
	WebAuthnAssertionInvalid        Code = 6202
	SAMLResponseInvalid             Code = 6210
	SAMLAssertionReplayed           Code = 6211
	SCIMRequestInvalid              Code = 6220
	SessionRevoked                  Code = 6230
	SessionIdleTimeout              Code = 6231
	SessionAbsoluteTimeout          Code = 6232
	CronExpressionInvalid           Code = 7150
	WebhookSignatureInvalid         Code = 7151
	WebhookReplayed                 Code = 7152
	ApprovalRequired                Code = 8001
	ApprovalInvalidated             Code = 8002
	ApproverNotEligible             Code = 8003
	ApprovalExpired                 Code = 8004
	BreakGlassJustificationRequired Code = 8005
	PostHocReviewOverdue            Code = 8006
	ChangeFreezeActive              Code = 8010
	RoleUnknown                     Code = 8100
	PermissionNotInCatalogue        Code = 8101
	ScopeInvalid                    Code = 8102
	LastOwnerCannotBeRemoved        Code = 8103
	RestrictionPatternInvalid       Code = 8104
	RestrictionLocksOutOwner        Code = 8105
	RestrictionExceedsCaller        Code = 8106
	RestrictionLimitReached         Code = 8107
	ExperimentOverlapsLayer         Code = 8200
	SampleSizeNotReached            Code = 8201
	SampleRatioMismatch             Code = 8202
	FlagTypeChangeRefused           Code = 8203
	IngestionOverQuota              Code = 8300
	RetentionChangeScheduled        Code = 8301
	ExportDestinationUnreachable    Code = 8302
	ChannelMisconfigured            Code = 8400
	DeliveryFailed                  Code = 8401
	SIEMDestinationUnreachable      Code = 8500
	SubjectNotFound                 Code = 8501
	ReconstructionDataExpired       Code = 8502
	GitRemoteRefused                Code = 8503
	EnterpriseFeatureNotLicensed    Code = 8600
	LicenseInvalid                  Code = 8601
	LicenseExpired                  Code = 8602
	LicenseLimitExceeded            Code = 8603
)

// Definition is the registered description of a code (DX-003).
type Definition struct {
	// Code is the stable code.
	Code Code `json:"code"`
	// Reason is the stable UPPER_SNAKE_CASE identifier.
	Reason Reason `json:"reason"`
	// Severity is the default severity of diagnostics with this code; policy
	// may raise it, never lower it.
	Severity Severity `json:"severity"`
	// Title names the problem in a few words.
	Title string `json:"title"`
	// Cause explains the likely cause in complete sentences.
	Cause string `json:"cause"`
	// Fix suggests how to resolve the problem.
	Fix string `json:"fix"`
	// Deprecated marks a retired code that is kept so it is never reused.
	Deprecated bool `json:"deprecated,omitempty"`
}

// Lookup returns the definition of a registered code.
func Lookup(c Code) (Definition, bool) {
	i := sort.Search(len(registry), func(i int) bool { return registry[i].Code >= c })
	if i < len(registry) && registry[i].Code == c {
		return registry[i], true
	}
	return Definition{}, false
}

// Definitions returns a copy of every registered definition in ascending
// code order.
func Definitions() []Definition { return append([]Definition(nil), registry...) }
