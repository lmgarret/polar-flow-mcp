package mcp

import (
	"context"
	"errors"
	"fmt"
	"strings"

	mcpgo "github.com/mark3labs/mcp-go/mcp"

	"github.com/lmgarret/polar-flow-mcp/internal/convert"
	"github.com/lmgarret/polar-flow-mcp/internal/flow"
	"github.com/lmgarret/polar-flow-mcp/internal/flow/gen"
)

// Favorite limits, as probed on /api/favoritetarget (2026-09-28). Flow answers
// a too-long name or description with an untranslated 400, and silently stores
// unknown sport ids, so these are checked before sending.
const (
	maxFavoriteNameRunes = 45
	maxFavoriteDescRunes = 500
	maxFavoriteDurationS = 359999 // 99:59:59 — "100:00:00" is rejected
	maxFavoriteDistanceM = 9999000
)

type toolHandler = func(context.Context, mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error)

// favoriteErrorResult turns the flow sentinel errors into the tool's
// user-facing answer: a missing id is a plain (non-error) statement, like the
// target tools; another account's id and anything else are tool errors.
func favoriteErrorResult(err error, what string, id int64) *mcpgo.CallToolResult {
	switch {
	case errors.Is(err, flow.ErrFavoriteNotFound):
		return mcpgo.NewToolResultText(fmt.Sprintf("No favorite with %s %d.", what, id))
	case errors.Is(err, flow.ErrNotOwned):
		return mcpgo.NewToolResultError(fmt.Sprintf("%s %d belongs to another Polar account.", what, id))
	default:
		return mcpgo.NewToolResultError(err.Error())
	}
}

// checkSport validates a sport id against Polar's catalogue. Several favorite
// endpoints store unknown ids verbatim (saveSport, create/update), and the
// session edit answers them with a bare 500.
func checkSport(ctx context.Context, fc *flow.Client, id int) error {
	_, ok, err := fc.SportName(ctx, id)
	if err != nil {
		return fmt.Errorf("could not load the sport catalogue to validate sport_id: %w", err)
	}
	if !ok {
		return fmt.Errorf("sport_id %d is not a Polar sport — call list_sports for valid ids", id)
	}
	return nil
}

// favoriteGoalArgs validates the create/update favorite arguments that are
// independent of the account (types, lengths, ranges, exclusivity). It runs
// before buildGoalExerciseTarget, which assumes well-typed input.
func favoriteGoalArgs(req mcpgo.CallToolRequest) (name, desc string, sportID int, err error) {
	name, present, err := textArg(req, "name", maxFavoriteNameRunes, true)
	if err != nil {
		return "", "", 0, err
	}
	if !present {
		return "", "", 0, errors.New("name is required (1–45 characters)")
	}
	desc, _, err = textArg(req, "description", maxFavoriteDescRunes, false)
	if err != nil {
		return "", "", 0, err
	}
	sid, sportPresent, err := intArgRange(req, "sport_id", 1, 100000, "")
	if err != nil {
		return "", "", 0, err
	}
	sportID = 1
	if sportPresent {
		sportID = int(sid)
	}
	if err := favoriteGoalShape(req); err != nil {
		return "", "", 0, err
	}
	return name, desc, sportID, nil
}

// errNoGoal is favoriteGoalShape's answer when no goal argument was given.
var errNoGoal = errors.New("a favorite needs a goal: duration_s, distance_m, or a phases array")

// favoriteGoalShape checks the goal arguments: exactly one of duration_s,
// distance_m or a non-empty phases array, each in Polar's accepted range.
func favoriteGoalShape(req mcpgo.CallToolRequest) error {
	_, hasDur, err := intArgRange(req, "duration_s", 1, maxFavoriteDurationS, "seconds (99:59:59)")
	if err != nil {
		return err
	}
	dist, hasDist, err := floatArgRange(req, "distance_m", 0, maxFavoriteDistanceM, "metres")
	if err != nil {
		return err
	}
	if hasDist && dist <= 0 {
		return errors.New("distance_m must be greater than 0 metres")
	}
	phases, hasPhases := rawArg(req, "phases")
	if hasPhases {
		list, ok := phases.([]any)
		if !ok {
			return fmt.Errorf("phases must be an array, got %s", jsonTypeName(phases))
		}
		hasPhases = len(list) > 0
	}
	switch {
	case hasPhases && (hasDur || hasDist):
		return errors.New("give either phases or one of duration_s / distance_m, not both")
	case hasDur && hasDist:
		return errors.New("give only one of duration_s or distance_m for a VOLUME favorite")
	case !hasPhases && !hasDur && !hasDist:
		return errNoGoal
	}
	return nil
}

