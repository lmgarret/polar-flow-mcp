package flow

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/go-faster/jx"

	"github.com/lmgarret/polar-flow-mcp/internal/convert"
	"github.com/lmgarret/polar-flow-mcp/internal/flow/gen"
)

// Favorites are date-less training-target templates (and imported GPX/TCX
// routes). Their endpoints are spread over four path prefixes and several
// answer JSON labelled text/plain; see internal/flow/openapi.yaml and
// polar-openapi-maker/docs/endpoints/favorites.md.

// readSmall drains a text/plain response body (bounded) into a string.
func readSmall(r io.Reader) string {
	if r == nil {
		return ""
	}
	b, _ := io.ReadAll(io.LimitReader(r, 4096))
	return strings.TrimSpace(string(b))
}

// ListFavorites returns every favorite (training-target templates and ROUTE
// favorites) in the rich /api/favorites shape.
func (c *Client) ListFavorites(ctx context.Context) ([]gen.FavoriteListing, error) {
	res, err := c.API.ListFavorites(ctx)
	if err != nil {
		return nil, wrapFlowError("list favorites", err)
	}
	switch v := res.(type) {
	case *gen.FavoritesListing:
		return v.Targets, nil
	case *gen.Unauthorized:
		return nil, ErrLoginFailed
	default:
		return nil, fmt.Errorf("flow: list favorites: unexpected response %T", res)
	}
}

// GetFavorite reads one favorite by favoriteId. Unknown → ErrFavoriteNotFound,
// another account's → ErrNotOwned.
func (c *Client) GetFavorite(ctx context.Context, id int64) (*gen.Favorite, error) {
	if id <= 0 {
		return nil, fmt.Errorf("flow: favorite id must be > 0")
	}
	res, err := c.API.GetFavorite(ctx, gen.GetFavoriteParams{ID: id})
	if err != nil {
		return nil, wrapFlowError("get favorite", err)
	}
	switch v := res.(type) {
	case *gen.Favorite:
		return v, nil
	case *gen.GetFavoriteNotFound:
		return nil, ErrFavoriteNotFound
	case *gen.GetFavoriteForbidden:
		return nil, ErrNotOwned
	case *gen.Unauthorized:
		return nil, ErrLoginFailed
	default:
		return nil, fmt.Errorf("flow: get favorite: unexpected response %T", res)
	}
}

// CreateFavorite posts a new favorite and returns its favoriteId (the 201 body
// is the bare id as text/plain).
func (c *Client) CreateFavorite(ctx context.Context, body *gen.FavoriteCreate) (int64, error) {
	res, err := c.API.CreateFavorite(ctx, body, gen.CreateFavoriteParams{
		XRequestedWith: gen.XRequestedWithXMLHttpRequest,
	})
	if err != nil {
		return 0, wrapFlowError("create favorite", err)
	}
	switch v := res.(type) {
	case *gen.CreateFavoriteCreated:
		raw := readSmall(v.Data)
		id, perr := strconv.ParseInt(raw, 10, 64)
		if perr != nil {
			return 0, fmt.Errorf("flow: create favorite: cannot parse id from %q", raw)
		}
		return id, nil
	case *gen.ValidationError:
		return 0, fmt.Errorf("flow: create favorite: %s", formatValidationError(v))
	case *gen.CreateFavoriteInternalServerError:
		return 0, errors.New("flow: create favorite: Polar returned 500 (invalid favorite type?)")
	case *gen.Unauthorized:
		return 0, ErrLoginFailed
	default:
		return 0, fmt.Errorf("flow: create favorite: unexpected response %T", res)
	}
}

// UpdateFavorite full-replaces a favorite. Each exerciseTargets[].id must carry
// the id from GetFavorite — Flow silently drops the changes of an entry sent
// with a null id.
func (c *Client) UpdateFavorite(ctx context.Context, id int64, body *gen.Favorite) error {
	if id <= 0 {
		return fmt.Errorf("flow: favorite id must be > 0")
	}
	res, err := c.API.UpdateFavorite(ctx, body, gen.UpdateFavoriteParams{
		ID:             id,
		XRequestedWith: gen.XRequestedWithXMLHttpRequest,
	})
	if err != nil {
		return wrapFlowError("update favorite", err)
	}
	switch v := res.(type) {
	case *gen.UpdateFavoriteOK:
		return nil
	case *gen.ValidationError:
		return fmt.Errorf("flow: update favorite: %s", formatValidationError(v))
	case *gen.UpdateFavoriteNotFound:
		return ErrFavoriteNotFound
	case *gen.UpdateFavoriteForbidden:
		return ErrNotOwned
	case *gen.Unauthorized:
		return ErrLoginFailed
	default:
		return fmt.Errorf("flow: update favorite: unexpected response %T", res)
	}
}

