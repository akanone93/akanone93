// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Package preferences keeps each signed-in user's Studio preferences, one
// set per organisation (STU-014, STU-058), and derives the organisation's
// onboarding steps from what it has done (STU-015).
//
// The preferences are stored as one JSON document per user and organisation;
// row-level security confines every statement to the organisation of the
// call, and every statement names the caller's own user, so no user reads or
// writes another's preferences.
package preferences

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"slices"

	"github.com/jackc/pgx/v5"
	"golang.org/x/text/language"

	"github.com/nightCode42/plux3/backend/internal/auth"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
	"github.com/nightCode42/plux3/backend/internal/schema/limits"
	"github.com/nightCode42/plux3/backend/internal/storage"
	"github.com/nightCode42/plux3/backend/internal/storage/dbgen"
	"github.com/nightCode42/plux3/backend/internal/tenancy"
)

// The themes and densities a user may choose.
const (
	ThemeSystem        = "system"
	ThemeLight         = "light"
	ThemeDark          = "dark"
	DensityComfortable = "comfortable"
	DensityCompact     = "compact"
	PlatformMaterial   = "material"
	PlatformCupertino  = "cupertino"
)

// The field names an update mask may hold: the field names of
// plux.v1.Preferences.
const (
	FieldTheme               = "theme"
	FieldLanguage            = "language"
	FieldDensity             = "density"
	FieldShortcuts           = "shortcuts"
	FieldFavouriteAppIDs     = "favourite_app_ids"
	FieldPinned              = "pinned"
	FieldPreview             = "preview"
	FieldOnboardingDismissed = "onboarding_dismissed"
)

// The onboarding steps, in the order Studio shows them (STU-015).
const (
	StepCreateApp      = "create_app"
	StepEditScreen     = "edit_screen"
	StepPairDevice     = "pair_device"
	StepPublishRelease = "publish_release"
	StepInviteMember   = "invite_member"
)

// Preferences are one user's Studio preferences in one organisation. The
// JSON form is what is stored.
type Preferences struct {
	Theme               string                     `json:"theme,omitempty"`
	Language            string                     `json:"language,omitempty"`
	Density             string                     `json:"density,omitempty"`
	Shortcuts           map[string]string          `json:"shortcuts,omitempty"`
	FavouriteAppIDs     []string                   `json:"favouriteAppIds,omitempty"`
	Pinned              []string                   `json:"pinned,omitempty"`
	Preview             map[string]PreviewSwitches `json:"preview,omitempty"`
	OnboardingDismissed bool                       `json:"onboardingDismissed,omitempty"`
}

// PreviewSwitches are the switches of one app's live preview (STU-058).
type PreviewSwitches struct {
	DeviceFrame  string            `json:"deviceFrame,omitempty"`
	Theme        string            `json:"theme,omitempty"`
	BrandOverlay string            `json:"brandOverlay,omitempty"`
	Locale       string            `json:"locale,omitempty"`
	TextScale    float64           `json:"textScale,omitempty"`
	Platform     string            `json:"platform,omitempty"`
	MockStates   map[string]string `json:"mockStates,omitempty"`
}

// Step is one onboarding step and whether the organisation has done it.
type Step struct {
	Key  string
	Done bool
}

// Options are what the service needs.
type Options struct {
	DB *storage.DB
	// Tenancy supplies the limits in force for the organisation.
	Tenancy *tenancy.Service
}

// Service reads and writes the caller's preferences.
type Service struct{ o Options }

// NewService returns the service.
func NewService(o Options) (*Service, error) {
	if o.DB == nil || o.Tenancy == nil {
		return nil, errors.New("preferences: a database and tenancy are required")
	}
	return &Service{o: o}, nil
}

// Defaults are the preferences of a user who never saved any.
func Defaults() Preferences {
	return Preferences{Theme: ThemeSystem, Density: DensityComfortable}
}

// Get returns the principal's preferences in the organisation of the call.
func (s *Service) Get(ctx context.Context, p auth.Principal) (Preferences, error) {
	key, err := owner(p)
	if err != nil {
		return Preferences{}, err
	}
	var out Preferences
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		out, err = load(ctx, dbgen.New(tx), key)
		return err
	})
	return out, err
}