// buildFavoriteCreate validates the arguments and builds the wire body.
func buildFavoriteCreate(ctx context.Context, fc *flow.Client, req mcpgo.CallToolRequest) (*gen.FavoriteCreate, error) {
	name, desc, sportID, err := favoriteGoalArgs(req)
	if err != nil {
		return nil, err
	}
	et, phased, errMsg := buildGoalExerciseTarget(req)
	if errMsg != "" {
		return nil, errors.New(errMsg)
	}
	if err := checkSport(ctx, fc, sportID); err != nil {
		return nil, err
	}
	et.ID.SetToNull()
	body := &gen.FavoriteCreate{
		Type:            gen.FavoriteCreateTypeVOLUME,
		Name:            name,
		ExerciseTargets: []gen.ExerciseTarget{et},
	}
	if phased {
		body.Type = gen.FavoriteCreateTypePHASED
	}
	body.Description.SetTo(desc)
	return body, nil
}

// favoriteEntryFromTarget converts a create-shaped exercise target into the
// update body's entry type.
func favoriteEntryFromTarget(et gen.ExerciseTarget) gen.FavoriteExerciseTargetEntry {
	e := gen.FavoriteExerciseTargetEntry{
		Duration: et.Duration,
		Distance: et.Distance,
		Calories: et.Calories,
		Phases:   et.Phases,
	}
	e.SportId.SetTo(et.SportId)
	return e
}

// favoriteResult reads a favorite back and renders it as the tool result.
func favoriteResult(ctx context.Context, fc *flow.Client, id int64, headline string) *mcpgo.CallToolResult {
	f, err := fc.GetFavorite(ctx, id)
	if err != nil {
		return mcpgo.NewToolResultText(headline)
	}
	return widgetResultText(headline, map[string]any{
		"type": "favorite_detail", "favorite": convert.FromWireFavorite(id, f),
	})
}

// ListFavoritesHandler lists favorites (templates and routes).
func ListFavoritesHandler(fc *flow.Client) toolHandler {
	return func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		kind, _, err := stringArg(req, "kind")
		if err != nil {
			return mcpgo.NewToolResultError(err.Error()), nil
		}
		switch kind {
		case "", "all", "templates", "routes":
		default:
			return mcpgo.NewToolResultError(fmt.Sprintf("kind must be \"all\", \"templates\" or \"routes\", got %q", kind)), nil
		}
		wire, err := fc.ListFavorites(ctx)
		if err != nil {
			return mcpgo.NewToolResultError(err.Error()), nil
		}
		items := make([]convert.FavoriteListItem, 0, len(wire))
		for _, it := range convert.FromWireFavoriteList(wire) {
			isRoute := it.Type == "ROUTE"
			if (kind == "templates" && isRoute) || (kind == "routes" && !isRoute) {
				continue
			}
			items = append(items, it)
		}
		var b strings.Builder
		if len(items) == 0 {
			b.WriteString("No favorites.")
		} else {
			fmt.Fprintf(&b, "Favorites (%d):\n", len(items))
			for _, it := range items {
				fmt.Fprintf(&b, "- favorite_id %d (exercise_target_id %d): %q [%s]\n",
					it.FavoriteID, it.ExerciseTargetID, it.Name, it.Type)
			}
		}
		return widgetResultText(b.String(), map[string]any{"type": "favorite_list", "favorites": items}), nil
	}
}

// GetFavoriteHandler returns one favorite.
func GetFavoriteHandler(fc *flow.Client) toolHandler {
	return func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		id, err := idArg(req, "favorite_id")
		if err != nil {
			return mcpgo.NewToolResultError(err.Error()), nil
		}
		f, err := fc.GetFavorite(ctx, id)
		if err != nil {
			return favoriteErrorResult(err, "favorite_id", id), nil
		}
		return widgetResult(map[string]any{"type": "favorite_detail", "favorite": convert.FromWireFavorite(id, f)}), nil
	}
}