// DeleteFavorite deletes a favorite (templates and routes alike).
func (c *Client) DeleteFavorite(ctx context.Context, id int64) error {
	if id <= 0 {
		return fmt.Errorf("flow: favorite id must be > 0")
	}
	res, err := c.API.DeleteFavorite(ctx, gen.DeleteFavoriteParams{
		ID:             id,
		XRequestedWith: gen.XRequestedWithXMLHttpRequest,
	})
	if err != nil {
		return wrapFlowError("delete favorite", err)
	}
	switch res.(type) {
	case *gen.DeleteFavoriteOK:
		return nil
	case *gen.DeleteFavoriteNotFound:
		return ErrFavoriteNotFound
	case *gen.DeleteFavoriteBadRequest:
		return ErrNotOwned
	case *gen.Unauthorized:
		return ErrLoginFailed
	default:
		return fmt.Errorf("flow: delete favorite: unexpected response %T", res)
	}
}

// RenameFavorite changes only a favorite's name (PUT /api/favorites/saveName).
func (c *Client) RenameFavorite(ctx context.Context, id int64, name string) error {
	if id <= 0 {
		return fmt.Errorf("flow: favorite id must be > 0")
	}
	res, err := c.API.RenameFavorite(ctx, &gen.RenameFavoriteReq{
		FavoriteId:   gen.NewInt64RenameFavoriteReqFavoriteId(id),
		FavoriteName: name,
	}, gen.RenameFavoriteParams{XRequestedWith: gen.XRequestedWithXMLHttpRequest})
	if err != nil {
		return wrapFlowError("rename favorite", err)
	}
	switch v := res.(type) {
	case *gen.RenameFavoriteOK:
		return nil
	case *gen.RenameFavoriteBadRequest:
		return fmt.Errorf("flow: rename favorite: rejected (empty or over 45 characters, or another account's favorite): %s", readSmall(v.Data))
	case *gen.RenameFavoriteInternalServerError:
		return ErrFavoriteNotFound
	default:
		return fmt.Errorf("flow: rename favorite: unexpected response %T", res)
	}
}

// ChangeFavoriteSport sets the sport of one of a favorite's exercise targets
// (PUT /api/favorites/saveSport). Flow validates neither the sport id nor the
// exercise-target id, so callers must check both first and read back after.
func (c *Client) ChangeFavoriteSport(ctx context.Context, favoriteID, exerciseTargetID int64, sportID int) error {
	if favoriteID <= 0 || exerciseTargetID <= 0 || sportID <= 0 {
		return fmt.Errorf("flow: change favorite sport: ids must be > 0")
	}
	res, err := c.API.ChangeFavoriteSport(ctx, &gen.ChangeFavoriteSportReq{
		FavoriteId:       favoriteID,
		FavoriteSportId:  sportID,
		ExerciseTargetId: exerciseTargetID,
	}, gen.ChangeFavoriteSportParams{XRequestedWith: gen.XRequestedWithXMLHttpRequest})
	if err != nil {
		return wrapFlowError("change favorite sport", err)
	}
	switch v := res.(type) {
	case *gen.ChangeFavoriteSportOK:
		return nil
	case *gen.ChangeFavoriteSportBadRequest:
		return fmt.Errorf("flow: change favorite sport: rejected: %s", readSmall(v.Data))
	case *gen.ChangeFavoriteSportInternalServerError:
		return ErrFavoriteNotFound
	default:
		return fmt.Errorf("flow: change favorite sport: unexpected response %T", res)
	}
}