// Update replaces the fields of the principal's preferences that mask names
// with those of in, and returns the result. The mask must name at least one
// field, and only fields of Preferences.
func (s *Service) Update(ctx context.Context, p auth.Principal, in Preferences, mask []string) (Preferences, error) {
	key, err := owner(p)
	if err != nil {
		return Preferences{}, err
	}
	if len(mask) == 0 {
		return Preferences{}, plxerr.New(plxerr.MissingProperty, "update_mask must name the fields to replace")
	}
	for _, f := range mask {
		if !known(f) {
			return Preferences{}, plxerr.New(plxerr.UnknownProperty, "update_mask names %q, which is not a field of Preferences", f)
		}
	}
	var out Preferences
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		q := dbgen.New(tx)
		cur, err := load(ctx, q, key)
		if err != nil {
			return err
		}
		next := apply(cur, in, mask)
		lim, err := s.o.Tenancy.Effective(ctx, tx, p.OrganizationID, "", "")
		if err != nil {
			return err //nolint:wrapcheck // failure wraps it
		}
		if err := validate(&next, lim); err != nil {
			return err
		}
		doc, err := json.Marshal(next)
		if err != nil {
			return fmt.Errorf("preferences: %w", err)
		}
		if most := lim.Get(limits.StudioPreferencesBytes); int64(len(doc)) > most {
			return plxerr.New(plxerr.OutOfRange, "the preferences take %d bytes, more than studio.preferencesBytes (%d)", len(doc), most)
		}
		if err := q.UpsertUserPreferences(ctx, dbgen.UpsertUserPreferencesParams{
			OrganizationID: key.OrganizationID, UserID: key.UserID, Preferences: doc,
		}); err != nil {
			return err //nolint:wrapcheck // failure wraps it
		}
		out = withDefaults(next)
		return nil
	})
	return out, err
}

// Onboarding returns the organisation's onboarding steps, in order, each
// done once the organisation has: an app; a document written; a device
// paired with a dev session; a release published; and a second member.
func (s *Service) Onboarding(ctx context.Context, p auth.Principal) ([]Step, error) {
	if p.OrganizationID == "" {
		return nil, plxerr.New(plxerr.MissingProperty, "onboarding belongs to an organisation; name one")
	}
	org, err := storage.UUID(p.OrganizationID)
	if err != nil {
		return nil, plxerr.New(plxerr.InvalidFormat, "the organisation identifier is not a UUID")
	}
	var facts dbgen.OnboardingFactsRow
	err = s.inOrg(ctx, p, func(ctx context.Context, tx pgx.Tx) error {
		var qerr error
		facts, qerr = dbgen.New(tx).OnboardingFacts(ctx, org)
		return qerr //nolint:wrapcheck // failure wraps it
	})
	if err != nil {
		return nil, fmt.Errorf("preferences: onboarding: %w", err)
	}
	return []Step{
		{Key: StepCreateApp, Done: facts.HasApp},
		{Key: StepEditScreen, Done: facts.HasDocument},
		{Key: StepPairDevice, Done: facts.HasDevice},
		{Key: StepPublishRelease, Done: facts.HasRelease},
		{Key: StepInviteMember, Done: facts.HasMembers},
	}, nil
}

// known reports whether an update mask may name a field.
func known(field string) bool {
	switch field {
	case FieldTheme, FieldLanguage, FieldDensity, FieldShortcuts, FieldFavouriteAppIDs, FieldPinned, FieldPreview, FieldOnboardingDismissed:
		return true
	}
	return false
}

func (s *Service) inOrg(ctx context.Context, p auth.Principal, f func(context.Context, pgx.Tx) error) error {
	return failure(s.o.DB.InTx(ctx, storage.Tenant{OrganizationID: p.OrganizationID, UserID: p.UserID}, f))
}

// owner returns the row key of the principal's preferences: preferences
// belong to a signed-in user in an organisation.
func owner(p auth.Principal) (dbgen.GetUserPreferencesParams, error) {
	if p.UserID == "" {
		return dbgen.GetUserPreferencesParams{}, plxerr.New(plxerr.AuthenticationRequired, "preferences belong to a signed-in user")
	}
	if p.OrganizationID == "" {
		return dbgen.GetUserPreferencesParams{}, plxerr.New(plxerr.MissingProperty, "preferences belong to an organisation; name one")
	}
	org, err := storage.UUID(p.OrganizationID)
	if err != nil {
		return dbgen.GetUserPreferencesParams{}, plxerr.New(plxerr.InvalidFormat, "the organisation identifier is not a UUID")
	}
	user, err := storage.UUID(p.UserID)
	if err != nil {
		return dbgen.GetUserPreferencesParams{}, plxerr.New(plxerr.InvalidFormat, "the user identifier is not a UUID")
	}
	return dbgen.GetUserPreferencesParams{OrganizationID: org, UserID: user}, nil
}

// load returns the stored preferences, or the defaults where none are.
func load(ctx context.Context, q *dbgen.Queries, key dbgen.GetUserPreferencesParams) (Preferences, error) {
	doc, err := q.GetUserPreferences(ctx, key)
	if errors.Is(err, pgx.ErrNoRows) {
		return Defaults(), nil
	}
	if err != nil {
		return Preferences{}, err //nolint:wrapcheck // failure wraps it
	}
	var out Preferences
	if err := json.Unmarshal(doc, &out); err != nil {
		return Preferences{}, fmt.Errorf("preferences: the stored preferences do not decode: %w", err)
	}
	return withDefaults(out), nil
}

