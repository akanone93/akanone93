// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package compiler

import (
	"slices"
	"strconv"
	"strings"

	"github.com/nightCode42/plux3/backend/internal/bundle"
)

// EngineFeature is a feature of the runtime's engines a bundle can list in
// required_features (BND-008), and the runtime version that first supports
// it; Runtime is empty for a marker the compiler lists whatever the app's
// minimum runtime.
type EngineFeature struct {
	Name    string `json:"name"`
	Runtime string `json:"runtime,omitempty"`
}

// engineFeatureTables are the engine features the checks require, each with
// its runtimes by revision (useRevision): the same tables the checks use, so
// a feature is listed here by being defined.
var engineFeatureTables = []struct {
	name     string
	runtimes []string
}{
	{animFeature, animRuntimes},
	{animTimelinesFeature, animRuntimes},
	{animTransitionsFeature, animRuntimes},
	{dbFeature, dbRuntimes},
	{dataFeature, dataRuntimes},
	{streamsFeature, ioRuntimes},
	{outboxFeature, ioRuntimes},
	{transfersFeature, ioRuntimes},
	{formsFeature, formsRuntimes},
	{formSensitiveFeature, formSensitiveRuntimes},
	{guardsFeature, guardRuntimes},
	{secureUIFeature, secureUIRuntimes},
	{stateWriteFeature, stateRuntimes},
	{stateComputedFeature, stateRuntimes},
	{statePersistenceFeature, stateRuntimes},
	{variantFeature, variantRuntimes},
	// The action engine's features, required through requireFeature.
	{triggersFeature, actionsRuntimes},
	{concurrencyFeature, actionsRuntimes},
	{retryFeature, actionsRuntimes},
	{flowsFeature, actionsRuntimes},
	{controlFeature, actionsRuntimes},
	{deviceFeature, actionsRuntimes},
}

// EngineFeatures lists every engine feature the compiler can require of a
// runtime, sorted by name (#271): the feature tables above, the PXL groups
// (pxlFeatureRuntimes) and the markers every bundle that needs them lists.
// Registry revisions (widget.*, type.*, enum.*) are not among them: the
// runtime derives those from the same generated registry. The list is
// written to schema/testdata/features/engine.json, which the runtime's tests
// read, so a feature the compiler can require and the runtime does not
// declare fails a test instead of a device (#257).
func EngineFeatures() []EngineFeature {
	var out []EngineFeature
	seen := map[string]bool{}
	add := func(f EngineFeature) {
		if !seen[f.Name] {
			seen[f.Name] = true
			out = append(out, f)
		}
	}
	for _, t := range engineFeatureTables {
		for i, rt := range t.runtimes {
			add(EngineFeature{Name: t.name + ".v" + strconv.Itoa(i+1), Runtime: rt})
		}
	}
	for name, runtimes := range pxlFeatureRuntimes {
		add(EngineFeature{Name: name, Runtime: runtimes[0]})
	}
	for _, marker := range []string{"pxl.v1", bundle.FeatureWasm, bundle.FeatureConfidential} {
		add(EngineFeature{Name: marker})
	}
	slices.SortFunc(out, func(a, b EngineFeature) int { return strings.Compare(a.Name, b.Name) })
	return out
}
