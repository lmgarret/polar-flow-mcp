//go:build live

package mcp

// Live round-trips against flow.polar.com for the favorites, route and
// session-edit tools: create → read back → update → read back → delete, with
// cleanup. Writes real (temporary) data, so it only builds with -tags live and
// only runs when POLAR_LIVE_JAR points at a cookie jar (COOKIE_JAR_PATH
// format) holding a valid FLOW_SESSION — it never logs in with a password:
//
//	POLAR_LIVE_JAR=/path/jar.json CGO_ENABLED=0 go test -tags live -run Live -v ./internal/mcp/
//
// Use a test account.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	mcpgo "github.com/mark3labs/mcp-go/mcp"

	"github.com/lmgarret/polar-flow-mcp/internal/flow"
)

func liveClient(t *testing.T) *flow.Client {
	t.Helper()
	jar := os.Getenv("POLAR_LIVE_JAR")
	if jar == "" {
		t.Skip("POLAR_LIVE_JAR not set")
	}
	fc, err := flow.New(context.Background(), flow.Config{JarPath: jar, HTTPTimeout: 60 * time.Second})
	if err != nil {
		t.Fatalf("flow.New: %v", err)
	}
	return fc
}

// liveCall runs a tool handler and fails on a tool error.
func liveCall(t *testing.T, h toolHandler, args map[string]any) *mcpgo.CallToolResult {
	t.Helper()
	res := callTool(t, h, args)
	if res.IsError {
		t.Fatalf("tool error: %s", resultText(res))
	}
	return res
}

// payload returns the tool result's structured payload as generic JSON.
func payload(t *testing.T, res *mcpgo.CallToolResult) map[string]any {
	t.Helper()
	b, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal structured content: %v", err)
	}
	return m
}

func asMap(v any) map[string]any { m, _ := v.(map[string]any); return m }
func asArr(v any) []any          { a, _ := v.([]any); return a }
func asNum(v any) float64        { f, _ := v.(float64); return f }

func dig(m map[string]any, path ...string) any {
	var cur any = m
	for _, p := range path {
		mm, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = mm[p]
	}
	return cur
}

//nolint:gocyclo // one sequential live scenario
func TestLiveFavoritesRoundTrip(t *testing.T) {
	fc := liveClient(t)
	ctx := context.Background()
	suffix := time.Now().Format("150405")

	// create → read back
	res := liveCall(t, CreateFavoriteHandler(fc), map[string]any{
		"name": "live-fav-" + suffix, "description": "round trip", "duration_s": 2700.0, "sport_id": 1.0,
	})
	fav := asMap(payload(t, res)["favorite"])
	id := int64(asNum(fav["favorite_id"]))
	t.Cleanup(func() { _ = fc.DeleteFavorite(ctx, id) })
	if got := dig(fav, "exercise_targets"); asMap(asArr(got)[0])["duration_s"] != 2700.0 {
		t.Fatalf("created duration_s = %v", got)
	}
	t.Logf("created favorite %d", id)

	// update (full replace, distance goal) → read back
	res = liveCall(t, UpdateFavoriteHandler(fc), map[string]any{
		"favorite_id": float64(id), "name": "live-fav-upd-" + suffix, "distance_m": 6000.0, "sport_id": 2.0,
	})
	fav = asMap(payload(t, res)["favorite"])
	et := asMap(asArr(fav["exercise_targets"])[0])
	if fav["name"] != "live-fav-upd-"+suffix || et["distance_m"] != 6000.0 || et["sport_id"] != 2.0 || et["duration_s"] != nil {
		t.Fatalf("update not applied: %+v", fav)
	}

	// rename → read back
	res = liveCall(t, RenameFavoriteHandler(fc), map[string]any{"favorite_id": float64(id), "name": "live-renamed-" + suffix})
	if got := dig(payload(t, res), "favorite", "name"); got != "live-renamed-"+suffix {
		t.Fatalf("rename read-back = %v", got)
	}

	// set sport → verified read back
	res = liveCall(t, SetFavoriteSportHandler(fc), map[string]any{"favorite_id": float64(id), "sport_id": 3.0})
	if got := dig(payload(t, res), "favorite", "exercise_targets"); asMap(asArr(got)[0])["sport_id"] != 3.0 {
		t.Fatalf("sport read-back = %v", got)
	}

	// schedule on a date → target exists → delete target
	date := time.Now().AddDate(0, 0, 40).Format("2006-01-02")
	res = liveCall(t, ScheduleFavoriteHandler(fc), map[string]any{"favorite_id": float64(id), "date": date, "time": "06:45"})
	sched := asMap(payload(t, res)["target"])
	tid := int64(asNum(sched["target_id"]))
	t.Cleanup(func() { _ = fc.DeleteTrainingTarget(ctx, tid) })
	if sched["date"] != date || sched["time"] != "06:45" || sched["distance_m"] != 6000.0 {
		t.Fatalf("schedule result = %+v", sched)
	}
	tgt, err := fc.GetTrainingTarget(ctx, tid)
	if err != nil || tgt.Datetime != date+"T06:45" || tgt.Name != "live-renamed-"+suffix {
		t.Fatalf("scheduled target read-back: %+v %v", tgt, err)
	}
	t.Logf("scheduled target %d at %s", tid, tgt.Datetime)

	// Same minute again → Polar shifts it; the tool says so.
	res = liveCall(t, ScheduleFavoriteHandler(fc), map[string]any{"favorite_id": float64(id), "date": date, "time": "06:45"})
	tid2 := int64(asNum(asMap(payload(t, res)["target"])["target_id"]))
	t.Cleanup(func() { _ = fc.DeleteTrainingTarget(ctx, tid2) })
	if !strings.Contains(resultText(res), "moved it from 06:45") {
		t.Fatalf("collision not reported: %s", resultText(res))
	}
	for _, x := range []int64{tid, tid2} {
		if err := fc.DeleteTrainingTarget(ctx, x); err != nil {
			t.Fatalf("delete scheduled target %d: %v", x, err)
		}
	}

	// delete favorite → gone
	liveCall(t, DeleteFavoriteHandler(fc), map[string]any{"favorite_id": float64(id)})
	if _, err := fc.GetFavorite(ctx, id); err != flow.ErrFavoriteNotFound {
		t.Fatalf("favorite still readable after delete: %v", err)
	}
	res = callTool(t, GetFavoriteHandler(fc), map[string]any{"favorite_id": float64(id)})
	if !strings.Contains(resultText(res), "No favorite") {
		t.Fatalf("get after delete = %q", resultText(res))
	}
}

