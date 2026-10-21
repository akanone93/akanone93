// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Package api is the ConnectRPC edge: handlers that authenticate,
// authorise, translate and delegate, and the interceptors that do the
// cross-cutting work (L-1, ADR-0005).
//
// Handlers hold no business logic. Domain packages return plxerr errors
// with registered reasons; one interceptor here turns them into Connect
// errors with google.rpc.ErrorInfo and the Plux code (SRV-006). No
// domain package imports connectrpc (L-2).
package api

import (
	"errors"
	"fmt"
	"net/http"

	"connectrpc.com/connect"
	"google.golang.org/genproto/googleapis/rpc/errdetails"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// Domain is the error domain of every ErrorInfo this server returns.
const Domain = "plux.dev"

// codes maps each registered reason to the Connect code that carries it.
// It is a table rather than a chain of type switches, so that adding a
// failure mode is one line and an unmapped one is visible.
var codes = map[plxerr.Code]connect.Code{
	// The caller sent something the server cannot accept.
	plxerr.UnknownProperty:          connect.CodeInvalidArgument,
	plxerr.MissingProperty:          connect.CodeInvalidArgument,
	plxerr.WrongJSONType:            connect.CodeInvalidArgument,
	plxerr.InvalidEnumValue:         connect.CodeInvalidArgument,
	plxerr.InvalidFormat:            connect.CodeInvalidArgument,
	plxerr.OutOfRange:               connect.CodeInvalidArgument,
	plxerr.InvalidStructure:         connect.CodeInvalidArgument,
	plxerr.InvalidJSON:              connect.CodeInvalidArgument,
	plxerr.UnsupportedSchemaVersion: connect.CodeInvalidArgument,
	plxerr.InvalidProjectLayout:     connect.CodeInvalidArgument,
	plxerr.InvalidPageToken:         connect.CodeInvalidArgument,
	plxerr.RequestTooLarge:          connect.CodeInvalidArgument,

	// The caller's view of the world is out of date.
	plxerr.RevisionConflict:    connect.CodeAborted,
	plxerr.IdempotencyConflict: connect.CodeAlreadyExists,

	// The caller may not do this, or may not know that it exists.
	plxerr.AuthenticationRequired:       connect.CodeUnauthenticated,
	plxerr.MultiFactorRequired:          connect.CodePermissionDenied,
	plxerr.PermissionDenied:             connect.CodePermissionDenied,
	plxerr.ResourceNotFound:             connect.CodeNotFound,
	plxerr.ResourceExists:               connect.CodeAlreadyExists,
	plxerr.PreconditionFailed:           connect.CodeFailedPrecondition,
	plxerr.EditingLockHeld:              connect.CodeFailedPrecondition,
	plxerr.TranslationStateForbidden:    connect.CodePermissionDenied,
	plxerr.TranslationDocumentForbidden: connect.CodePermissionDenied,

	// The caller must slow down or ask for less.
	plxerr.RateLimited:      connect.CodeResourceExhausted,
	plxerr.LimitExceeded:    connect.CodeResourceExhausted,
	plxerr.LimitApproaching: connect.CodeResourceExhausted,

	// The server refused to reach somewhere.
	plxerr.OutboundRequestBlocked: connect.CodePermissionDenied,
	plxerr.AssetRejected:          connect.CodeInvalidArgument,

	// Device identity, tokens, gateway and strong customer authentication.
	plxerr.AttestationFailed:            connect.CodePermissionDenied,
	plxerr.AssuranceInsufficient:        connect.CodePermissionDenied,
	plxerr.KeyNotHardwareBacked:         connect.CodePermissionDenied,
	plxerr.DeviceKeyUnavailable:         connect.CodeFailedPrecondition,
	plxerr.DevProviderInProduction:      connect.CodePermissionDenied,
	plxerr.RegistrationChallengeInvalid: connect.CodeInvalidArgument,
	plxerr.DeviceRevoked:                connect.CodeUnauthenticated,
	plxerr.ReattestationRequired:        connect.CodeFailedPrecondition,
	plxerr.LegacyRegistrationRefused:    connect.CodeFailedPrecondition,
	plxerr.AttestationUnavailable:       connect.CodeUnavailable,
	plxerr.DPoPProofInvalid:             connect.CodeUnauthenticated,
	plxerr.DPoPReplay:                   connect.CodeUnauthenticated,
	plxerr.DPoPNonceRequired:            connect.CodeUnauthenticated,
	plxerr.TokenBindingMismatch:         connect.CodeUnauthenticated,
	plxerr.AccessTokenInvalid:           connect.CodeUnauthenticated,
	plxerr.ReplayCacheUnavailable:       connect.CodeUnavailable,
	plxerr.UserTokenInvalid:             connect.CodeUnauthenticated,
	plxerr.CertificatePinMismatch:       connect.CodeUnavailable,
	plxerr.DevelopmentKeyInProduction:   connect.CodePermissionDenied,
	plxerr.UpdateMetadataInvalid:        connect.CodePermissionDenied,
	plxerr.SecurityConfigHashMismatch:   connect.CodeDataLoss,
	plxerr.SecurityConfigLoosensPreset:  connect.CodeInvalidArgument,
	plxerr.SecurityConfigOutOfBounds:    connect.CodeInvalidArgument,
	plxerr.ConfidentialKeyUnavailable:   connect.CodePermissionDenied,
	plxerr.GatewayUpstreamUnknown:       connect.CodeNotFound,
	plxerr.GatewayDestinationRefused:    connect.CodePermissionDenied,
	plxerr.GatewayUpstreamFailed:        connect.CodeUnavailable,
	plxerr.DirectDataSourceRefused:      connect.CodePermissionDenied,
	plxerr.GatewayRateLimited:           connect.CodeResourceExhausted,
	plxerr.BoundLocationSetByDevice:     connect.CodeInvalidArgument,
	plxerr.BoundValueChanged:            connect.CodeFailedPrecondition,
	plxerr.LiveConfigStale:              connect.CodeFailedPrecondition,
	plxerr.LiveConfigUnavailable:        connect.CodeUnavailable,
	plxerr.ConfigStoreUnavailable:       connect.CodeUnavailable,
	plxerr.SCASignatureInvalid:          connect.CodePermissionDenied,
	plxerr.SCAChallengeInvalid:          connect.CodeInvalidArgument,
	plxerr.SCADynamicLinkingMismatch:    connect.CodePermissionDenied,
	plxerr.TransactionRequiresGateway:   connect.CodeFailedPrecondition,
	plxerr.RASPDetection:                connect.CodePermissionDenied,

	// Functions (FN-006, FN-009, FN-016, FN-018, FN-031).
	plxerr.FunctionNotFound:                 connect.CodeNotFound,
	plxerr.FunctionAliasNotFound:            connect.CodeNotFound,
	plxerr.FunctionKVLimitExceeded:          connect.CodeResourceExhausted,
	plxerr.FunctionBuildLimitExceeded:       connect.CodeResourceExhausted,
	plxerr.FunctionBuildFailed:              connect.CodeFailedPrecondition,
	plxerr.FunctionArtifactSignatureInvalid: connect.CodeFailedPrecondition,
	plxerr.FunctionTestsFailed:              connect.CodeFailedPrecondition,
	plxerr.FunctionDeclarationMismatch:      connect.CodeFailedPrecondition,

	// Invoking a function (FN-020, FN-022): the system errors a device
	// sees as a code with a generic message.
	plxerr.FunctionTimeout:           connect.CodeDeadlineExceeded,
	plxerr.FunctionMemoryExceeded:    connect.CodeResourceExhausted,
	plxerr.FunctionCapabilityDenied:  connect.CodePermissionDenied,
	plxerr.FunctionOutputTooLarge:    connect.CodeResourceExhausted,
	plxerr.FunctionCallDepthExceeded: connect.CodeResourceExhausted,
	plxerr.FunctionHTTPCallsExceeded: connect.CodeResourceExhausted,
	plxerr.FunctionTrapped:           connect.CodeAborted,
	plxerr.FunctionInputInvalid:      connect.CodeInvalidArgument,
	plxerr.FunctionOutputInvalid:     connect.CodeAborted,
	plxerr.FunctionEgressRefused:     connect.CodePermissionDenied,
	plxerr.IdempotencyKeyConflict:    connect.CodeAlreadyExists,
	plxerr.FunctionRateLimited:       connect.CodeResourceExhausted,
	plxerr.FunctionABIUnsupported:    connect.CodeFailedPrecondition,
	plxerr.FunctionModuleInvalid:     connect.CodeFailedPrecondition,
	plxerr.FunctionCancelled:         connect.CodeCanceled,
	plxerr.FunctionInputTooLarge:     connect.CodeInvalidArgument,

	// Publish gates: the release was refused (A11Y-002, TST-003).
	plxerr.AccessibilityBlocksPublish: connect.CodeFailedPrecondition,
	plxerr.GoldenRunMissing:           connect.CodeFailedPrecondition,
	plxerr.GoldenDifferenceUnapproved: connect.CodeFailedPrecondition,

	// Translation import: the file or a key in it was refused (I18N-009).
	plxerr.TranslationImportInvalid:       connect.CodeInvalidArgument,
	plxerr.TranslationImportKeyUnknown:    connect.CodeInvalidArgument,
	plxerr.TranslationImportSourceChanged: connect.CodeInvalidArgument,

	// P9: policies, control, rollouts, federation, governance and licence.
	plxerr.PolicyDocumentInvalid:           connect.CodeInvalidArgument,
	plxerr.UnknownFlagType:                 connect.CodeInvalidArgument,
	plxerr.VariantWithoutControl:           connect.CodeInvalidArgument,
	plxerr.VariantWeightsInvalid:           connect.CodeInvalidArgument,
	plxerr.SegmentRuleInvalid:              connect.CodeInvalidArgument,
	plxerr.ExperimentKeyUnknown:            connect.CodeInvalidArgument,
	plxerr.WinnerNotAVariant:               connect.CodeInvalidArgument,
	plxerr.VariantTreeExceedsBudget:        connect.CodeFailedPrecondition,
	plxerr.ControlSignatureInvalid:         connect.CodeUnauthenticated,
	plxerr.ControlDocumentExpired:          connect.CodeFailedPrecondition,
	plxerr.ControlRollbackRejected:         connect.CodeFailedPrecondition,
	plxerr.MandatoryUpdatePending:          connect.CodeFailedPrecondition,
	plxerr.ForcedRollbackApplied:           connect.CodeFailedPrecondition,
	plxerr.RolloutPausedByHealthGate:       connect.CodeFailedPrecondition,
	plxerr.RolloutRolledBack:               connect.CodeFailedPrecondition,
	plxerr.TargetingRuleInvalid:            connect.CodeInvalidArgument,
	plxerr.ScheduleOverlapsRollout:         connect.CodeFailedPrecondition,
	plxerr.StepUpRequired:                  connect.CodePermissionDenied,
	plxerr.StepUpExpired:                   connect.CodePermissionDenied,
	plxerr.WebAuthnAssertionInvalid:        connect.CodeUnauthenticated,
	plxerr.SAMLResponseInvalid:             connect.CodeUnauthenticated,
	plxerr.SAMLAssertionReplayed:           connect.CodeUnauthenticated,
	plxerr.SCIMRequestInvalid:              connect.CodeInvalidArgument,
	plxerr.SessionRevoked:                  connect.CodeUnauthenticated,
	plxerr.SessionIdleTimeout:              connect.CodeUnauthenticated,
	plxerr.SessionAbsoluteTimeout:          connect.CodeUnauthenticated,
	plxerr.CronExpressionInvalid:           connect.CodeInvalidArgument,
	plxerr.WebhookSignatureInvalid:         connect.CodeUnauthenticated,
	plxerr.WebhookReplayed:                 connect.CodeUnauthenticated,
	plxerr.ApprovalRequired:                connect.CodeFailedPrecondition,
	plxerr.ApprovalInvalidated:             connect.CodeFailedPrecondition,
	plxerr.ApproverNotEligible:             connect.CodePermissionDenied,
	plxerr.ApprovalExpired:                 connect.CodeFailedPrecondition,
	plxerr.BreakGlassJustificationRequired: connect.CodeInvalidArgument,
	plxerr.PostHocReviewOverdue:            connect.CodeFailedPrecondition,
	plxerr.ChangeFreezeActive:              connect.CodeFailedPrecondition,
	plxerr.RoleUnknown:                     connect.CodeNotFound,
	plxerr.PermissionNotInCatalogue:        connect.CodeInvalidArgument,
	plxerr.ScopeInvalid:                    connect.CodeInvalidArgument,
	plxerr.LastOwnerCannotBeRemoved:        connect.CodeFailedPrecondition,
	plxerr.RestrictionPatternInvalid:       connect.CodeInvalidArgument,
	plxerr.RestrictionLocksOutOwner:        connect.CodeFailedPrecondition,
	plxerr.RestrictionExceedsCaller:        connect.CodePermissionDenied,
	plxerr.RestrictionLimitReached:         connect.CodeResourceExhausted,
	plxerr.ExperimentOverlapsLayer:         connect.CodeFailedPrecondition,
	plxerr.SampleSizeNotReached:            connect.CodeFailedPrecondition,
	plxerr.SampleRatioMismatch:             connect.CodeFailedPrecondition,
	plxerr.FlagTypeChangeRefused:           connect.CodeFailedPrecondition,
	plxerr.IngestionOverQuota:              connect.CodeResourceExhausted,
	plxerr.RetentionChangeScheduled:        connect.CodeFailedPrecondition,
	plxerr.ExportDestinationUnreachable:    connect.CodeUnavailable,
	plxerr.ChannelMisconfigured:            connect.CodeFailedPrecondition,
	plxerr.DeliveryFailed:                  connect.CodeUnavailable,
	plxerr.SIEMDestinationUnreachable:      connect.CodeUnavailable,
	plxerr.SubjectNotFound:                 connect.CodeNotFound,
	plxerr.ReconstructionDataExpired:       connect.CodeFailedPrecondition,
	plxerr.GitRemoteRefused:                connect.CodeFailedPrecondition,
	plxerr.EnterpriseFeatureNotLicensed:    connect.CodePermissionDenied,
	plxerr.LicenseInvalid:                  connect.CodePermissionDenied,
	plxerr.LicenseExpired:                  connect.CodePermissionDenied,
	plxerr.LicenseLimitExceeded:            connect.CodePermissionDenied,

	// The server failed.
	plxerr.InternalServerError: connect.CodeInternal,
	plxerr.UpstreamUnavailable: connect.CodeUnavailable,
}

// ConnectCode returns the Connect code a Plux code is reported with.
// An unmapped code is internal: a failure the edge does not know how to
// describe is not described at all.
func ConnectCode(c plxerr.Code) connect.Code {
	if code, ok := codes[c]; ok {
		return code
	}
	return connect.CodeInternal
}

// Error translates a domain error into a Connect error carrying the
// Connect code, google.rpc.ErrorInfo and the Plux code (SRV-006).
//
// incident identifies an unrecognised failure in the server's logs; the
// response carries the identifier and nothing else about it, so an
// internal message can never leak.
func Error(err error, incident string) error {
	if err == nil {
		return nil
	}
	out := toConnect(err, incident)
	var carrier *withHeader
	if errors.As(err, &carrier) {
		var ce *connect.Error
		if errors.As(out, &ce) {
			for name, values := range carrier.header {
				for _, v := range values {
					ce.Meta().Add(name, v)
				}
			}
		}
	}
	return out
}

// withHeader is an error that also carries response headers, such as the
// configuration epoch every gateway response has (DAT-095).
type withHeader struct {
	err    error
	header http.Header
}

func (e *withHeader) Error() string { return e.err.Error() }
func (e *withHeader) Unwrap() error { return e.err }

// toConnect turns a domain error into a Connect error.
func toConnect(err error, incident string) error {
	var connErr *connect.Error
	if errors.As(err, &connErr) {
		return connErr
	}
	var plx *plxerr.Error
	if !errors.As(err, &plx) {
		return internal(incident)
	}
	code := ConnectCode(plx.Code)
	if code == connect.CodeInternal {
		return internal(incident)
	}
	out := connect.NewError(code, fmt.Errorf("%s: %s", plx.Code, plx.Message))
	if detail := info(plx.Code, plx.Reason(), plx.Details); detail != nil {
		out.AddDetail(detail)
	}
	return out
}

// internal returns the one error an unrecognised failure becomes.
func internal(incident string) error {
	out := connect.NewError(connect.CodeInternal,
		fmt.Errorf("%s: internal error, incident %s", plxerr.InternalServerError, incident))
	if detail := info(plxerr.InternalServerError, "INTERNAL_SERVER_ERROR", map[string]string{"incident": incident}); detail != nil {
		out.AddDetail(detail)
	}
	return out
}

// info builds the ErrorInfo detail. A detail that cannot be encoded is
// dropped rather than replacing the error with one about encoding.
func info(code plxerr.Code, reason plxerr.Reason, details map[string]string) *connect.ErrorDetail {
	metadata := map[string]string{"code": code.String(), "docURL": code.DocURL()}
	for k, v := range details {
		metadata[k] = v
	}
	detail, err := connect.NewErrorDetail(&errdetails.ErrorInfo{
		Reason:   string(reason),
		Domain:   Domain,
		Metadata: metadata,
	})
	if err != nil {
		return nil
	}
	return detail
}
