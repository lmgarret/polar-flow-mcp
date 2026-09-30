package flow

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/lmgarret/polar-flow-mcp/internal/flow/gen"
)

// Default zone sources sent to recalculate: the server only fills a zone list
// whose source is set, and DEFAULT asks for the computed zones.
const (
	hrSourceDefault    = "HEART_RATE_ZONE_SETTING_SOURCE_DEFAULT"
	speedSourceDefault = "SPEED_ZONE_SETTING_SOURCE_DEFAULT"
	powerSourceDefault = "POWER_ZONE_SETTING_SOURCE_DEFAULT"
)

// SportProfileUUID returns the uuid Polar gives a sport's profile:
// 0f000000-0080-0000-0000-<sportId as 12 hex digits> (RUNNING → …000000000001).
// The recalculate endpoint needs a uuid of this form even when no profile is
// stored; one with a zero sport segment is rejected.
func SportProfileUUID(sportID int) uuid.UUID {
	return uuid.MustParse(fmt.Sprintf("0f000000-0080-0000-0000-%012x", sportID))
}

// ListSportProfiles returns the account's stored sport profiles. The elements
// are slim (sportId only, no settings) — read one with GetSportProfile for its
// zones.
func (c *Client) ListSportProfiles(ctx context.Context) ([]gen.SportProfile, error) {
	res, err := c.API.ListSportProfiles(ctx)
	if err != nil {
		return nil, wrapFlowError("list sport profiles", err)
	}
	switch v := res.(type) {
	case *gen.ListSportProfilesOKApplicationJSON:
		return []gen.SportProfile(*v), nil
	case *gen.Unauthorized:
		return nil, ErrLoginFailed
	default:
		return nil, fmt.Errorf("flow: list sport profiles: unexpected response %T", res)
	}
}

// GetSportProfile reads one stored sport profile, including its
// settings.zoneLimits. ok=false when no profile has that uuid.
func (c *Client) GetSportProfile(ctx context.Context, id uuid.UUID) (p *gen.SportProfile, ok bool, err error) {
	res, err := c.API.GetSportProfile(ctx, gen.GetSportProfileParams{ID: id})
	if err != nil {
		return nil, false, wrapFlowError("get sport profile", err)
	}
	switch v := res.(type) {
	case *gen.SportProfile:
		return v, true, nil
	case *gen.GetSportProfileNotFound:
		return nil, false, nil
	case *gen.GetSportProfileBadRequest:
		return nil, false, fmt.Errorf("flow: get sport profile: %s", readBody(v.Data))
	case *gen.Unauthorized:
		return nil, false, ErrLoginFailed
	default:
		return nil, false, fmt.Errorf("flow: get sport profile: unexpected response %T", res)
	}
}

// DefaultSportZones computes a sport's default heart-rate, speed and power
// zones (and the thresholds they derive from) for this account, via
// POST /api/sports/profiles/{uuid}/recalculate. That endpoint persists nothing
// and needs no stored profile, so it answers for sports the user never set up.
// userID must be the signed-in account's id (Polar rejects any other).
func (c *Client) DefaultSportZones(ctx context.Context, userID int64, sportID int) (*gen.SportProfile, error) {
	id := SportProfileUUID(sportID)
	body := &gen.SportProfileWriteRequest{
		UUID:   id.String(),
		UserId: int(userID),
		Profile: gen.SportProfileBody{
			SportId: sportID,
			Settings: gen.NewOptSportProfileBodySettings(gen.SportProfileBodySettings{
				ZoneLimits: gen.NewOptSportProfileZoneLimits(gen.SportProfileZoneLimits{
					HeartRateSettingSource: gen.NewOptString(hrSourceDefault),
					SpeedSettingSource:     gen.NewOptString(speedSourceDefault),
					PowerSettingSource:     gen.NewOptString(powerSourceDefault),
				}),
			}),
		},
	}
	res, err := c.API.RecalculateSportProfile(ctx, body, gen.RecalculateSportProfileParams{
		ID:             id,
		XRequestedWith: gen.XRequestedWithXMLHttpRequest,
	})
	if err != nil {
		return nil, wrapFlowError("compute default zones", err)
	}
	switch v := res.(type) {
	case *gen.SportProfile:
		return v, nil
	case *gen.RecalculateSportProfileBadRequest:
		return nil, fmt.Errorf("flow: compute default zones: %s", readBody(v.Data))
	case *gen.Unauthorized:
		return nil, ErrLoginFailed
	default:
		return nil, fmt.Errorf("flow: compute default zones: unexpected response %T", res)
	}
}

// readBody returns a short plain-text error body for messages.
func readBody(r io.Reader) string {
	if r == nil {
		return "(empty body)"
	}
	b, _ := io.ReadAll(io.LimitReader(r, 512))
	if s := strings.TrimSpace(string(b)); s != "" {
		return s
	}
	return "(empty body)"
}

// Zone setting sources a write can choose per list: FREE keeps hand-entered
// limits, DEFAULT makes Flow recompute the list (the limits sent are ignored).
const (
	HRSourceFree       = "HEART_RATE_ZONE_SETTING_SOURCE_FREE"
	SpeedSourceFree    = "SPEED_ZONE_SETTING_SOURCE_FREE"
	PowerSourceFree    = "POWER_ZONE_SETTING_SOURCE_FREE"
	HRSourceDefault    = hrSourceDefault
	SpeedSourceDefault = speedSourceDefault
	PowerSourceDefault = powerSourceDefault
)