// CreateFavoriteHandler creates a favorite (training-target template).
func CreateFavoriteHandler(fc *flow.Client) toolHandler {
	return func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		body, err := buildFavoriteCreate(ctx, fc, req)
		if err != nil {
			return mcpgo.NewToolResultError(err.Error()), nil
		}
		id, err := fc.CreateFavorite(ctx, body)
		if err != nil {
			return mcpgo.NewToolResultError(err.Error()), nil
		}
		return favoriteResult(ctx, fc, id, fmt.Sprintf("Created favorite %d (%q).", id, body.Name)), nil
	}
}

// UpdateFavoriteHandler full-replaces a (single-sport, non-route) favorite.
func UpdateFavoriteHandler(fc *flow.Client) toolHandler {
	return func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		id, err := idArg(req, "favorite_id")
		if err != nil {
			return mcpgo.NewToolResultError(err.Error()), nil
		}
		create, err := buildFavoriteCreate(ctx, fc, req)
		if err != nil {
			// A goal-less call is how a rename of a route or multi-sport favorite
			// arrives here; no goal would make those editable, so say why
			// instead of asking for one.
			if errors.Is(err, errNoGoal) {
				if existing, gerr := fc.GetFavorite(ctx, id); gerr == nil {
					if msg := notReplaceable(existing, id); msg != "" {
						return mcpgo.NewToolResultError(msg), nil
					}
				}
			}
			return mcpgo.NewToolResultError(err.Error()), nil
		}
		existing, err := fc.GetFavorite(ctx, id)
		if err != nil {
			return favoriteErrorResult(err, "favorite_id", id), nil
		}
		if msg := notReplaceable(existing, id); msg != "" {
			return mcpgo.NewToolResultError(msg), nil
		}
		etID, ok := existing.ExerciseTargets[0].ID.Get()
		if !ok {
			return mcpgo.NewToolResultError(fmt.Sprintf("favorite %d has no exercise-target id to update", id)), nil
		}
		// Flow keys the update on exerciseTargets[].id and silently drops the
		// changes of an entry sent with a null id, so carry the live id over.
		entry := favoriteEntryFromTarget(create.ExerciseTargets[0])
		entry.ID.SetTo(etID)
		body := &gen.Favorite{
			Type:            gen.FavoriteType(create.Type),
			Name:            create.Name,
			ExerciseTargets: []gen.FavoriteExerciseTargetEntry{entry},
		}
		body.Description.SetTo(create.Description.Or(""))
		if err := fc.UpdateFavorite(ctx, id, body); err != nil {
			return favoriteErrorResult(err, "favorite_id", id), nil
		}
		return favoriteResult(ctx, fc, id, fmt.Sprintf("Updated favorite %d.", id)), nil
	}
}

// notReplaceable explains why update_favorite cannot full-replace a favorite,
// or returns "" when it can.
func notReplaceable(f *gen.Favorite, id int64) string {
	switch {
	case f.Type == gen.FavoriteTypeROUTE:
		return fmt.Sprintf("favorite %d is a ROUTE; update_favorite cannot edit routes — use rename_favorite "+
			"or set_favorite_sport, or import a new route", id)
	case len(f.ExerciseTargets) != 1:
		return fmt.Sprintf("favorite %d has %d exercise targets (multi-sport); update_favorite only replaces "+
			"single-sport favorites", id, len(f.ExerciseTargets))
	}
	return ""
}

// DeleteFavoriteHandler deletes a favorite or route, after confirmation.
func DeleteFavoriteHandler(fc *flow.Client) toolHandler {
	return func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		id, err := idArg(req, "favorite_id")
		if err != nil {
			return mcpgo.NewToolResultError(err.Error()), nil
		}
		f, err := fc.GetFavorite(ctx, id)
		if err != nil {
			return favoriteErrorResult(err, "favorite_id", id), nil
		}
		if result, proceed := requireConfirm(ctx, req, "delete_favorite", func() string {
			return fmt.Sprintf("Permanently delete the %s favorite %q (id %d)? This cannot be undone.",
				strings.ToLower(string(f.Type)), f.Name, id)
		}); !proceed {
			return result, nil
		}
		if err := fc.DeleteFavorite(ctx, id); err != nil {
			return favoriteErrorResult(err, "favorite_id", id), nil
		}
		result := mcpgo.NewToolResultText(fmt.Sprintf("Deleted favorite %d (%q).", id, f.Name))
		result.StructuredContent = map[string]any{"type": "favorite_deleted", "data": map[string]any{"favorite_id": id}}
		return result, nil
	}
}

