package convert

import (
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-faster/jx"

	"github.com/lmgarret/polar-flow-mcp/internal/flow/gen"
)

func TestFeelingToWire(t *testing.T) {
	// Captured from the edit form's dropdown (2026-09-28).
	want := map[int]string{5: "0.19", 4: "0.39", 3: "0.59", 2: "0.79", 1: "0.99"}
	for rating, wire := range want {
		got, ok := FeelingToWire(rating)
		if !ok || got != wire {
			t.Fatalf("FeelingToWire(%d) = %q,%v want %q", rating, got, ok, wire)
		}
	}
	for _, bad := range []int{0, 6, -1} {
		if _, ok := FeelingToWire(bad); ok {
			t.Fatalf("FeelingToWire(%d) accepted", bad)
		}
	}
}

func TestFeelingFromWire(t *testing.T) {
	tests := []struct {
		in   float64
		want int // 0 = nil
	}{
		{0.19, 5}, {0.39, 4}, {0.59, 3}, {0.79, 2}, {0.99, 1},
		{0.5, 3},  // off-scale value stored verbatim by Flow → nearest bucket
		{1.0, 1},  // accepted by Flow
		{0.0, 0},  // "cleared / invalid input"
		{-0.1, 0}, // not a Flow value
		{1.5, 0},
	}
	for _, tt := range tests {
		got := FeelingFromWire(tt.in)
		if tt.want == 0 {
			if got != nil {
				t.Fatalf("FeelingFromWire(%v) = %d, want nil", tt.in, *got)
			}
			continue
		}
		if got == nil || *got != tt.want {
			t.Fatalf("FeelingFromWire(%v) = %v, want %d", tt.in, got, tt.want)
		}
	}
	// Round-trip every rating.
	for r := 1; r <= 5; r++ {
		w, _ := FeelingToWire(r)
		f, err := strconv.ParseFloat(w, 64)
		if err != nil {
			t.Fatalf("wire %q: %v", w, err)
		}
		if got := FeelingFromWire(f); got == nil || *got != r {
			t.Fatalf("round-trip %d → %s → %v", r, w, got)
		}
	}
}

func TestClockToSeconds(t *testing.T) {
	tests := []struct {
		in   string
		want int
		ok   bool
	}{
		{"00:30:00", 1800, true},
		{"99:59:59", 359999, true},
		{"00:00:00", 0, true},
		{"30:00", 0, false},
		{"00:60:00", 0, false},
		{"aa:00:00", 0, false},
		{"", 0, false},
	}
	for _, tt := range tests {
		got, ok := ClockToSeconds(tt.in)
		if got != tt.want || ok != tt.ok {
			t.Fatalf("ClockToSeconds(%q) = %d,%v want %d,%v", tt.in, got, ok, tt.want, tt.ok)
		}
	}
}

func TestParseDashDMY(t *testing.T) {
	if got, ok := ParseDashDMY("28-09-2026"); !ok || got != "2026-09-28" {
		t.Fatalf("ParseDashDMY = %q,%v", got, ok)
	}
	for _, bad := range []string{"2026-09-28", "28.9.2026", ""} {
		if _, ok := ParseDashDMY(bad); ok {
			t.Fatalf("ParseDashDMY(%q) accepted", bad)
		}
	}
}

func TestWireFavouriteTo(t *testing.T) {
	// The only `to` form createTargetFromFavourite accepts: milliseconds and a
	// numeric offset. The wall clock is kept whatever the location.
	loc := time.FixedZone("x", 2*3600)
	got := WireFavouriteTo(time.Date(2026, 10, 20, 9, 30, 0, 0, loc))
	if got != "2026-10-20T09:30:00.000+00:00" {
		t.Fatalf("WireFavouriteTo = %q", got)
	}
}

