// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package archive

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/nightCode42/plux3/backend/internal/audit"
	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/document"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/tenancy"
)

// Tenancy is what the service needs of the tenancy service.
type Tenancy interface {
	GetApp(ctx context.Context, p auth.Principal, appID string) (tenancy.App, error)
	CreateApp(ctx context.Context, p auth.Principal, key, name string) (tenancy.App, error)
	UpdateAppMetadata(ctx context.Context, p auth.Principal, appID string, m tenancy.Metadata) (tenancy.App, error)
	DeleteApp(ctx context.Context, p auth.Principal, appID string) (tenancy.TrashItem, error)
	PurgeFromTrash(ctx context.Context, p auth.Principal, itemID string) error
}

// Documents is what the service needs of the document service.
type Documents interface {
	Export(ctx context.Context, p auth.Principal, appID, pluginID string) ([]document.File, error)
	Import(ctx context.Context, p auth.Principal, appID, pluginID, session string, files []document.File) (document.Written, error)
	ListPlugins(ctx context.Context, p auth.Principal, appID string, after storage.Cursor, size int32) ([]document.Plugin, error)
	ReleaseLock(ctx context.Context, p auth.Principal, appID, pluginID, session string) error
}

// IDs generates identifiers; here, the importing session's name.
type IDs interface{ New() (string, error) }

// Options are what the service needs.
type Options struct {
	DB        *storage.DB
	Audit     *audit.Log
	Tenancy   Tenancy
	Documents Documents
	IDs       IDs
	// Limits are the installation's limits (LIM-001); zero means the defaults.
	Limits limits.Set
	// Samples are the sample apps; nil uses those embedded in the binary.
	Samples []Sample
}

// Service exports, imports and duplicates apps (STU-023, STU-024) and
// offers the sample apps (STU-015).
type Service struct{ o Options }

// New returns a service.
func New(o Options) (*Service, error) {
	if o.DB == nil || o.Audit == nil || o.Tenancy == nil || o.Documents == nil || o.IDs == nil {
		return nil, plxerr.New(plxerr.InvalidFormat, "archive: the database, audit log, tenancy, documents and IDs are required")
	}
	if o.Limits.IsZero() {
		o.Limits = limits.Defaults()
	}
	if o.Samples == nil {
		samples, err := Embedded()
		if err != nil {
			return nil, err
		}
		o.Samples = samples
	}
	return &Service{o: o}, nil
}

// Exported is an archive with its name and hash.
type Exported struct {
	Filename string
	Data     []byte
	SHA256   string
}

// Imported is the outcome of an import or a duplicate.
type Imported struct {
	App       tenancy.App
	Documents int
	// ErrorDiagnostics counts the validation errors the project still has.
	ErrorDiagnostics int
}

// Export returns an app's drafts as an archive. It needs plugin.read on the
// app, and is audited.
func (s *Service) Export(ctx context.Context, p auth.Principal, appID string) (Exported, error) {
	app, err := s.o.Tenancy.GetApp(ctx, p, appID)
	if err != nil {
		return Exported{}, fmt.Errorf("archive: %w", err)
	}
	files, err := s.o.Documents.Export(ctx, p, appID, "")
	if err != nil {
		return Exported{}, fmt.Errorf("archive: %w", err)
	}
	data, err := Pack(AppInfo{Key: app.Key, Name: app.Name, Description: app.Description, Tags: app.Tags}, files)
	if err != nil {
		return Exported{}, err
	}
	out := Exported{Filename: app.Key + ".plux", Data: data, SHA256: sum(data)}
	return out, s.record(ctx, p, audit.AppExported, appID, out.SHA256)
}

// Import creates an app from an archive, which is untrusted input. key and
// name override the archive's when not empty. A failure after the app was
// created removes it again.
func (s *Service) Import(ctx context.Context, p auth.Principal, data []byte, key, name string) (Imported, error) {
	return s.importAs(ctx, p, data, key, name, audit.AppImported, "")
}

// Duplicate creates an app from an export of another, through the same path
// as an import.
func (s *Service) Duplicate(ctx context.Context, p auth.Principal, appID, key, name string) (Imported, error) {
	if err := authorize(p); err != nil {
		return Imported{}, err
	}
	src, err := s.Export(ctx, p, appID)
	if err != nil {
		return Imported{}, err
	}
	app, err := s.o.Tenancy.GetApp(ctx, p, appID)
	if err != nil {
		return Imported{}, fmt.Errorf("archive: %w", err)
	}
	if key == "" {
		key = strings.TrimRight(truncate(app.Key+"-copy", 63), "-")
	}
	if name == "" {
		name = truncate(app.Name+" (copy)", 64)
	}
	return s.importAs(ctx, p, src.Data, key, name, audit.AppDuplicated, appID)
}

