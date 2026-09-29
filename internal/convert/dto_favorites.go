package convert

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/lmgarret/polar-flow-mcp/internal/flow/gen"
)

// Canonical DTOs for favorites, routes, scheduled favorites and session
// feeling. Same conventions as dto.go: seconds (*_s), metres (*_m), ISO dates,
// real nulls. Phase trees stay pass-through (same pragmatic scope as targets).

// FavoriteListItem is one entry of list_favorites, mapped from
// gen.FavoriteListing (whose duration is milliseconds, whose sportName is the
// UPPERCASE constant, and whose description is "" for templates / null for
// routes).
type FavoriteListItem struct {
	FavoriteID       int64    `json:"favorite_id"`
	ExerciseTargetID int64    `json:"exercise_target_id"`
	Name             string   `json:"name"`
	Description      string   `json:"description,omitempty"`
	Type             string   `json:"type"`
	SportID          *int     `json:"sport_id"`
	SportName        string   `json:"sport_name,omitempty"`
	SportCategory    string   `json:"sport_category,omitempty"`
	DurationS        *int     `json:"duration_s"`
	DistanceM        *float64 `json:"distance_m"`
	Calories         *int     `json:"calories"`
	RouteSource      string   `json:"route_source,omitempty"`
}

// FromWireFavoriteList maps /api/favorites targets to canonical items.
func FromWireFavoriteList(in []gen.FavoriteListing) []FavoriteListItem {
	out := make([]FavoriteListItem, 0, len(in))
	for _, f := range in {
		item := FavoriteListItem{
			FavoriteID:       f.FavoriteId.Or(0),
			ExerciseTargetID: f.ExerciseTargetId.Or(0),
			Name:             f.FavoriteName.Or(""),
			Type:             string(f.Type.Or("")),
		}
		if v, ok := f.FavoriteDescription.Get(); ok {
			item.Description = v
		}
		if v, ok := f.SportId.Get(); ok {
			item.SportID = &v
		}
		if v, ok := f.SportName.Get(); ok {
			item.SportName = v
		}
		item.SportCategory = SportCategory(item.SportName, derefInt(item.SportID))
		if v, ok := f.Duration.Get(); ok {
			secs := MillisToSeconds(int64(v))
			item.DurationS = &secs
		}
		if v, ok := f.Distance.Get(); ok {
			item.DistanceM = &v
		}
		if v, ok := f.Calories.Get(); ok {
			item.Calories = &v
		}
		if v, ok := f.RouteSource.Get(); ok {
			item.RouteSource = v
		}
		out = append(out, item)
	}
	return out
}

// FavoriteExerciseTargetDTO is one exercise block of a favorite.
type FavoriteExerciseTargetDTO struct {
	ExerciseTargetID *int64      `json:"exercise_target_id"`
	SportID          *int        `json:"sport_id"`
	SportCategory    string      `json:"sport_category,omitempty"`
	DurationS        *int        `json:"duration_s"`
	DistanceM        *float64    `json:"distance_m"`
	Calories         *float64    `json:"calories"`
	Phases           []gen.Phase `json:"phases"`
}

// FavoriteDetail is the canonical shape for get_favorite, mapped from
// gen.Favorite (durations HH:MM:SS → seconds; PHASED read-backs carry
// server-rolled-up totals).
type FavoriteDetail struct {
	FavoriteID      int64                       `json:"favorite_id"`
	Name            string                      `json:"name"`
	Description     string                      `json:"description,omitempty"`
	Type            string                      `json:"type"`
	ExerciseTargets []FavoriteExerciseTargetDTO `json:"exercise_targets"`
}

// FromWireFavorite maps gen.Favorite to the canonical FavoriteDetail.
func FromWireFavorite(id int64, f *gen.Favorite) FavoriteDetail {
	out := FavoriteDetail{FavoriteID: id, ExerciseTargets: []FavoriteExerciseTargetDTO{}}
	if f == nil {
		return out
	}
	out.Name = f.Name
	out.Type = string(f.Type)
	if v, ok := f.Description.Get(); ok {
		out.Description = v
	}
	for _, et := range f.ExerciseTargets {
		dto := FavoriteExerciseTargetDTO{Phases: et.Phases}
		if dto.Phases == nil {
			dto.Phases = []gen.Phase{}
		}
		if v, ok := et.ID.Get(); ok {
			dto.ExerciseTargetID = &v
		}
		if v, ok := et.SportId.Get(); ok {
			dto.SportID = &v
		}
		dto.SportCategory = SportCategory("", derefInt(dto.SportID))
		if v, ok := et.Duration.Get(); ok {
			if secs, parsed := ClockToSeconds(v); parsed {
				dto.DurationS = &secs
			}
		}
		if v, ok := et.Distance.Get(); ok {
			dto.DistanceM = &v
		}
		if v, ok := et.Calories.Get(); ok {
			dto.Calories = &v
		}
		out.ExerciseTargets = append(out.ExerciseTargets, dto)
	}
	return out
}