// Captured /api/favorites entries (template + route), trimmed.
const favoritesListingJSON = `{"hasErrors":false,"linkedServices":{"strava":false},"targets":[
 {"exerciseTargetId":1539651421,"favoriteId":83468869,"sportName":"RUNNING","sportId":1,"type":"PHASED","segmentId":null,"favoriteName":"probe-fav-src","favoriteDescription":"src desc","duration":600000,"calories":null,"distance":2000.0,"supportedDevices":[],"supportedDeviceIds":[],"routeSource":null,"externalRouteIdentifier":null,"segmentType":null},
 {"exerciseTargetId":1539651678,"favoriteId":83468958,"sportName":null,"sportId":null,"type":"ROUTE","segmentId":null,"favoriteName":"probe-route","favoriteDescription":null,"duration":null,"calories":null,"distance":266.20719585231143,"supportedDevices":[],"supportedDeviceIds":[],"routeSource":"FLOW_FILE_IMPORT","externalRouteIdentifier":"3d0cd881-a910-4205-860e-9d016d05d268","segmentType":null}]}`

func TestFromWireFavoriteList(t *testing.T) {
	var l gen.FavoritesListing
	if err := l.Decode(jx.DecodeStr(favoritesListingJSON)); err != nil {
		t.Fatalf("decode: %v", err)
	}
	items := FromWireFavoriteList(l.Targets)
	if len(items) != 2 {
		t.Fatalf("items = %d", len(items))
	}
	tpl, route := items[0], items[1]
	if tpl.FavoriteID != 83468869 || tpl.ExerciseTargetID != 1539651421 || tpl.Type != "PHASED" {
		t.Fatalf("template ids/type wrong: %+v", tpl)
	}
	if tpl.DurationS == nil || *tpl.DurationS != 600 {
		t.Fatalf("template duration_s = %v, want 600 (from 600000 ms)", tpl.DurationS)
	}
	if tpl.SportID == nil || *tpl.SportID != 1 || tpl.SportCategory == "" {
		t.Fatalf("template sport wrong: %+v", tpl)
	}
	if route.SportID != nil || route.DurationS != nil || route.Description != "" || route.RouteSource != "FLOW_FILE_IMPORT" {
		t.Fatalf("route nulls wrong: %+v", route)
	}
}

// Captured createTargetFromFavourite responses (text/plain JSON).
func TestFromWireTargetFromFavorite(t *testing.T) {
	tests := []struct {
		name, body string
		want       ScheduledTarget
	}{
		{
			name: "phased, default time",
			body: `{"id":1461116717,"name":"probe-fav-src","sport":{"id":1,"name":"RUNNING","localizedName":null,"defaultSpeedViewSetting":"PACE","sportType":"SINGLE_SPORT","iconUrl":null,"powerZonesRelevant":true,"tapActive":true},"iconUrl":"https://x","description":"src desc","date":"28-09-2026","time":"18:00","distance":"2000.0","duration":"600000","kiloCalories":null,"url":"/target/1461116717"}`,
			want: ScheduledTarget{TargetID: 1461116717, Name: "probe-fav-src", Date: "2026-09-28", Time: "18:00",
				Start: "2026-09-28T18:00", SportID: ptr(1), SportCategory: "run", DurationS: ptr(600), DistanceM: ptrF(2000)},
		},
		{
			name: "calories only: distance and duration are \"0\"",
			body: `{"id":1461117227,"name":"kcal-probe","sport":{"id":1,"name":"RUNNING"},"description":"","date":"12-11-2026","time":"07:00","distance":"0","duration":"0","kiloCalories":"450","url":"/target/1461117227"}`,
			want: ScheduledTarget{TargetID: 1461117227, Name: "kcal-probe", Date: "2026-11-12", Time: "07:00",
				Start: "2026-11-12T07:00", SportID: ptr(1), SportCategory: "run", Calories: ptr(450)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var v gen.TargetFromFavorite
			if err := v.Decode(jx.DecodeStr(tt.body)); err != nil {
				t.Fatalf("decode: %v", err)
			}
			got := FromWireTargetFromFavorite(&v)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got  %+v\nwant %+v", got, tt.want)
			}
		})
	}
}