func TestLiveSaveTargetAsFavorite(t *testing.T) {
	fc := liveClient(t)
	ctx := context.Background()
	suffix := time.Now().Format("150405")
	date := time.Now().AddDate(0, 0, 41).Format("2006-01-02")

	res := liveCall(t, CreateTrainingTargetHandler(fc), map[string]any{
		"name": "live-src-" + suffix, "date": date, "time": "07:10", "sport_id": 1.0,
		"phases": []any{
			map[string]any{"type": "warmup", "duration_s": 600.0},
			map[string]any{"type": "repeat", "reps": 4.0, "goal": map[string]any{"distance_m": 1000.0},
				"intensity": map[string]any{"hr_zone": 4.0}, "recovery": map[string]any{"duration_s": 120.0}},
			map[string]any{"type": "cooldown", "duration_s": 600.0},
		},
	})
	tid := int64(asNum(payload(t, res)["id"]))
	t.Cleanup(func() { _ = fc.DeleteTrainingTarget(ctx, tid) })

	res = liveCall(t, SaveTargetAsFavoriteHandler(fc), map[string]any{"target_id": float64(tid)})
	fav := asMap(payload(t, res)["favorite"])
	fid := int64(asNum(fav["favorite_id"]))
	t.Cleanup(func() { _ = fc.DeleteFavorite(ctx, fid) })
	phases := asArr(asMap(asArr(fav["exercise_targets"])[0])["phases"])
	if fav["type"] != "PHASED" || fav["name"] != "live-src-"+suffix || len(phases) != 3 {
		t.Fatalf("favorite from target = %+v", fav)
	}
	t.Logf("saved target %d as favorite %d", tid, fid)
	if err := fc.DeleteFavorite(ctx, fid); err != nil {
		t.Fatalf("cleanup favorite: %v", err)
	}
	if err := fc.DeleteTrainingTarget(ctx, tid); err != nil {
		t.Fatalf("cleanup target: %v", err)
	}
}

