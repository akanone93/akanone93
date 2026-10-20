// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package controldoc

import (
	"fmt"

	flatbuffers "github.com/google/flatbuffers/go"

	"github.com/nightCode42/plux3/backend/internal/controldoc/controlfbs"
)

// initialSize is the builder's first allocation; a small document fits.
const initialSize = 512

// wireScopes maps a scope to its place in the wire enumeration.
var wireScopes = map[Scope]controlfbs.SwitchScope{
	ScopePlugin:      controlfbs.SwitchScopePlugin,
	ScopePage:        controlfbs.SwitchScopePage,
	ScopeActionGraph: controlfbs.SwitchScopeActionGraph,
	ScopeDataSource:  controlfbs.SwitchScopeDataSource,
	ScopeFunction:    controlfbs.SwitchScopeFunction,
}

// Encode returns the FlatBuffers buffer of a document, with the file
// identifier PLXC. The document is validated first; the kill switches are
// written sorted, so the same document always has the same bytes
// (CMP-002). The reserved flags and experiments vectors are written empty.
func Encode(d Document) ([]byte, error) {
	if err := d.Validate(); err != nil {
		return nil, fmt.Errorf("controldoc.Encode: %w", err)
	}
	b := flatbuffers.NewBuilder(initialSize)
	appID, environment, channel := b.CreateString(d.AppID), b.CreateString(d.Environment), b.CreateString(d.Channel)
	switches := writeSwitches(b, sortedSwitches(d.KillSwitches))
	var mandatory, rollback flatbuffers.UOffsetT
	if m := d.MandatoryUpdate; m != nil {
		mandatory = writeMandatory(b, *m)
	}
	if r := d.ForcedRollback; r != nil {
		rollback = writeRollback(b, *r)
	}
	controlfbs.ControlDocumentStartFlagsVector(b, 0)
	flags := b.EndVector(0)
	controlfbs.ControlDocumentStartExperimentsVector(b, 0)
	experiments := b.EndVector(0)

	controlfbs.ControlDocumentStart(b)
	controlfbs.ControlDocumentAddVersion(b, d.Version)
	controlfbs.ControlDocumentAddAppId(b, appID)
	controlfbs.ControlDocumentAddEnvironment(b, environment)
	controlfbs.ControlDocumentAddChannel(b, channel)
	controlfbs.ControlDocumentAddIssuedAt(b, d.IssuedAt.Unix())
	controlfbs.ControlDocumentAddExpiresAt(b, d.ExpiresAt.Unix())
	controlfbs.ControlDocumentAddKillSwitches(b, switches)
	if mandatory != 0 {
		controlfbs.ControlDocumentAddMandatoryUpdate(b, mandatory)
	}
	if rollback != 0 {
		controlfbs.ControlDocumentAddForcedRollback(b, rollback)
	}
	controlfbs.ControlDocumentAddFlags(b, flags)
	controlfbs.ControlDocumentAddExperiments(b, experiments)
	controlfbs.FinishControlDocumentBuffer(b, controlfbs.ControlDocumentEnd(b))
	return b.FinishedBytes(), nil
}

// writeSwitches writes the kill switch vector in the order given.
func writeSwitches(b *flatbuffers.Builder, switches []KillSwitch) flatbuffers.UOffsetT {
	targets := make([]flatbuffers.UOffsetT, len(switches))
	for i, k := range switches {
		targets[i] = b.CreateString(k.Target)
	}
	tables := make([]flatbuffers.UOffsetT, len(switches))
	for i, k := range switches {
		controlfbs.KillSwitchStart(b)
		controlfbs.KillSwitchAddScope(b, wireScopes[k.Scope])
		controlfbs.KillSwitchAddTarget(b, targets[i])
		tables[i] = controlfbs.KillSwitchEnd(b)
	}
	controlfbs.ControlDocumentStartKillSwitchesVector(b, len(tables))
	for i := len(tables) - 1; i >= 0; i-- {
		b.PrependUOffsetT(tables[i])
	}
	return b.EndVector(len(tables))
}

// writeMandatory writes the mandatory update table.
func writeMandatory(b *flatbuffers.Builder, m MandatoryUpdate) flatbuffers.UOffsetT {
	id := b.CreateString(m.ReleaseID)
	var key flatbuffers.UOffsetT
	if m.MessageKey != "" {
		key = b.CreateString(m.MessageKey)
	}
	controlfbs.MandatoryUpdateStart(b)
	controlfbs.MandatoryUpdateAddReleaseId(b, id)
	controlfbs.MandatoryUpdateAddReleaseSequence(b, m.ReleaseSequence)
	if key != 0 {
		controlfbs.MandatoryUpdateAddMessageKey(b, key)
	}
	return controlfbs.MandatoryUpdateEnd(b)
}

// writeRollback writes the forced rollback table.
func writeRollback(b *flatbuffers.Builder, r ForcedRollback) flatbuffers.UOffsetT {
	id := b.CreateString(r.ReleaseID)
	controlfbs.ForcedRollbackStart(b)
	controlfbs.ForcedRollbackAddReleaseId(b, id)
	controlfbs.ForcedRollbackAddReleaseSequence(b, r.ReleaseSequence)
	return controlfbs.ForcedRollbackEnd(b)
}
