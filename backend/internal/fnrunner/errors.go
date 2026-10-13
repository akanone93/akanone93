// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package fnrunner

import (
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"google.golang.org/genproto/googleapis/rpc/errdetails"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// errorDomain is the domain of the ErrorInfo detail, the same as the public
// API's (SRV-006), so the api role reads both alike.
const errorDomain = "plux.dev"

// connectCodes maps the Plux codes this role produces to the Connect code
// that carries them. The api role reads the Plux code from the ErrorInfo
// detail; the Connect code is the coarse class. A code not listed is
// internal.
var connectCodes = map[plxerr.Code]connect.Code{
	plxerr.RateLimited:                      connect.CodeResourceExhausted,
	plxerr.FunctionTimeout:                  connect.CodeDeadlineExceeded,
	plxerr.FunctionMemoryExceeded:           connect.CodeResourceExhausted,
	plxerr.FunctionCapabilityDenied:         connect.CodePermissionDenied,
	plxerr.FunctionOutputTooLarge:           connect.CodeResourceExhausted,
	plxerr.FunctionCallDepthExceeded:        connect.CodeResourceExhausted,
	plxerr.FunctionHTTPCallsExceeded:        connect.CodeResourceExhausted,
	plxerr.FunctionTrapped:                  connect.CodeAborted,
	plxerr.FunctionInputInvalid:             connect.CodeAborted,
	plxerr.FunctionOutputInvalid:            connect.CodeAborted,
	plxerr.FunctionNotFound:                 connect.CodeNotFound,
	plxerr.FunctionABIUnsupported:           connect.CodeFailedPrecondition,
	plxerr.FunctionModuleInvalid:            connect.CodeFailedPrecondition,
	plxerr.FunctionArtifactSignatureInvalid: connect.CodeDataLoss,
	plxerr.FunctionCancelled:                connect.CodeCanceled,
	plxerr.FunctionInputTooLarge:            connect.CodeInvalidArgument,
	plxerr.InvalidStructure:                 connect.CodeInvalidArgument,
	plxerr.PermissionDenied:                 connect.CodePermissionDenied,
	plxerr.UpstreamUnavailable:              connect.CodeUnavailable,
}

// toConnect turns an error of this role into a Connect error that carries
// the Plux code in its message and in a google.rpc.ErrorInfo detail. An
// error without a Plux code becomes an internal error: its text stays
// in the log, not in the response.
func toConnect(err error) error {
	var plx *plxerr.Error
	if !errors.As(err, &plx) {
		return connect.NewError(connect.CodeInternal,
			fmt.Errorf("%s: internal error", plxerr.InternalServerError))
	}
	code, ok := connectCodes[plx.Code]
	if !ok {
		code = connect.CodeInternal
	}
	out := connect.NewError(code, fmt.Errorf("%s: %s", plx.Code, plx.Message))
	metadata := map[string]string{"code": plx.Code.String(), "docURL": plx.Code.DocURL()}
	for k, v := range plx.Details {
		metadata[k] = v
	}
	if detail, derr := connect.NewErrorDetail(&errdetails.ErrorInfo{
		Reason: string(plx.Reason()), Domain: errorDomain, Metadata: metadata,
	}); derr == nil {
		out.AddDetail(detail)
	}
	return out
}

// codeOf returns the Plux code of err, or InternalServerError.
func codeOf(err error) plxerr.Code {
	if c, ok := plxerr.CodeOf(err); ok {
		return c
	}
	return plxerr.InternalServerError
}