// ErrUnknownSport is returned when Flow refuses a sport id (create answers an
// empty 500 for an id outside /api/sports/sports).
var ErrUnknownSport = errors.New("flow: unknown sport id")

// ErrLastSportProfile is returned when deleting the account's only sport
// profile, which Flow refuses.
var ErrLastSportProfile = errors.New("flow: the last sport profile cannot be deleted")

// CreateSportProfile creates the sport profile for sportID with Polar's
// default settings and zones, via POST /api/sports/profiles/create/{sportId}.
// It is idempotent: when a profile already exists Flow answers 200 with the
// stored profile and changes nothing — created reports which case it was.
func (c *Client) CreateSportProfile(ctx context.Context, sportID int) (p *gen.SportProfile, created bool, err error) {
	res, err := c.API.CreateSportProfile(ctx, gen.CreateSportProfileParams{
		SportId:        sportID,
		XRequestedWith: gen.XRequestedWithXMLHttpRequest,
	})
	if err != nil {
		return nil, false, wrapFlowError("create sport profile", err)
	}
	switch v := res.(type) {
	case *gen.CreateSportProfileCreated:
		return (*gen.SportProfile)(v), true, nil
	case *gen.CreateSportProfileOK:
		return (*gen.SportProfile)(v), false, nil
	case *gen.CreateSportProfileInternalServerError:
		return nil, false, ErrUnknownSport
	case *gen.Unauthorized:
		return nil, false, ErrLoginFailed
	default:
		return nil, false, fmt.Errorf("flow: create sport profile: unexpected response %T", res)
	}
}

// UpdateSportProfileZones saves the zones of a stored profile via
// POST /api/sports/profiles/{uuid}/update-zones. p is the stored profile (from
// GetSportProfile) with zl its new zone limits. Flow reads only
// settings.zoneLimits, requires all three setting sources, and recomputes any
// list whose source is DEFAULT. userID must be the signed-in account's id.
func (c *Client) UpdateSportProfileZones(ctx context.Context, userID int64, p *gen.SportProfile, zl gen.SportProfileZoneLimits) error {
	id, err := uuid.Parse(p.UUID)
	if err != nil {
		return fmt.Errorf("flow: update zones: profile has an unexpected id %q", p.UUID)
	}
	// Flow stores its own write time and 409s a write whose modified is not
	// newer. It reports that time truncated to the second, so a write within
	// the same second as the last one (right after create) must still clear
	// it: stay a full second ahead of the stored value, whatever the clock.
	modified := time.Now().UTC()
	if stored, ok := p.Modified.Get(); ok && modified.Before(stored.Add(time.Second)) {
		modified = stored.Add(time.Second)
	}
	body := &gen.SportProfileWriteRequest{
		UUID:     p.UUID,
		UserId:   int(userID),
		Modified: gen.NewOptDateTime(modified),
		Profile: gen.SportProfileBody{
			SportId: p.Profile.SportId,
			Settings: gen.NewOptSportProfileBodySettings(gen.SportProfileBodySettings{
				ZoneLimits: gen.NewOptSportProfileZoneLimits(zl),
			}),
		},
	}
	res, err := c.API.UpdateSportProfileZones(ctx, body, gen.UpdateSportProfileZonesParams{
		ID:             id,
		XRequestedWith: gen.XRequestedWithXMLHttpRequest,
	})
	if err != nil {
		return wrapFlowError("update zones", err)
	}
	switch v := res.(type) {
	case *gen.UpdateSportProfileZonesOK:
		return nil
	case *gen.UpdateSportProfileZonesBadRequest:
		return fmt.Errorf("flow: update zones rejected: %s", zoneValidationMessage(readBody(v.Data)))
	case *gen.UpdateSportProfileZonesConflict:
		return fmt.Errorf("flow: update zones: the profile changed meanwhile, retry: %s", readBody(v.Data))
	case *gen.Unauthorized:
		return ErrLoginFailed
	default:
		return fmt.Errorf("flow: update zones: unexpected response %T", res)
	}
}

// zoneValidationMessage trims Flow's validation 400 ("Invalid update sport
// profile request for user … with message 'Invalid request: Profile validation
// failed with N error(s): …'") down to the list of errors.
func zoneValidationMessage(body string) string {
	const marker = "error(s): "
	if i := strings.Index(body, marker); i >= 0 {
		return strings.TrimSuffix(body[i+len(marker):], "'")
	}
	return body
}

// DeleteSportProfile deletes a stored sport profile. Flow answers 200 for a
// uuid that does not exist, so callers check existence first; deleting the
// last profile is refused (ErrLastSportProfile).
func (c *Client) DeleteSportProfile(ctx context.Context, id uuid.UUID) error {
	res, err := c.API.DeleteSportProfile(ctx, gen.DeleteSportProfileParams{
		ID:             id,
		XRequestedWith: gen.XRequestedWithXMLHttpRequest,
	})
	if err != nil {
		return wrapFlowError("delete sport profile", err)
	}
	switch v := res.(type) {
	case *gen.DeleteSportProfileOK:
		return nil
	case *gen.DeleteSportProfileBadRequest:
		msg := readBody(v.Data)
		if strings.Contains(msg, "last active profile") {
			return ErrLastSportProfile
		}
		return fmt.Errorf("flow: delete sport profile: %s", msg)
	case *gen.Unauthorized:
		return ErrLoginFailed
	default:
		return fmt.Errorf("flow: delete sport profile: unexpected response %T", res)
	}
}