func TestLiveRouteRoundTrip(t *testing.T) {
	fc := liveClient(t)
	ctx := context.Background()
	suffix := time.Now().Format("150405")
	var b strings.Builder
	b.WriteString(`<?xml version="1.0"?><gpx version="1.1" xmlns="http://www.topografix.com/GPX/1/1"><trk><name>live-route-` + suffix + `</name><trkseg>`)
	for i := 0; i < 50; i++ {
		fmt.Fprintf(&b, `<trkpt lat="%.5f" lon="%.5f"><ele>%d</ele></trkpt>`, 45.0+0.0005*float64(i), 6.0+0.0003*float64(i), 100+i)
	}
	b.WriteString(`</trkseg><trkseg><trkpt lat="45.03" lon="6.02"/></trkseg></trk></gpx>`)

	for _, tc := range []struct {
		name, content string
		points        int
	}{
		{"gpx", b.String(), 51},
		{"tcx", `<?xml version="1.0"?><TrainingCenterDatabase xmlns="http://www.garmin.com/xmlschemas/TrainingCenterDatabase/v2"><Courses><Course><Name>live-tcx-` + suffix + `</Name><Track>` +
			`<Trackpoint><Position><LatitudeDegrees>45.0</LatitudeDegrees><LongitudeDegrees>6.0</LongitudeDegrees></Position><AltitudeMeters>100</AltitudeMeters><DistanceMeters>0</DistanceMeters></Trackpoint>` +
			`<Trackpoint><Position><LatitudeDegrees>45.001</LatitudeDegrees><LongitudeDegrees>6.001</LongitudeDegrees></Position><DistanceMeters>140</DistanceMeters></Trackpoint>` +
			`</Track></Course></Courses></TrainingCenterDatabase>`, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res := liveCall(t, ImportRouteHandler(fc), map[string]any{"content": tc.content, "sport_id": 1.0})
			route := asMap(payload(t, res)["route"])
			fid := asNum(route["favorite_id"])
			if fid == 0 {
				t.Fatalf("new favorite not found: %s", resultText(res))
			}
			t.Cleanup(func() { _ = fc.DeleteFavorite(ctx, int64(fid)) })
			etID := asNum(route["exercise_target_id"])

			res = liveCall(t, GetRouteHandler(fc), map[string]any{"exercise_target_id": etID})
			geo := asMap(payload(t, res)["route"])
			if int(asNum(geo["point_count"])) != tc.points || geo["sport_id"] != 1.0 {
				t.Fatalf("geometry = %+v", geo)
			}
			if d := asNum(geo["distance_m"]); d != asNum(route["distance_m"]) {
				t.Fatalf("distance read-back %v != uploaded %v", d, route["distance_m"])
			}
			res = liveCall(t, GetRouteHandler(fc), map[string]any{"favorite_id": fid, "max_points": 10.0})
			if n := asNum(asMap(payload(t, res)["route"])["returned_points"]); int(n) != min(10, tc.points) {
				t.Fatalf("returned_points = %v", n)
			}
			// Passing the favorite id where the exercise-target id belongs.
			res = callTool(t, GetRouteHandler(fc), map[string]any{"exercise_target_id": fid})
			if !strings.Contains(resultText(res), "not the favorite_id") {
				t.Fatalf("wrong-id answer = %q", resultText(res))
			}
			res = liveCall(t, RenameFavoriteHandler(fc), map[string]any{"favorite_id": fid, "name": "live-route-renamed"})
			if got := dig(payload(t, res), "favorite", "name"); got != "live-route-renamed" {
				t.Fatalf("route rename read-back = %v", got)
			}
			liveCall(t, DeleteFavoriteHandler(fc), map[string]any{"favorite_id": fid})
			res = callTool(t, GetRouteHandler(fc), map[string]any{"exercise_target_id": etID})
			if !strings.Contains(resultText(res), "No route") {
				t.Fatalf("route still readable after delete: %q", resultText(res))
			}
		})
	}
}