// FavoriteCreateFromTarget copies a training target read back from
// GET /api/trainingtarget/{id} into a FavoriteCreate — what the web UI's
// "Ajouter aux favoris" does client-side: drop the datetime, null each
// exercise target's id (the server assigns new ones), and for PHASED targets
// drop the rolled-up duration/distance totals (the phases carry the goals).
// Phase ids are cleared too so the server assigns fresh ones, exactly as it
// does for a new favorite.
func FavoriteCreateFromTarget(t *gen.GetTrainingTargetOK, name string) *gen.FavoriteCreate {
	fav := &gen.FavoriteCreate{
		Type: gen.FavoriteCreateType(t.Type),
		Name: name,
	}
	if v, ok := t.Description.Get(); ok {
		fav.Description.SetTo(v)
	} else {
		fav.Description.SetTo("")
	}
	for _, et := range t.ExerciseTargets {
		c := gen.ExerciseTarget{
			SportId:  et.SportId,
			Calories: et.Calories,
			Phases:   clearPhaseIDs(et.Phases),
		}
		c.ID.SetToNull()
		if t.Type == gen.GetTrainingTargetOKTypePHASED {
			c.Duration.SetToNull()
			c.Distance.SetToNull()
		} else {
			c.Duration = et.Duration
			c.Distance = et.Distance
		}
		fav.ExerciseTargets = append(fav.ExerciseTargets, c)
	}
	return fav
}

// clearPhaseIDs returns a copy of phases with every (nested) phase id unset.
func clearPhaseIDs(in []gen.Phase) []gen.Phase {
	if in == nil {
		return []gen.Phase{}
	}
	out := make([]gen.Phase, len(in))
	for i, p := range in {
		switch p.Type {
		case gen.PhaseLeafPhase:
			p.PhaseLeaf.ID.Reset()
		case gen.PhaseRepeatPhase:
			p.PhaseRepeat.ID.Reset()
			leaves := make([]gen.PhaseLeaf, len(p.PhaseRepeat.Phases))
			for j, l := range p.PhaseRepeat.Phases {
				l.ID.Reset()
				leaves[j] = l
			}
			p.PhaseRepeat.Phases = leaves
		}
		out[i] = p
	}
	return out
}

// ScheduledTarget is the canonical result of schedule_favorite, mapped from
// gen.TargetFromFavorite (date DD-MM-YYYY, distance as a string of metres,
// duration as a string of milliseconds, "0" meaning unset).
type ScheduledTarget struct {
	TargetID      int64    `json:"target_id"`
	Name          string   `json:"name"`
	Date          string   `json:"date"`
	Time          string   `json:"time"`
	Start         string   `json:"start"`
	SportID       *int     `json:"sport_id"`
	SportCategory string   `json:"sport_category,omitempty"`
	DurationS     *int     `json:"duration_s"`
	DistanceM     *float64 `json:"distance_m"`
	Calories      *int     `json:"calories"`
}

// FromWireTargetFromFavorite maps the schedule response to ScheduledTarget.
func FromWireTargetFromFavorite(t *gen.TargetFromFavorite) ScheduledTarget {
	out := ScheduledTarget{}
	if t == nil {
		return out
	}
	out.TargetID = t.ID
	out.Name = t.Name.Or("")
	out.Time = t.Time.Or("")
	if d, ok := ParseDashDMY(t.Date.Or("")); ok {
		out.Date = d
		if out.Time != "" {
			out.Start = d + "T" + out.Time
		}
	}
	sportName := ""
	if s, ok := t.Sport.Get(); ok {
		if v, ok := s.ID.Get(); ok {
			out.SportID = &v
		}
		sportName = s.Name.Or("")
	}
	out.SportCategory = SportCategory(sportName, derefInt(out.SportID))
	if v, ok := t.Duration.Get(); ok {
		if ms, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil && ms > 0 {
			secs := MillisToSeconds(int64(ms))
			out.DurationS = &secs
		}
	}
	if v, ok := t.Distance.Get(); ok {
		if m, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil && m > 0 {
			out.DistanceM = &m
		}
	}
	if v, ok := t.KiloCalories.Get(); ok {
		if k, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && k > 0 {
			out.Calories = &k
		}
	}
	return out
}