// Captured GET /api/trainingtarget/{id} for the target saved as a favorite,
// and the POST /api/favoritetarget body the web UI built from it.
const targetForFavoriteJSON = `{"datetime":"2026-10-15T09:00","sourceProgramType":"UNKNOWN","isUserEditable":true,"name":"probe-fav-src","exerciseTargets":[{"id":1539651413,"duration":"00:10:00","distance":2000.0,"calories":null,"index":0,"sportId":1,"phases":[{"id":2929451679,"lowerZone":1,"upperZone":2,"intensityType":"HEART_RATE_ZONES","phaseChangeType":"AUTOMATIC","goalType":"DURATION","duration":"00:10:00","name":"Warm-up","phaseType":"PHASE"},{"id":2929451680,"lowerZone":3,"upperZone":3,"intensityType":"HEART_RATE_ZONES","phaseChangeType":"AUTOMATIC","goalType":"DISTANCE","duration":"00:00:00","distance":2000.0,"name":"Work","phaseType":"PHASE"}]}],"description":"src desc","type":"PHASED"}`

const uiFavoriteFromTargetJSON = `{"type":"PHASED","name":"probe-fav-src","description":"src desc","exerciseTargets":[{"id":null,"distance":null,"calories":null,"index":0,"sportId":1,"phases":[{"id":2929451679,"lowerZone":1,"upperZone":2,"intensityType":"HEART_RATE_ZONES","phaseChangeType":"AUTOMATIC","goalType":"DURATION","duration":"00:10:00","name":"Warm-up","phaseType":"PHASE"},{"id":2929451680,"lowerZone":3,"upperZone":3,"intensityType":"HEART_RATE_ZONES","phaseChangeType":"AUTOMATIC","goalType":"DISTANCE","duration":"00:00:00","distance":2000,"name":"Work","phaseType":"PHASE"}],"duration":null}]}`

// stripKeys removes the given keys at any depth (to compare modulo fields the
// tool deliberately leaves out: phase ids and the read-only index).
func stripKeys(v any, keys ...string) any {
	switch x := v.(type) {
	case map[string]any:
		for _, k := range keys {
			delete(x, k)
		}
		for k, c := range x {
			x[k] = stripKeys(c, keys...)
		}
	case []any:
		for i, c := range x {
			x[i] = stripKeys(c, keys...)
		}
	}
	return v
}

func TestFavoriteCreateFromTarget_MatchesWebUI(t *testing.T) {
	var tgt gen.GetTrainingTargetOK
	if err := tgt.Decode(jx.DecodeStr(targetForFavoriteJSON)); err != nil {
		t.Fatalf("decode target: %v", err)
	}
	fav := FavoriteCreateFromTarget(&tgt, tgt.Name)
	var e jx.Encoder
	fav.Encode(&e)
	got := mustJSON(t, e.String())
	want := mustJSON(t, uiFavoriteFromTargetJSON)
	// The UI copies phase ids and the read-only index; the tool lets the server
	// assign fresh ones (it does anyway). Everything else must be identical.
	stripKeys(want, "index")
	wantMap, _ := want.(map[string]any)
	ets, _ := wantMap["exerciseTargets"].([]any)
	et0, _ := ets[0].(map[string]any)
	stripKeys(et0["phases"], "id")
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("favorite body mismatch\n got: %s\nwant: %s", e.String(), uiFavoriteFromTargetJSON)
	}
}