//nolint:gocyclo // one sequential live scenario
func TestLiveSessionEditRoundTrip(t *testing.T) {
	fc := liveClient(t)
	ctx := context.Background()
	suffix := time.Now().Format("150405")
	day := time.Now().AddDate(0, 0, -3)
	date := day.Format("2006-01-02")

	liveCall(t, CreateTrainingSessionHandler(fc), map[string]any{
		"name": "live-sess-" + suffix, "date": date, "time": "06:10", "duration_s": 1800.0,
		"distance_m": 5000.0, "hr_avg": 140.0, "sport_id": 1.0, "note": "live",
	})
	ui, err := fc.GetUserInfo(ctx)
	if err != nil {
		t.Fatalf("user: %v", err)
	}
	sessions, err := fc.ListTrainingSessions(ctx, ui.ID, day, day)
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	var sid int64
	for _, s := range sessions {
		if v, _ := s.Note.Get(); v == "live" && strings.HasPrefix(s.StartDate, date+" 06:10") {
			sid = s.ID
		}
	}
	if sid == 0 {
		t.Fatalf("created session not found among %d", len(sessions))
	}
	t.Cleanup(func() { _ = fc.DeleteTrainingSession(ctx, sid) })
	t.Logf("created session %d", sid)

	// note + feeling (partial endpoint) → read back
	res := liveCall(t, EditTrainingSessionHandler(fc), map[string]any{"session_id": float64(sid), "note": "partial edit", "feeling": 5.0})
	sum := asMap(payload(t, res)["summary"])
	if sum["note"] != "partial edit" || sum["feeling"] != 5.0 || sum["name"] != "live-sess-"+suffix {
		t.Fatalf("partial edit read-back = %+v", sum)
	}

	// full form edit → read back (other fields preserved)
	res = liveCall(t, EditTrainingSessionHandler(fc), map[string]any{
		"session_id": float64(sid), "name": "live-sess-edited-" + suffix, "duration_s": 2400.0,
		"distance_m": 7200.0, "hr_max": 171.0, "kcal": 480.0, "speed_kmh": 10.8, "sport_id": 2.0,
	})
	sum = asMap(payload(t, res)["summary"])
	want := map[string]any{
		"name": "live-sess-edited-" + suffix, "session_duration_s": 2400.0, "distance_m": 7200.0,
		"hr_avg": 140.0, "hr_max": 171.0, "calories": 480.0, "note": "partial edit", "feeling": 5.0, "sport_id": 2.0,
	}
	for k, v := range want {
		if sum[k] != v {
			t.Fatalf("full edit read-back %s = %v, want %v (summary %+v)", k, sum[k], v, sum)
		}
	}

	// clear note, change feeling only → name kept (the endpoint would reset
	// a missing name to the sport name)
	res = liveCall(t, EditTrainingSessionHandler(fc), map[string]any{"session_id": float64(sid), "note": "", "feeling": 2.0})
	sum = asMap(payload(t, res)["summary"])
	if _, hasNote := sum["note"]; hasNote || sum["feeling"] != 2.0 || sum["name"] != "live-sess-edited-"+suffix {
		t.Fatalf("clear-note read-back = %+v", sum)
	}

	// delete → gone; a second delete reports "no session" instead of Flow's 200
	liveCall(t, DeleteTrainingSessionHandler(fc), map[string]any{"session_id": float64(sid)})
	if _, err := fc.CheckTrainingSession(ctx, sid); err != flow.ErrSessionNotFound {
		t.Fatalf("session still there: %v", err)
	}
	res = callTool(t, DeleteTrainingSessionHandler(fc), map[string]any{"session_id": float64(sid)})
	if !strings.Contains(resultText(res), "No session") {
		t.Fatalf("second delete = %q", resultText(res))
	}
}

// TestLiveTrainingZones reads zones for every stored profile and computes the
// defaults for a sport the account has no profile for. Read-only: recalculate
// persists nothing.
func TestLiveTrainingZones(t *testing.T) {
	fc := liveClient(t)
	check := func(res *mcpgo.CallToolResult) []any {
		sports := asArr(payload(t, res)["sports"])
		for _, s := range sports {
			hr := asArr(dig(asMap(s), "heart_rate", "zones"))
			if len(hr) != 5 {
				t.Fatalf("heart-rate zones = %v", hr)
			}
			for i := 1; i < 5; i++ {
				if asMap(hr[i])["min_bpm"] != asMap(hr[i-1])["max_bpm"] {
					t.Fatalf("zones not contiguous: %v", hr)
				}
			}
		}
		t.Logf("%s", resultText(res))
		return sports
	}
	check(liveCall(t, GetTrainingZonesHandler(fc), map[string]any{}))
	sports := check(liveCall(t, GetTrainingZonesHandler(fc), map[string]any{"sport_id": 2.0}))
	if len(sports) != 1 || len(asArr(dig(asMap(sports[0]), "power", "zones"))) != 5 {
		t.Fatalf("cycling should have power zones: %v", sports)
	}
}
