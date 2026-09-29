package mcp

import (
	"context"
	"errors"
	"fmt"
	"strings"

	mcpgo "github.com/mark3labs/mcp-go/mcp"

	"github.com/lmgarret/polar-flow-mcp/internal/convert"
	"github.com/lmgarret/polar-flow-mcp/internal/flow"
)

const (
	defaultRouteName      = "Imported route"
	defaultRouteMaxPoints = 500
	maxRouteMaxPoints     = 100000
)

// importRouteArgs is the validated import_route input.
type importRouteArgs struct {
	route   *convert.ParsedRoute
	name    string
	sportID *int
}

// parseImportRouteArgs validates the arguments and parses the file — every
// check that needs no request.
func parseImportRouteArgs(req mcpgo.CallToolRequest) (importRouteArgs, error) {
	var a importRouteArgs
	content, hasContent, err := stringArg(req, "content")
	if err != nil {
		return a, err
	}
	if !hasContent || strings.TrimSpace(content) == "" {
		return a, errors.New("content is required: the full text of a GPX or TCX file")
	}
	format, _, err := stringArg(req, "format")
	if err != nil {
		return a, err
	}
	nameArg, hasName, err := stringArg(req, "name")
	if err != nil {
		return a, err
	}
	if v, ok, err := intArgRange(req, "sport_id", 1, 100000, ""); err != nil {
		return a, err
	} else if ok {
		n := int(v)
		a.sportID = &n
	}
	if a.route, err = convert.ParseRoute([]byte(content), format); err != nil {
		return a, err
	}
	a.name = a.route.Name
	if strings.TrimSpace(a.name) == "" {
		a.name = defaultRouteName
	}
	if hasName {
		// An explicit name is validated as given — a blank one is an error,
		// not a request for the default.
		a.name = nameArg
	}
	if err := convert.ValidateRouteName(a.name); err != nil {
		if !hasName {
			return a, fmt.Errorf("the file's route name %q is too long (%v); pass a shorter name", a.name, err)
		}
		return a, err
	}
	return a, nil
}

// newRouteFavorite finds the ROUTE favorite that appeared after an import:
// not in `before`, preferring one with the imported name.
func newRouteFavorite(before map[int64]bool, after []convert.FavoriteListItem, name string) *convert.FavoriteListItem {
	var created *convert.FavoriteListItem
	for i := range after {
		it := after[i]
		if before[it.FavoriteID] || it.Type != "ROUTE" {
			continue
		}
		if created == nil || it.Name == name {
			created = &after[i]
		}
	}
	return created
}

// ImportRouteHandler parses a GPX/TCX file in Go and uploads it the way the
// web UI does (JSON trackpoints, not the file), then finds the new ROUTE
// favorite — Flow's import answers 200 with an empty body.
func ImportRouteHandler(fc *flow.Client) toolHandler {
	return func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		a, err := parseImportRouteArgs(req)
		if err != nil {
			return mcpgo.NewToolResultError(err.Error()), nil
		}
		if a.sportID != nil {
			// Flow silently stores an unknown sport as null.
			if err := checkSport(ctx, fc, *a.sportID); err != nil {
				return mcpgo.NewToolResultError(err.Error()), nil
			}
		}
		before, err := fc.ListFavorites(ctx)
		if err != nil {
			return mcpgo.NewToolResultError(err.Error()), nil
		}
		seen := make(map[int64]bool, len(before))
		for _, f := range before {
			seen[f.FavoriteId.Or(0)] = true
		}
		if err := fc.ImportRoute(ctx, convert.ToWireRouteImport(a.route, a.name, a.sportID)); err != nil {
			return mcpgo.NewToolResultError(err.Error()), nil
		}
		data := map[string]any{
			"name": a.name, "format": string(a.route.Format), "point_count": len(a.route.Points),
			"distance_m": a.route.DistanceM, "sport_id": a.sportID,
		}
		msg := fmt.Sprintf("Imported route %q (%d points, %.0f m).", a.name, len(a.route.Points), a.route.DistanceM)
		after, err := fc.ListFavorites(ctx)
		if err != nil {
			return mcpgo.NewToolResultText(msg + " Listing favorites afterwards failed: " + err.Error()), nil
		}
		if created := newRouteFavorite(seen, convert.FromWireFavoriteList(after), a.name); created != nil {
			data["favorite_id"] = created.FavoriteID
			data["exercise_target_id"] = created.ExerciseTargetID
			msg += fmt.Sprintf(" favorite_id %d, exercise_target_id %d.", created.FavoriteID, created.ExerciseTargetID)
		} else {
			msg += " The new favorite did not show up in list_favorites yet."
		}
		return widgetResultText(msg, map[string]any{"type": "route_imported", "route": data}), nil
	}
}