// RenameFavoriteHandler renames a favorite or route.
func RenameFavoriteHandler(fc *flow.Client) toolHandler {
	return func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		id, err := idArg(req, "favorite_id")
		if err != nil {
			return mcpgo.NewToolResultError(err.Error()), nil
		}
		name, present, err := textArg(req, "name", maxFavoriteNameRunes, true)
		if err != nil {
			return mcpgo.NewToolResultError(err.Error()), nil
		}
		if !present {
			return mcpgo.NewToolResultError("name is required (1–45 characters)"), nil
		}
		// saveName answers an unknown id with a 500; check it exists first.
		if _, err := fc.GetFavorite(ctx, id); err != nil {
			return favoriteErrorResult(err, "favorite_id", id), nil
		}
		if err := fc.RenameFavorite(ctx, id, name); err != nil {
			return favoriteErrorResult(err, "favorite_id", id), nil
		}
		return favoriteResult(ctx, fc, id, fmt.Sprintf("Renamed favorite %d to %q.", id, name)), nil
	}
}

// SetFavoriteSportHandler changes the sport of one of a favorite's exercise
// targets. Flow validates neither id, so both are checked here and the change
// is verified by reading the favorite back.
func SetFavoriteSportHandler(fc *flow.Client) toolHandler {
	return func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		id, err := idArg(req, "favorite_id")
		if err != nil {
			return mcpgo.NewToolResultError(err.Error()), nil
		}
		sid, err := idArg(req, "sport_id")
		if err != nil {
			return mcpgo.NewToolResultError(err.Error()), nil
		}
		etArg, etGiven, err := intArgRange(req, "exercise_target_id", 1, 1<<52, "")
		if err != nil {
			return mcpgo.NewToolResultError(err.Error()), nil
		}
		if err := checkSport(ctx, fc, int(sid)); err != nil {
			return mcpgo.NewToolResultError(err.Error()), nil
		}
		f, err := fc.GetFavorite(ctx, id)
		if err != nil {
			return favoriteErrorResult(err, "favorite_id", id), nil
		}
		etID, errMsg := pickExerciseTarget(f, etArg, etGiven)
		if errMsg != "" {
			return mcpgo.NewToolResultError(errMsg), nil
		}
		if err := fc.ChangeFavoriteSport(ctx, id, etID, int(sid)); err != nil {
			return favoriteErrorResult(err, "favorite_id", id), nil
		}
		after, err := fc.GetFavorite(ctx, id)
		if err != nil {
			return mcpgo.NewToolResultText(fmt.Sprintf("Set sport %d on favorite %d (read-back failed: %v).", sid, id, err)), nil
		}
		for _, et := range after.ExerciseTargets {
			if v, ok := et.ID.Get(); ok && v == etID {
				if got, ok := et.SportId.Get(); !ok || got != int(sid) {
					return mcpgo.NewToolResultError(fmt.Sprintf(
						"Polar acknowledged the change but favorite %d still reads back sport %v", id, et.SportId)), nil
				}
			}
		}
		return widgetResultText(fmt.Sprintf("Set sport %d on favorite %d.", sid, id), map[string]any{
			"type": "favorite_detail", "favorite": convert.FromWireFavorite(id, after),
		}), nil
	}
}

// pickExerciseTarget resolves which exercise target of f to change: the given
// one (which must belong to f), or the only one.
func pickExerciseTarget(f *gen.Favorite, given int64, isGiven bool) (int64, string) {
	ids := make([]int64, 0, len(f.ExerciseTargets))
	for _, et := range f.ExerciseTargets {
		if v, ok := et.ID.Get(); ok {
			ids = append(ids, v)
		}
	}
	if isGiven {
		for _, v := range ids {
			if v == given {
				return v, ""
			}
		}
		return 0, fmt.Sprintf("exercise_target_id %d is not part of this favorite (its exercise targets: %v)", given, ids)
	}
	switch len(ids) {
	case 1:
		return ids[0], ""
	case 0:
		return 0, "this favorite has no exercise target to set a sport on"
	default:
		return 0, fmt.Sprintf("this favorite has %d exercise targets %v; pass exercise_target_id to pick one", len(ids), ids)
	}
}