func TestFavoriteCreateFromTarget_VolumeKeepsGoal(t *testing.T) {
	const vol = `{"datetime":"2026-10-15T09:00","name":"Easy","exerciseTargets":[{"id":7,"duration":"00:45:00","distance":null,"calories":null,"index":0,"sportId":1,"phases":[]}],"description":null,"type":"VOLUME"}`
	var tgt gen.GetTrainingTargetOK
	if err := tgt.Decode(jx.DecodeStr(vol)); err != nil {
		t.Fatalf("decode: %v", err)
	}
	fav := FavoriteCreateFromTarget(&tgt, "Easy")
	et := fav.ExerciseTargets[0]
	if d, ok := et.Duration.Get(); !ok || d != "00:45:00" {
		t.Fatalf("VOLUME duration lost: %v", et.Duration)
	}
	if !et.ID.Null {
		t.Fatalf("exercise target id must be null, got %v", et.ID)
	}
	if desc, _ := fav.Description.Get(); desc != "" {
		t.Fatalf("null description must become \"\", got %q", desc)
	}
}

// Captured summary of a manual session (ids and names anonymised).
const manualSummaryJSON = `{"id":8429796771,"nextTrainingId":-1,"previousTrainingId":-1,"userId":11111111,"feeling":0.39,"note":"edited note","latitude":null,"longitude":null,"startDate":"2026-09-20T08:00:00","stopTime":"2026-09-20T08:30:00","duration":"PT30M","distance":null,"kiloCalories":300,"trainingLoad":null,"trainingBenefit":null,"carboPercentage":null,"fatPercentage":null,"proteinPercentage":null,"hrMax":150,"hrAverage":140,"timezoneOffset":null,"trainingLoadPro":null,"exercises":{"8459583326":{"id":8459583326,"sport":{"id":1},"startTime":"2026-09-20T08:00:00","stopTime":"2026-09-20T08:30:00","duration":"PT30M","distance":5000.0,"calories":300,"trainingStatistic":{"SPEED":{"max":null,"avg":10.0,"min":null},"HEART_RATE":{"max":150.0,"avg":140.0,"min":null}},"sportParent":"RUNNING","defaultSpeedViewSettings":"PACE"}},"periodDataUuid":null,"deviceName":"","trainingSessionName":"probe-edit-renamed"}`

// The body built from the live summary must be exactly what the web UI's edit
// form sends when nothing is changed (minus the synthesized hrSamples, which
// the server does not need, and feeling, which is left unchanged).
func TestEditBodyFromSummary_MatchesWebUIForm(t *testing.T) {
	var s gen.SessionSummary
	if err := s.Decode(jx.DecodeStr(manualSummaryJSON)); err != nil {
		t.Fatalf("decode: %v", err)
	}
	body, err := EditBodyFromSummary(&s)
	if err != nil {
		t.Fatalf("EditBodyFromSummary: %v", err)
	}
	var e jx.Encoder
	body.Encode(&e)
	got := mustJSON(t, e.String())
	want := mustJSON(t, `{"sport":1,"duration":1800,"distance":5000,"hrAverage":140,"note":"edited note","trainingSessionName":"probe-edit-renamed","hrMax":150,"kiloCalories":300,"editedExerciseId":null,"speedAverage":10}`)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("edit body mismatch\n got: %s", e.String())
	}
}

func TestEditBodyFromSummary_Refuses(t *testing.T) {
	tests := []struct {
		name string
		mut  func(string) string
	}{
		{"device recorded", func(s string) string {
			return strings.Replace(s, `"deviceName":""`, `"deviceName":"Polar Pacer Pro"`, 1)
		}},
		{"multi exercise", func(s string) string {
			return strings.Replace(s, `"exercises":{"8459583326":`, `"exercises":{"1":{"id":1,"sport":{"id":2}},"8459583326":`, 1)
		}},
		{"no exercises", func(s string) string {
			i := strings.Index(s, `"exercises":`)
			j := strings.Index(s, `,"periodDataUuid"`)
			return s[:i] + `"exercises":{}` + s[j:]
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var s gen.SessionSummary
			if err := s.Decode(jx.DecodeStr(tt.mut(manualSummaryJSON))); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if _, err := EditBodyFromSummary(&s); err != ErrSessionNotEditable {
				t.Fatalf("err = %v, want ErrSessionNotEditable", err)
			}
		})
	}
}