// routeRef is the validated get_route input.
type routeRef struct {
	exerciseTargetID, favoriteID int64
	byFavorite                   bool
	maxPoints                    int
}

func parseRouteRef(req mcpgo.CallToolRequest) (routeRef, error) {
	var r routeRef
	etID, hasET, err := intArgRange(req, "exercise_target_id", 1, 1<<52, "")
	if err != nil {
		return r, err
	}
	favID, hasFav, err := intArgRange(req, "favorite_id", 1, 1<<52, "")
	if err != nil {
		return r, err
	}
	maxPoints, hasMax, err := intArgRange(req, "max_points", 2, maxRouteMaxPoints, "")
	if err != nil {
		return r, err
	}
	switch {
	case hasET && hasFav:
		return r, errors.New("pass either exercise_target_id or favorite_id, not both")
	case !hasET && !hasFav:
		return r, errors.New("pass exercise_target_id or favorite_id (from list_favorites)")
	}
	r = routeRef{exerciseTargetID: etID, favoriteID: favID, byFavorite: hasFav, maxPoints: defaultRouteMaxPoints}
	if hasMax {
		r.maxPoints = int(maxPoints)
	}
	return r, nil
}

// routeExerciseTarget resolves a favorite_id to its route's exercise target.
// A non-nil result is the final tool answer (not found / not a route).
func routeExerciseTarget(ctx context.Context, fc *flow.Client, favID int64) (int64, *mcpgo.CallToolResult) {
	list, err := fc.ListFavorites(ctx)
	if err != nil {
		return 0, mcpgo.NewToolResultError(err.Error())
	}
	for _, it := range convert.FromWireFavoriteList(list) {
		if it.FavoriteID != favID {
			continue
		}
		if it.Type != "ROUTE" {
			return 0, mcpgo.NewToolResultError(fmt.Sprintf("favorite %d is a %s favorite, not a route", favID, it.Type))
		}
		return it.ExerciseTargetID, nil
	}
	return 0, mcpgo.NewToolResultText(fmt.Sprintf("No favorite with favorite_id %d.", favID))
}

// GetRouteHandler returns a ROUTE favorite's geometry. Flow's endpoint takes
// the exercise-target id and answers 403 for anything else (including a
// favorite id passed by mistake), so the tool also accepts favorite_id and
// resolves it through the favorites list.
func GetRouteHandler(fc *flow.Client) toolHandler {
	return func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		ref, err := parseRouteRef(req)
		if err != nil {
			return mcpgo.NewToolResultError(err.Error()), nil
		}
		etID := ref.exerciseTargetID
		if ref.byFavorite {
			var answer *mcpgo.CallToolResult
			if etID, answer = routeExerciseTarget(ctx, fc, ref.favoriteID); answer != nil {
				return answer, nil
			}
		}
		et, err := fc.GetFavoriteExerciseTarget(ctx, etID)
		if err != nil {
			if errors.Is(err, flow.ErrFavoriteNotFound) {
				return mcpgo.NewToolResultText(fmt.Sprintf("No route with exercise_target_id %d. Note this is the "+
					"exercise_target_id from list_favorites, not the favorite_id.", etID)), nil
			}
			return mcpgo.NewToolResultError(err.Error()), nil
		}
		geo, ok := convert.FromWireRouteGeometry(et, ref.maxPoints)
		if !ok {
			return mcpgo.NewToolResultError(fmt.Sprintf("exercise target %d has no route geometry (not a ROUTE favorite)", etID)), nil
		}
		if ref.byFavorite {
			geo.FavoriteID = &ref.favoriteID
		}
		return widgetResult(map[string]any{"type": "route_geometry", "route": geo}), nil
	}
}