// SaveTargetAsFavoriteHandler copies a scheduled training target into a new
// favorite, the way the web UI's "Ajouter aux favoris" button does.
func SaveTargetAsFavoriteHandler(fc *flow.Client) toolHandler {
	return func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		tid, err := idArg(req, "target_id")
		if err != nil {
			return mcpgo.NewToolResultError(err.Error()), nil
		}
		override, hasName, err := textArg(req, "name", maxFavoriteNameRunes, true)
		if err != nil {
			return mcpgo.NewToolResultError(err.Error()), nil
		}
		t, err := fc.GetTrainingTarget(ctx, tid)
		if err != nil {
			if errors.Is(err, flow.ErrTargetNotFound) {
				return mcpgo.NewToolResultText(fmt.Sprintf("No target with id %d.", tid)), nil
			}
			return mcpgo.NewToolResultError(err.Error()), nil
		}
		if t.Type == gen.GetTrainingTargetOKTypeROUTE {
			// Favorite create rejects the ROUTE type with a 500.
			return mcpgo.NewToolResultError(fmt.Sprintf("target %d is a ROUTE-type target (created from a route "+
				"favorite); it has no goal to save as a favorite", tid)), nil
		}
		name := t.Name
		if hasName {
			name = override
		} else if n := convert.PolarTextLen(name); n > maxFavoriteNameRunes {
			return mcpgo.NewToolResultError(fmt.Sprintf("the target's name is %d characters but favorite names "+
				"are limited to %d; pass a shorter name", n, maxFavoriteNameRunes)), nil
		}
		body := convert.FavoriteCreateFromTarget(t, name)
		id, err := fc.CreateFavorite(ctx, body)
		if err != nil {
			return mcpgo.NewToolResultError(err.Error()), nil
		}
		return favoriteResult(ctx, fc, id, fmt.Sprintf("Saved target %d as favorite %d (%q).", tid, id, name)), nil
	}
}

// ScheduleFavoriteHandler creates a training target from a favorite on a date.
func ScheduleFavoriteHandler(fc *flow.Client) toolHandler {
	return func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		id, err := idArg(req, "favorite_id")
		if err != nil {
			return mcpgo.NewToolResultError(err.Error()), nil
		}
		date, hasDate, err := stringArg(req, "date")
		if err != nil {
			return mcpgo.NewToolResultError(err.Error()), nil
		}
		if !hasDate || strings.TrimSpace(date) == "" {
			return mcpgo.NewToolResultError("date is required (YYYY-MM-DD)"), nil
		}
		clock, hasClock, err := stringArg(req, "time")
		if err != nil {
			return mcpgo.NewToolResultError(err.Error()), nil
		}
		if !hasClock {
			clock = "18:00"
		}
		when, err := convert.ParseLocalDateTime(date, clock)
		if err != nil {
			return mcpgo.NewToolResultError(err.Error()), nil
		}
		if when.Hour() == 0 && when.Minute() == 0 {
			return mcpgo.NewToolResultError("time 00:00 cannot be scheduled: Polar reads midnight as \"no time\" " +
				"and moves the target to 18:00 — use 00:01 or another time"), nil
		}
		f, err := fc.GetFavorite(ctx, id)
		if err != nil {
			return favoriteErrorResult(err, "favorite_id", id), nil
		}
		if f.Type == gen.FavoriteTypeROUTE {
			// Flow accepts it, but creates a goal-less ROUTE-type target that
			// the diary's picker never offers; keep to what the UI does.
			return mcpgo.NewToolResultError(fmt.Sprintf("favorite %d is a ROUTE; only training-target favorites "+
				"can be scheduled (Polar would create an empty ROUTE-type target with no goal)", id)), nil
		}
		t, err := fc.CreateTargetFromFavorite(ctx, id, when)
		if err != nil {
			return favoriteErrorResult(err, "favorite_id", id), nil
		}
		dto := convert.FromWireTargetFromFavorite(t)
		msg := fmt.Sprintf("Scheduled favorite %d as training target %d on %s at %s.", id, dto.TargetID, dto.Date, dto.Time)
		if want := when.Format("15:04"); dto.Time != "" && dto.Time != want {
			msg += fmt.Sprintf(" Polar moved it from %s because another target already starts then.", want)
		}
		return widgetResultText(msg, map[string]any{"type": "favorite_scheduled", "target": dto}), nil
	}
}