// ToWireRouteImport builds the importRoute body from a parsed route in the
// shape the web UI sends: {route:[{latitude,longitude,altitude,distance[,time]}],
// sport, name, distance}.
func ToWireRouteImport(r *ParsedRoute, name string, sportID *int) *gen.RouteImport {
	body := &gen.RouteImport{
		Route:    make([]gen.RoutePoint, 0, len(r.Points)),
		Name:     name,
		Distance: r.DistanceM,
	}
	if sportID != nil {
		body.Sport.SetTo(*sportID)
	} else {
		body.Sport.SetToNull()
	}
	for _, p := range r.Points {
		rp := gen.RoutePoint{Latitude: p.Lat, Longitude: p.Lon}
		if p.AltitudeM != nil {
			rp.Altitude.SetTo(*p.AltitudeM)
		} else {
			rp.Altitude.SetToNull()
		}
		rp.Distance.SetTo(p.DistanceM)
		if p.Time != "" {
			rp.Time.SetTo(p.Time)
		}
		body.Route = append(body.Route, rp)
	}
	return body
}

// RouteGeometryPoint is one read-back waypoint.
type RouteGeometryPoint struct {
	Lat       float64  `json:"lat"`
	Lon       float64  `json:"lon"`
	AltitudeM *float64 `json:"altitude_m"`
}

// RouteGeometry is the canonical result of get_route, mapped from
// gen.FavoriteExerciseTarget. Flow's synthesized per-point `time` (a 1000 ms
// sequence index, not elapsed time) is dropped. Points may be down-sampled:
// PointCount is always the full count, Points holds what was returned.
type RouteGeometry struct {
	ExerciseTargetID int64                `json:"exercise_target_id"`
	FavoriteID       *int64               `json:"favorite_id,omitempty"`
	Name             string               `json:"name"`
	DistanceM        float64              `json:"distance_m"`
	SportID          *int                 `json:"sport_id"`
	SportName        string               `json:"sport_name,omitempty"`
	SportCategory    string               `json:"sport_category,omitempty"`
	PointCount       int                  `json:"point_count"`
	ReturnedPoints   int                  `json:"returned_points"`
	Start            *RouteGeometryPoint  `json:"start,omitempty"`
	Points           []RouteGeometryPoint `json:"points"`
}

// FromWireRouteGeometry maps an exercise target with a gpsRoute to
// RouteGeometry, evenly down-sampling to at most maxPoints points (first and
// last always kept; maxPoints <= 0 means all). ok=false when the exercise
// target carries no route geometry (not a ROUTE favorite).
func FromWireRouteGeometry(et *gen.FavoriteExerciseTarget, maxPoints int) (RouteGeometry, bool) {
	out := RouteGeometry{Points: []RouteGeometryPoint{}}
	if et == nil {
		return out, false
	}
	route, ok := et.GpsRoute.Get()
	if !ok {
		return out, false
	}
	out.ExerciseTargetID = et.ID.Or(0)
	out.Name = route.Name.Or("")
	out.DistanceM = route.Distance
	if len(et.Sport) > 0 && string(et.Sport) != "null" {
		var s struct {
			ID   *int   `json:"id"`
			Name string `json:"name"`
		}
		if json.Unmarshal(et.Sport, &s) == nil {
			out.SportID = s.ID
			out.SportName = s.Name
		}
	}
	out.SportCategory = SportCategory(out.SportName, derefInt(out.SportID))
	out.PointCount = len(route.Waypoints)
	if out.PointCount > 0 {
		sp := wirePoint(route.StartPoint)
		out.Start = &sp
	}
	for _, i := range sampleIndexes(out.PointCount, maxPoints) {
		out.Points = append(out.Points, wirePoint(route.Waypoints[i]))
	}
	out.ReturnedPoints = len(out.Points)
	return out, true
}

func wirePoint(w gen.Waypoint) RouteGeometryPoint {
	p := RouteGeometryPoint{Lat: w.Latitude, Lon: w.Longitude}
	if v, ok := w.Altitude.Get(); ok {
		p.AltitudeM = &v
	}
	return p
}

// sampleIndexes returns up to maxN evenly spaced indexes into [0, n), always
// including the first and last. maxN <= 0 or maxN >= n returns every index.
func sampleIndexes(n, maxN int) []int {
	if n <= 0 {
		return nil
	}
	if maxN <= 0 || maxN >= n {
		idx := make([]int, n)
		for i := range idx {
			idx[i] = i
		}
		return idx
	}
	if maxN == 1 {
		return []int{0}
	}
	idx := make([]int, 0, maxN)
	for k := 0; k < maxN; k++ {
		idx = append(idx, k*(n-1)/(maxN-1))
	}
	return idx
}
