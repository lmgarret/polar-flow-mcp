package mcp

import (
	"strings"
	"testing"
)

const (
	testGPX         = `<?xml version="1.0"?><gpx version="1.1" xmlns="http://www.topografix.com/GPX/1/1"><trk><name>Park loop</name><trkseg><trkpt lat="45.0" lon="6.0"></trkpt><trkpt lat="45.001" lon="6.001"></trkpt><trkpt lat="45.002" lon="6.0015"><ele>1200.5</ele></trkpt></trkseg></trk></gpx>`
	favoritesBefore = `{"hasErrors":false,"linkedServices":{},"targets":[{"exerciseTargetId":11,"favoriteId":1,"sportName":"RUNNING","sportId":1,"type":"VOLUME","favoriteName":"Easy","favoriteDescription":"","duration":2700000,"calories":null,"distance":null,"supportedDevices":[],"supportedDeviceIds":[]}]}`
	favoritesAfter  = `{"hasErrors":false,"linkedServices":{},"targets":[{"exerciseTargetId":11,"favoriteId":1,"sportName":"RUNNING","sportId":1,"type":"VOLUME","favoriteName":"Easy","favoriteDescription":"","duration":2700000,"calories":null,"distance":null,"supportedDevices":[],"supportedDeviceIds":[]},` +
		`{"exerciseTargetId":1539651678,"favoriteId":83468958,"sportName":"RUNNING","sportId":1,"type":"ROUTE","favoriteName":"Park loop","favoriteDescription":null,"duration":null,"calories":null,"distance":254.12470690353456,"supportedDevices":[],"supportedDeviceIds":[],"routeSource":"FLOW_FILE_IMPORT"}]}`
)

func TestImportRoute(t *testing.T) {
	ff := newFakeFlow(t)
	ff.on("GET", "/api/favorites", 200, "application/json", favoritesBefore)
	ff.on("GET", "/api/favorites", 200, "application/json", favoritesAfter)
	ff.on("POST", "/api/favorites/trainingTargets/importRoute", 200, "", "")
	res := callTool(t, ImportRouteHandler(ff.client()), map[string]any{"content": testGPX, "sport_id": 1.0})
	if res.IsError {
		t.Fatalf("error: %s", resultText(res))
	}
	w := ff.onlyWrite()
	if w.XRequestedWith != "XMLHttpRequest" {
		t.Fatalf("missing X-Requested-With")
	}
	// Same body the web UI sends for this file (captured), with the sport set.
	assertJSONEqual(t, w.Body, `{"route":[{"latitude":45,"longitude":6,"altitude":0,"distance":0},{"latitude":45.001,"longitude":6.001,"altitude":0,"distance":136.18501998200443},{"latitude":45.002,"longitude":6.0015,"altitude":1200.5,"distance":254.12470690353456}],"sport":1,"name":"Park loop","distance":254.12470690353456}`)
	text := resultText(res)
	if !strings.Contains(text, "favorite_id 83468958, exercise_target_id 1539651678") || !strings.Contains(text, "3 points") {
		t.Fatalf("result = %q", text)
	}
}

func TestImportRoute_Validation(t *testing.T) {
	onePoint := `<gpx><trk><trkseg><trkpt lat="45" lon="6"/></trkseg></trk></gpx>`
	badLat := `<gpx><trk><trkseg><trkpt lat="91" lon="6"/><trkpt lat="45" lon="6"/></trkseg></trk></gpx>`
	longName := strings.Replace(testGPX, "Park loop", strings.Repeat("r", 46), 1)
	runValidationCases(t, ImportRouteHandler, []validationCase{
		{"content missing", map[string]any{}, "content is required"},
		{"content blank", map[string]any{"content": "  \n"}, "content is required"},
		{"content wrong type", map[string]any{"content": 5.0}, "content must be a string"},
		{"not xml", map[string]any{"content": "lat,lon\n45,6"}, "not valid XML"},
		{"kml", map[string]any{"content": "<kml/>"}, "neither <gpx> nor <TrainingCenterDatabase>"},
		{"format mismatch", map[string]any{"content": testGPX, "format": "tcx"}, `format "tcx" was requested`},
		{"format unsupported", map[string]any{"content": testGPX, "format": "fit"}, `format must be "auto"`},
		{"one point", map[string]any{"content": onePoint}, "at least 2 are needed"},
		{"latitude out of range", map[string]any{"content": badLat}, "latitude 91"},
		{"name from file too long", map[string]any{"content": longName}, "pass a shorter name"},
		{"name arg too long", map[string]any{"content": testGPX, "name": strings.Repeat("r", 46)}, "limit is 45"},
		{"name arg blank", map[string]any{"content": testGPX, "name": "   "}, "non-whitespace"},
		{"sport_id zero", map[string]any{"content": testGPX, "sport_id": 0.0}, "sport_id must be between 1"},
	})
	t.Run("unknown sport (Polar would store null)", func(t *testing.T) {
		ff := newFakeFlow(t)
		res := callTool(t, ImportRouteHandler(ff.client()), map[string]any{"content": testGPX, "sport_id": 9999.0})
		if !res.IsError || !strings.Contains(resultText(res), "not a Polar sport") || len(ff.requests()) != 0 {
			t.Fatalf("got %q", resultText(res))
		}
	})
}