// apply returns cur with the fields that mask names taken from in.
func apply(cur, in Preferences, mask []string) Preferences {
	for _, f := range mask {
		switch f {
		case FieldTheme:
			cur.Theme = in.Theme
		case FieldLanguage:
			cur.Language = in.Language
		case FieldDensity:
			cur.Density = in.Density
		case FieldShortcuts:
			cur.Shortcuts = maps.Clone(in.Shortcuts)
		case FieldFavouriteAppIDs:
			cur.FavouriteAppIDs = slices.Clone(in.FavouriteAppIDs)
		case FieldPinned:
			cur.Pinned = slices.Clone(in.Pinned)
		case FieldPreview:
			cur.Preview = maps.Clone(in.Preview)
		case FieldOnboardingDismissed:
			cur.OnboardingDismissed = in.OnboardingDismissed
		}
	}
	return cur
}

// withDefaults fills the fields left empty with their defaults.
func withDefaults(p Preferences) Preferences {
	if p.Theme == "" {
		p.Theme = ThemeSystem
	}
	if p.Density == "" {
		p.Density = DensityComfortable
	}
	return p
}

// validate checks every field of p against its enum, format and bound, and
// writes back the canonical form of the language tags.
func validate(p *Preferences, lim limits.Set) error {
	if !slices.Contains([]string{"", ThemeSystem, ThemeLight, ThemeDark}, p.Theme) {
		return plxerr.New(plxerr.InvalidEnumValue, "theme %q is not system, light or dark", p.Theme)
	}
	if !slices.Contains([]string{"", DensityComfortable, DensityCompact}, p.Density) {
		return plxerr.New(plxerr.InvalidEnumValue, "density %q is not comfortable or compact", p.Density)
	}
	tag, err := languageTag("language", p.Language)
	if err != nil {
		return err
	}
	p.Language = tag
	for k, v := range p.Shortcuts {
		if k == "" || v == "" {
			return plxerr.New(plxerr.InvalidFormat, "a shortcut needs a command ID and a key binding")
		}
	}
	if err := bounded("favourite_app_ids", p.FavouriteAppIDs, lim.Get(limits.StudioFavouriteApps)); err != nil {
		return err
	}
	for _, id := range p.FavouriteAppIDs {
		if _, err := storage.UUID(id); err != nil {
			return plxerr.New(plxerr.InvalidFormat, "favourite app %q is not an app ID", id)
		}
	}
	if err := bounded("pinned", p.Pinned, lim.Get(limits.StudioPinnedItems)); err != nil {
		return err
	}
	if slices.Contains(p.Pinned, "") {
		return plxerr.New(plxerr.InvalidFormat, "a pinned item needs a path")
	}
	for app, sw := range p.Preview {
		if err := validatePreview(app, &sw); err != nil {
			return err
		}
		p.Preview[app] = sw
	}
	return nil
}

func validatePreview(app string, sw *PreviewSwitches) error {
	if _, err := storage.UUID(app); err != nil {
		return plxerr.New(plxerr.InvalidFormat, "preview key %q is not an app ID", app)
	}
	if !slices.Contains([]string{"", PlatformMaterial, PlatformCupertino}, sw.Platform) {
		return plxerr.New(plxerr.InvalidEnumValue, "preview platform %q is not material or cupertino", sw.Platform)
	}
	if math.IsNaN(sw.TextScale) || math.IsInf(sw.TextScale, 0) || sw.TextScale < 0 {
		return plxerr.New(plxerr.OutOfRange, "the preview text scale must be a finite, non-negative number")
	}
	tag, err := languageTag("preview locale", sw.Locale)
	if err != nil {
		return err
	}
	sw.Locale = tag
	return nil
}

// languageTag returns the canonical form of a BCP 47 tag; empty stays empty.
func languageTag(field, s string) (string, error) {
	if s == "" {
		return "", nil
	}
	t, err := language.Parse(s)
	if err != nil {
		return "", plxerr.New(plxerr.InvalidFormat, "%s %q is not a BCP 47 language tag", field, s)
	}
	return t.String(), nil
}

// bounded refuses a list longer than its limit or holding an item twice.
func bounded(field string, items []string, most int64) error {
	if int64(len(items)) > most {
		return plxerr.New(plxerr.OutOfRange, "%s holds %d items, more than its limit of %d", field, len(items), most)
	}
	seen := make(map[string]struct{}, len(items))
	for _, it := range items {
		if _, dup := seen[it]; dup {
			return plxerr.New(plxerr.DuplicateID, "%s holds %q twice", field, it)
		}
		seen[it] = struct{}{}
	}
	return nil
}

// failure keeps a domain error as it is and marks any other as this
// package's.
func failure(err error) error {
	if err == nil {
		return nil
	}
	var perr *plxerr.Error
	if errors.As(err, &perr) {
		return err
	}
	return fmt.Errorf("preferences: %w", err)
}