// ImportSample imports a sample app as an ordinary import (F17).
func (s *Service) ImportSample(ctx context.Context, p auth.Principal, id, key, name string) (Imported, error) {
	if err := authorize(p); err != nil {
		return Imported{}, err
	}
	for _, sample := range s.o.Samples {
		if sample.ID == id {
			return s.importAs(ctx, p, sample.Data, key, name, audit.AppImported, "sample:"+id)
		}
	}
	return Imported{}, plxerr.New(plxerr.ResourceNotFound, "no such sample app")
}

// ListSamples lists the sample apps in id order.
func (s *Service) ListSamples() []Sample { return s.o.Samples }

func (s *Service) importAs(ctx context.Context, p auth.Principal, data []byte, key, name string, action audit.Action, source string) (out Imported, err error) {
	if err = authorize(p); err != nil {
		return Imported{}, err
	}
	m, files, err := Unpack(data, LimitsOf(s.o.Limits))
	if err != nil {
		return Imported{}, err
	}
	if key == "" {
		key = m.App.Key
	}
	if name == "" {
		name = m.App.Name
	}
	app, err := s.o.Tenancy.CreateApp(ctx, p, key, name)
	if err != nil {
		return Imported{}, fmt.Errorf("archive: %w", err)
	}
	defer func() {
		if err != nil {
			s.discard(ctx, p, app.ID)
		}
	}()
	desc := m.App.Description
	if _, err = s.o.Tenancy.UpdateAppMetadata(ctx, p, app.ID, tenancy.Metadata{Description: &desc, SetTags: true, Tags: m.App.Tags}); err != nil {
		return Imported{}, fmt.Errorf("archive: %w", err)
	}
	session, err := s.o.IDs.New()
	if err != nil {
		return Imported{}, fmt.Errorf("archive: session: %w", err)
	}
	session = "import-" + session
	written, err := s.o.Documents.Import(ctx, p, app.ID, "", session, files)
	if err != nil {
		return Imported{}, fmt.Errorf("archive: %w", err)
	}
	s.release(ctx, p, app.ID, session)
	out = Imported{Documents: len(written.Documents), ErrorDiagnostics: countErrors(written.Diagnostics)}
	if out.App, err = s.o.Tenancy.GetApp(ctx, p, app.ID); err != nil {
		return Imported{}, fmt.Errorf("archive: %w", err)
	}
	detail := sum(data)
	if source != "" {
		detail = source + " " + detail
	}
	if err = s.record(ctx, p, action, app.ID, detail); err != nil {
		return Imported{}, err
	}
	return out, nil
}

// release lets go of the locks the import took, so that the person who
// imported can edit at once.
func (s *Service) release(ctx context.Context, p auth.Principal, appID, session string) {
	_ = s.o.Documents.ReleaseLock(ctx, p, appID, "", session)
	plugins, err := s.o.Documents.ListPlugins(ctx, p, appID, storage.Cursor{}, int32(s.o.Limits.Get(limits.AppPlugins))) //nolint:gosec // a registry limit
	if err != nil {
		return
	}
	for _, pl := range plugins {
		_ = s.o.Documents.ReleaseLock(ctx, p, appID, pl.ID, session)
	}
}

// discard removes an app whose import failed, from the trash too.
func (s *Service) discard(ctx context.Context, p auth.Principal, appID string) {
	item, err := s.o.Tenancy.DeleteApp(ctx, p, appID)
	if err != nil {
		return
	}
	_ = s.o.Tenancy.PurgeFromTrash(ctx, p, item.ID)
}

func (s *Service) record(ctx context.Context, p auth.Principal, action audit.Action, appID, detail string) error {
	err := s.o.DB.InTx(ctx, storage.Tenant{OrganizationID: p.OrganizationID, UserID: p.UserID}, func(ctx context.Context, tx pgx.Tx) error {
		_, err := s.o.Audit.Append(ctx, tx, audit.Entry{
			OrganizationID: p.OrganizationID, Actor: p.Actor(), Action: action, TargetKind: "app", TargetID: appID, Detail: detail,
		})
		return err //nolint:wrapcheck // the audit package wraps its own failures
	})
	if err != nil {
		return fmt.Errorf("archive: audit: %w", err)
	}
	return nil
}

// authorize refuses a caller who may not create apps, before any input is
// looked at: an untrusted archive is not parsed for someone who cannot import.
func authorize(p auth.Principal) error {
	if err := p.Authorize(auth.AppManage); err != nil {
		return fmt.Errorf("archive: %w", err)
	}
	return nil
}

func countErrors(d plxerr.Diagnostics) int {
	n := 0
	for _, x := range d {
		if x.Severity == plxerr.SeverityError {
			n++
		}
	}
	return n
}

func sum(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