const routeET = `{"distance":254.12470690353456,"duration":null,"id":1539651678,"index":null,"kiloCalories":null,"modified":null,"created":null,"type":"ROUTE","routeServiceRoute":null,"segmentId":null,"favoritePhases":[],"sport":{"id":1,"name":"RUNNING"},"gpsRoute":{"id":null,"user":{"id":11111111},"name":"Park loop","parentRouteId":null,"order":null,"distance":254.12470690353456,"startPoint":{"longitude":6.0,"latitude":45.0,"altitude":0.0,"time":0},"waypoints":[{"longitude":6.0,"latitude":45.0,"altitude":0.0,"time":0},{"longitude":6.001,"latitude":45.001,"altitude":0.0,"time":1000},{"longitude":6.0015,"latitude":45.002,"altitude":1200.5,"time":2000}]}}`

func TestGetRoute(t *testing.T) {
	t.Run("by exercise_target_id", func(t *testing.T) {
		ff := newFakeFlow(t)
		ff.on("GET", "/api/favorites/exerciseTarget/1539651678", 200, "application/json", routeET)
		res := callTool(t, GetRouteHandler(ff.client()), map[string]any{"exercise_target_id": 1539651678.0, "max_points": 2.0})
		text := resultText(res)
		if res.IsError || !strings.Contains(text, `"point_count": 3`) || !strings.Contains(text, `"returned_points": 2`) {
			t.Fatalf("got %q", text)
		}
	})
	t.Run("by favorite_id resolves the exercise target", func(t *testing.T) {
		ff := newFakeFlow(t)
		ff.on("GET", "/api/favorites", 200, "application/json", favoritesAfter)
		ff.on("GET", "/api/favorites/exerciseTarget/1539651678", 200, "application/json", routeET)
		res := callTool(t, GetRouteHandler(ff.client()), map[string]any{"favorite_id": 83468958.0})
		if res.IsError || !strings.Contains(resultText(res), `"favorite_id": 83468958`) {
			t.Fatalf("got %q", resultText(res))
		}
	})
	t.Run("favorite that is not a route", func(t *testing.T) {
		ff := newFakeFlow(t)
		ff.on("GET", "/api/favorites", 200, "application/json", favoritesAfter)
		res := callTool(t, GetRouteHandler(ff.client()), map[string]any{"favorite_id": 1.0})
		if !res.IsError || !strings.Contains(resultText(res), "not a route") {
			t.Fatalf("got %q", resultText(res))
		}
	})
	t.Run("favorite_id passed as exercise_target_id (Flow 403)", func(t *testing.T) {
		ff := newFakeFlow(t)
		ff.on("GET", "/api/favorites/exerciseTarget/83468958", 403, "", "")
		res := callTool(t, GetRouteHandler(ff.client()), map[string]any{"exercise_target_id": 83468958.0})
		if res.IsError || !strings.Contains(resultText(res), "not the favorite_id") {
			t.Fatalf("got %q", resultText(res))
		}
	})
	runValidationCases(t, GetRouteHandler, []validationCase{
		{"neither id", map[string]any{}, "pass exercise_target_id or favorite_id"},
		{"both ids", map[string]any{"exercise_target_id": 1.0, "favorite_id": 2.0}, "not both"},
		{"max_points 1", map[string]any{"exercise_target_id": 1.0, "max_points": 1.0}, "max_points must be between 2"},
		{"id string", map[string]any{"exercise_target_id": "1539651678"}, "must be a number"},
	})
}
