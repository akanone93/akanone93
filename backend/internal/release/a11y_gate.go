// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package release

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/security/settings"
)

// maxGateFindings bounds how many findings the message of PLX-3060 lists;
// the diagnostics beside it name every one.
const maxGateFindings = 5

// isA11yCode reports whether a code is one of the accessibility checks
// (PLX-1401 to PLX-1408; PLX-1409 only says a colour was not checked).
func isA11yCode(c plxerr.Code) bool {
	return c >= plxerr.AccessibleNameMissing && c <= plxerr.LiveRegionStatic
}

// blocksAccessibility is the environment's accessibilityBlocksPublish
// setting (A11Y-002, SEC-180). Without a security configuration service
// it is the standard profile's false.
func (s *Service) blocksAccessibility(ctx context.Context, appID, environmentID string) (bool, error) {
	if s.o.SecurityConfig == nil {
		return false, nil
	}
	values, _, err := s.o.SecurityConfig.Effective(ctx, appID, environmentID)
	if err != nil {
		return false, fmt.Errorf("release: %w", err)
	}
	return values.Get(settings.AccessibilityBlocksPublish).Bool(), nil
}

// gateAccessibility applies accessibilityBlocksPublish to the diagnostics
// of a compilation (A11Y-002). The compiler reports an unwaived finding of
// the strict and maximum profiles as an error; when the setting is on, any
// such error refuses the publish with PLX-3060 beside the findings (they
// are already unwaived: a waived finding is no diagnostic). The returned
// diagnostics are a copy when they change.
func (s *Service) gateAccessibility(ctx context.Context, appID, environmentID string, diags plxerr.Diagnostics) (plxerr.Diagnostics, error) {
	var findings []plxerr.Diagnostic
	for _, d := range diags {
		if isA11yCode(d.Code) && d.Severity == plxerr.SeverityError {
			findings = append(findings, d)
		}
	}
	if len(findings) == 0 {
		return diags, nil
	}
	blocks, err := s.blocksAccessibility(ctx, appID, environmentID)
	if err != nil || !blocks {
		return diags, err
	}
	listed := make([]string, 0, maxGateFindings)
	for _, d := range findings[:min(len(findings), maxGateFindings)] {
		listed = append(listed, fmt.Sprintf("%s at %s%s", d.Code, d.File, d.Path))
	}
	more := ""
	if len(findings) > len(listed) {
		more = fmt.Sprintf(" and %d more", len(findings)-len(listed))
	}
	return append(slices.Clone(diags), plxerr.NewDiagnostic(plxerr.AccessibilityBlocksPublish, plxerr.Location{},
		"%d unwaived accessibility error(s) block the publish: %s%s", len(findings), strings.Join(listed, "; "), more)), nil
}
