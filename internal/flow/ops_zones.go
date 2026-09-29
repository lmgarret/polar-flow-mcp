package flow

import (
	"context"
	"fmt"
	"io"
	"strings"

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