func TestFromWireSessionSummary_FeelingAndSport(t *testing.T) {
	var s gen.SessionSummary
	if err := s.Decode(jx.DecodeStr(manualSummaryJSON)); err != nil {
		t.Fatalf("decode: %v", err)
	}
	dto := FromWireSessionSummary(&s)
	if dto.Feeling == nil || *dto.Feeling != 4 {
		t.Fatalf("feeling = %v, want 4 (0.39)", dto.Feeling)
	}
	if dto.SportID == nil || *dto.SportID != 1 {
		t.Fatalf("sport_id = %v, want 1 from the exercise", dto.SportID)
	}
}

// Captured route read-back (trimmed to 5 points).
const routeGeometryJSON = `{"distance":532.4104430899847,"duration":null,"id":1530637488,"index":null,"kiloCalories":null,"modified":null,"created":null,"type":"ROUTE","routeServiceRoute":null,"segmentId":null,"favoritePhases":[],"sport":{"id":1,"name":"RUNNING"},"gpsRoute":{"id":null,"user":{"id":11111111},"name":"Geometry-hunt route","parentRouteId":null,"order":null,"distance":532.4104430899847,"startPoint":{"longitude":2.3522,"latitude":48.8566,"altitude":35,"time":0},"waypoints":[{"longitude":2.3522,"latitude":48.8566,"altitude":35,"time":0},{"longitude":2.3532,"latitude":48.8576,"altitude":null,"time":1000},{"longitude":2.3542,"latitude":48.8586,"altitude":37,"time":2000},{"longitude":2.3552,"latitude":48.8596,"altitude":38,"time":3000},{"longitude":2.3562,"latitude":48.8606,"altitude":39,"time":4000}]}}`

func TestFromWireRouteGeometry(t *testing.T) {
	var et gen.FavoriteExerciseTarget
	if err := et.Decode(jx.DecodeStr(routeGeometryJSON)); err != nil {
		t.Fatalf("decode: %v", err)
	}
	tests := []struct {
		max      int
		wantLats []float64
	}{
		{0, []float64{48.8566, 48.8576, 48.8586, 48.8596, 48.8606}},
		{500, []float64{48.8566, 48.8576, 48.8586, 48.8596, 48.8606}},
		{3, []float64{48.8566, 48.8586, 48.8606}},
		{2, []float64{48.8566, 48.8606}},
	}
	for _, tt := range tests {
		geo, ok := FromWireRouteGeometry(&et, tt.max)
		if !ok {
			t.Fatalf("not a route")
		}
		if geo.PointCount != 5 || geo.ReturnedPoints != len(tt.wantLats) {
			t.Fatalf("max %d: counts %d/%d", tt.max, geo.PointCount, geo.ReturnedPoints)
		}
		for i, lat := range tt.wantLats {
			if geo.Points[i].Lat != lat {
				t.Fatalf("max %d: point %d lat %v, want %v", tt.max, i, geo.Points[i].Lat, lat)
			}
		}
		if geo.Name != "Geometry-hunt route" || geo.SportID == nil || *geo.SportID != 1 || geo.ExerciseTargetID != 1530637488 {
			t.Fatalf("metadata wrong: %+v", geo)
		}
	}
	geo, _ := FromWireRouteGeometry(&et, 0)
	if geo.Points[1].AltitudeM != nil {
		t.Fatalf("null altitude must stay null")
	}
	// A non-route exercise target has gpsRoute null.
	var plain gen.FavoriteExerciseTarget
	if err := plain.Decode(jx.DecodeStr(`{"id":1,"type":"VOLUME","gpsRoute":null,"sport":null}`)); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := FromWireRouteGeometry(&plain, 0); ok {
		t.Fatalf("non-route reported as route")
	}
}

func ptr(v int) *int          { return &v }
func ptrF(v float64) *float64 { return &v }