// CreateTargetFromFavorite schedules a favorite as a training target at the
// wall-clock time `when` and returns the new target. Midnight means "no time"
// to Flow (it schedules at 18:00); callers should not pass exact midnight.
func (c *Client) CreateTargetFromFavorite(ctx context.Context, favoriteID int64, when time.Time) (*gen.TargetFromFavorite, error) {
	if favoriteID <= 0 {
		return nil, fmt.Errorf("flow: favorite id must be > 0")
	}
	res, err := c.API.CreateTargetFromFavorite(ctx, &gen.TargetFromFavoriteRequest{
		ID: strconv.FormatInt(favoriteID, 10),
		To: convert.WireFavouriteTo(when),
	}, gen.CreateTargetFromFavoriteParams{XRequestedWith: gen.XRequestedWithXMLHttpRequest})
	if err != nil {
		return nil, wrapFlowError("schedule favorite", err)
	}
	switch v := res.(type) {
	case *gen.TargetFromFavorite:
		return v, nil
	case *gen.CreateTargetFromFavoriteOKTextPlain:
		// The JSON body arrives labelled text/plain; decode it with the
		// generated model so the canonical mapping stays in convert.
		raw, rerr := io.ReadAll(io.LimitReader(v.Data, 1<<20))
		if rerr != nil {
			return nil, fmt.Errorf("flow: schedule favorite: read response: %w", rerr)
		}
		return DecodeTargetFromFavorite(raw)
	case *gen.CreateTargetFromFavoriteBadRequest:
		body := readSmall(v.Data)
		if strings.Contains(body, "invalid.favorite") {
			return nil, ErrNotOwned
		}
		return nil, fmt.Errorf("flow: schedule favorite: rejected: %s", body)
	case *gen.CreateTargetFromFavoriteInternalServerError:
		return nil, ErrFavoriteNotFound
	case *gen.Unauthorized:
		return nil, ErrLoginFailed
	default:
		return nil, fmt.Errorf("flow: schedule favorite: unexpected response %T", res)
	}
}

// DecodeTargetFromFavorite parses the text/plain JSON body of
// createTargetFromFavourite into the generated model.
func DecodeTargetFromFavorite(raw []byte) (*gen.TargetFromFavorite, error) {
	var out gen.TargetFromFavorite
	if err := out.Decode(jx.DecodeBytes(raw)); err != nil {
		return nil, fmt.Errorf("flow: schedule favorite: decode response: %w", err)
	}
	return &out, nil
}

// ImportRoute uploads a client-parsed GPX/TCX route. Flow answers 200 with an
// empty body; the new favorite has to be found via ListFavorites.
func (c *Client) ImportRoute(ctx context.Context, body *gen.RouteImport) error {
	res, err := c.API.ImportRoute(ctx, body, gen.ImportRouteParams{
		XRequestedWith: gen.XRequestedWithXMLHttpRequest,
	})
	if err != nil {
		return wrapFlowError("import route", err)
	}
	switch v := res.(type) {
	case *gen.ImportRouteOK:
		return nil
	case *gen.ImportRouteBadRequest:
		return fmt.Errorf("flow: import route: rejected: %s", readSmall(v.Data))
	case *gen.ImportRouteInternalServerError:
		return errors.New("flow: import route: Polar returned 500 (malformed route points)")
	case *gen.Unauthorized:
		return ErrLoginFailed
	default:
		return fmt.Errorf("flow: import route: unexpected response %T", res)
	}
}

// GetFavoriteExerciseTarget reads a favorite's inner exercise target — for
// ROUTE favorites, the GPS geometry. Takes the exerciseTargetId, not the
// favoriteId; Flow answers 403 (never 404) for any id that is not one of this
// account's exercise targets, mapped here to ErrFavoriteNotFound.
func (c *Client) GetFavoriteExerciseTarget(ctx context.Context, exerciseTargetID int64) (*gen.FavoriteExerciseTarget, error) {
	if exerciseTargetID <= 0 {
		return nil, fmt.Errorf("flow: exercise target id must be > 0")
	}
	res, err := c.API.GetFavoriteExerciseTarget(ctx, gen.GetFavoriteExerciseTargetParams{ID: exerciseTargetID})
	if err != nil {
		return nil, wrapFlowError("get favorite exercise target", err)
	}
	switch v := res.(type) {
	case *gen.FavoriteExerciseTarget:
		return v, nil
	case *gen.GetFavoriteExerciseTargetForbidden:
		return nil, ErrFavoriteNotFound
	case *gen.Unauthorized:
		return nil, ErrLoginFailed
	default:
		return nil, fmt.Errorf("flow: get favorite exercise target: unexpected response %T", res)
	}
}
