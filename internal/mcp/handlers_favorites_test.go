package mcp

import (
	"strings"
	"testing"
)

// Captured GET /api/favoritetarget/{id} bodies.
const (
	favVolumeJSON = `{"name":"Easy 45","exerciseTargets":[{"id":1539651613,"duration":"00:45:00","distance":null,"calories":null,"index":0,"sportId":1,"phases":[]}],"description":"","type":"VOLUME"}`
	favRouteJSON  = `{"name":"probe-route","exerciseTargets":[{"id":1539651678,"duration":null,"distance":null,"calories":null,"index":0,"sportId":null,"phases":[]}],"description":null,"type":"ROUTE"}`
	favMultiJSON  = `{"name":"Tri","exerciseTargets":[{"id":1,"duration":"00:30:00","distance":null,"calories":null,"index":0,"sportId":23,"phases":[]},{"id":2,"duration":"01:00:00","distance":null,"calories":null,"index":1,"sportId":2,"phases":[]}],"description":"","type":"VOLUME"}`
	textPlain     = "text/plain; charset=UTF-8"
)

var long46 = strings.Repeat("n", 46)

func TestCreateFavorite_Validation(t *testing.T) {
	runValidationCases(t, CreateFavoriteHandler, []validationCase{
		{"name missing", map[string]any{"duration_s": 1800.0}, "name is required"},
		{"name empty", map[string]any{"name": "", "duration_s": 1800.0}, "must not be empty"},
		{"name blank", map[string]any{"name": "   ", "duration_s": 1800.0}, "must not be empty"},
		{"name wrong type", map[string]any{"name": 42.0, "duration_s": 1800.0}, "name must be a string, got float64"},
		{"name 46 chars", map[string]any{"name": long46, "duration_s": 1800.0}, "46 characters; Polar's limit is 45"},
		{"description 501", map[string]any{"name": "x", "description": strings.Repeat("d", 501), "duration_s": 1800.0}, "description is 501 characters"},
		{"no goal", map[string]any{"name": "x"}, "needs a goal"},
		{"duration and distance", map[string]any{"name": "x", "duration_s": 60.0, "distance_m": 1000.0}, "only one of duration_s or distance_m"},
		{"phases and duration", map[string]any{"name": "x", "duration_s": 60.0, "phases": []any{map[string]any{"type": "warmup", "duration_s": 60.0}}}, "not both"},
		{"duration 0", map[string]any{"name": "x", "duration_s": 0.0}, "between 1 and 359999"},
		{"duration 100h", map[string]any{"name": "x", "duration_s": 360000.0}, "between 1 and 359999"},
		{"duration fractional", map[string]any{"name": "x", "duration_s": 60.5}, "whole number"},
		{"duration as string", map[string]any{"name": "x", "duration_s": "30:00"}, "duration_s must be a number, got a string"},
		{"distance 0", map[string]any{"name": "x", "distance_m": 0.0}, "greater than 0"},
		{"distance negative", map[string]any{"name": "x", "distance_m": -5.0}, "between 0 and 9.999e+06"},
		{"distance too far", map[string]any{"name": "x", "distance_m": 9999001.0}, "between 0 and 9.999e+06"},
		{"phases not an array", map[string]any{"name": "x", "phases": "warmup"}, "phases must be an array"},
		{"bad phase type", map[string]any{"name": "x", "phases": []any{map[string]any{"type": "sprint"}}}, `unknown type "sprint"`},
		{"repeat reps 1", map[string]any{"name": "x", "phases": []any{map[string]any{"type": "repeat", "reps": 1.0, "goal": map[string]any{"distance_m": 400.0}}}}, "reps is required and must be >= 2"},
		{"sport_id zero", map[string]any{"name": "x", "duration_s": 60.0, "sport_id": 0.0}, "sport_id must be between 1"},
		{"sport_id string", map[string]any{"name": "x", "duration_s": 60.0, "sport_id": "running"}, "sport_id must be a number"},
	})
}

func TestCreateFavorite_UnknownSportChecksCatalogOnly(t *testing.T) {
	ff := newFakeFlow(t)
	res := callTool(t, CreateFavoriteHandler(ff.client()), map[string]any{"name": "x", "duration_s": 60.0, "sport_id": 9999.0})
	if !res.IsError || !strings.Contains(resultText(res), "sport_id 9999 is not a Polar sport") {
		t.Fatalf("got %q", resultText(res))
	}
	if w := ff.writes(); len(w) != 0 {
		t.Fatalf("unknown sport must not be written (Polar would store it verbatim): %+v", w)
	}
}

func TestCreateFavorite_WireBody(t *testing.T) {
	tests := []struct {
		name string
		args map[string]any
		want string
	}{
		{
			name: "volume duration",
			args: map[string]any{"name": "Easy 45", "duration_s": 2700.0},
			want: `{"type":"VOLUME","name":"Easy 45","description":"","exerciseTargets":[{"sportId":1,"duration":"00:45:00","phases":[],"id":null}]}`,
		},
		{
			name: "volume distance, cycling, description",
			args: map[string]any{"name": "Ride", "distance_m": 40000.0, "sport_id": 2.0, "description": "flat"},
			want: `{"type":"VOLUME","name":"Ride","description":"flat","exerciseTargets":[{"sportId":2,"distance":40000,"phases":[],"id":null}]}`,
		},
		{
			name: "phased",
			args: map[string]any{"name": "6x800", "phases": []any{
				map[string]any{"type": "warmup", "duration_s": 900.0},
				map[string]any{"type": "repeat", "reps": 6.0, "goal": map[string]any{"distance_m": 800.0},
					"intensity": map[string]any{"hr_zone": 4.0}, "recovery": map[string]any{"duration_s": 90.0}},
			}},
			want: `{"type":"PHASED","name":"6x800","description":"","exerciseTargets":[{"sportId":1,"phases":[` +
				`{"phaseType":"PHASE","name":"Warm-up","phaseChangeType":"AUTOMATIC","goalType":"DURATION","duration":"00:15:00","intensityType":"NONE"},` +
				`{"phaseType":"REPEAT","repeatCount":6,"phases":[` +
				`{"phaseType":"PHASE","name":"Work","phaseChangeType":"AUTOMATIC","goalType":"DISTANCE","distance":800,"intensityType":"HEART_RATE_ZONES","lowerZone":4,"upperZone":4},` +
				`{"phaseType":"PHASE","name":"Recovery","phaseChangeType":"AUTOMATIC","goalType":"DURATION","duration":"00:01:30","intensityType":"NONE"}]}],"id":null}]}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ff := newFakeFlow(t)
			ff.on("POST", "/api/favoritetarget", 201, textPlain, "83468869")
			ff.on("GET", "/api/favoritetarget/83468869", 200, "application/json", favVolumeJSON)
			res := callTool(t, CreateFavoriteHandler(ff.client()), tt.args)
			if res.IsError {
				t.Fatalf("error: %s", resultText(res))
			}
			w := ff.onlyWrite()
			if w.XRequestedWith != "XMLHttpRequest" {
				t.Fatalf("missing X-Requested-With")
			}
			assertJSONEqual(t, w.Body, tt.want)
			if !strings.Contains(resultText(res), "Created favorite 83468869") {
				t.Fatalf("result = %q", resultText(res))
			}
		})
	}
}

func TestCreateFavorite_ServerValidationSurfaced(t *testing.T) {
	ff := newFakeFlow(t)
	ff.on("POST", "/api/favoritetarget", 400, "application/json", `{"target.name":["Un problème inattendu est survenu."]}`)
	res := callTool(t, CreateFavoriteHandler(ff.client()), map[string]any{"name": "x", "duration_s": 60.0})
	if !res.IsError || !strings.Contains(resultText(res), "target.name") {
		t.Fatalf("got %q", resultText(res))
	}
}

func TestUpdateFavorite(t *testing.T) {
	t.Run("carries the exercise-target id", func(t *testing.T) {
		ff := newFakeFlow(t)
		ff.on("GET", "/api/favoritetarget/83468889", 200, "application/json", favVolumeJSON)
		ff.on("POST", "/api/favoritetarget/83468889", 200, "", "")
		res := callTool(t, UpdateFavoriteHandler(ff.client()), map[string]any{
			"favorite_id": 83468889.0, "name": "Easy 50", "duration_s": 3000.0,
		})
		if res.IsError {
			t.Fatalf("error: %s", resultText(res))
		}
		assertJSONEqual(t, ff.onlyWrite().Body,
			`{"type":"VOLUME","name":"Easy 50","description":"","exerciseTargets":[{"id":1539651613,"sportId":1,"duration":"00:50:00","phases":[]}]}`)
	})
	for _, tc := range []struct {
		name, fav, wantErr string
	}{
		{"route refused", favRouteJSON, "is a ROUTE"},
		{"multi-sport refused", favMultiJSON, "2 exercise targets"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ff := newFakeFlow(t)
			ff.on("GET", "/api/favoritetarget/7", 200, "application/json", tc.fav)
			res := callTool(t, UpdateFavoriteHandler(ff.client()), map[string]any{"favorite_id": 7.0, "name": "x", "duration_s": 60.0})
			if !res.IsError || !strings.Contains(resultText(res), tc.wantErr) {
				t.Fatalf("got %q", resultText(res))
			}
			if w := ff.writes(); len(w) != 0 {
				t.Fatalf("no write expected: %+v", w)
			}
		})
	}
	// A rename-style call (no goal) on a route: the route is why it cannot be
	// done, not the missing goal.
	for _, tc := range []struct {
		name, fav, wantErr string
	}{
		{"route without goal", favRouteJSON, "update_favorite cannot edit routes"},
		{"multi-sport without goal", favMultiJSON, "2 exercise targets"},
		{"volume without goal", favVolumeJSON, "a favorite needs a goal"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ff := newFakeFlow(t)
			ff.on("GET", "/api/favoritetarget/7", 200, "application/json", tc.fav)
			res := callTool(t, UpdateFavoriteHandler(ff.client()), map[string]any{"favorite_id": 7.0, "name": "x"})
			if !res.IsError || !strings.Contains(resultText(res), tc.wantErr) || len(ff.writes()) != 0 {
				t.Fatalf("got %q, writes %v", resultText(res), ff.writes())
			}
		})
	}
	t.Run("unknown id", func(t *testing.T) {
		ff := newFakeFlow(t)
		ff.on("GET", "/api/favoritetarget/1", 404, textPlain, "favorites.notification_oops")
		res := callTool(t, UpdateFavoriteHandler(ff.client()), map[string]any{"favorite_id": 1.0, "name": "x", "duration_s": 60.0})
		if res.IsError || !strings.Contains(resultText(res), "No favorite with favorite_id 1") {
			t.Fatalf("got %q (error=%v)", resultText(res), res.IsError)
		}
	})
	runValidationCases(t, UpdateFavoriteHandler, []validationCase{
		{"id missing", map[string]any{"name": "x", "duration_s": 60.0}, "favorite_id is required"},
		{"id negative", map[string]any{"favorite_id": -3.0, "name": "x", "duration_s": 60.0}, "favorite_id must be a positive integer id"},
		{"id string", map[string]any{"favorite_id": "83468889", "name": "x", "duration_s": 60.0}, "favorite_id must be a positive integer id"},
		{"id fractional", map[string]any{"favorite_id": 1.5, "name": "x", "duration_s": 60.0}, "whole number"},
		{"name too long", map[string]any{"favorite_id": 1.0, "name": long46, "duration_s": 60.0}, "limit is 45"},
	})
}

func TestGetAndDeleteFavorite(t *testing.T) {
	t.Run("get foreign", func(t *testing.T) {
		ff := newFakeFlow(t)
		ff.on("GET", "/api/favoritetarget/83468000", 403, textPlain, "favorites.notification_oops")
		res := callTool(t, GetFavoriteHandler(ff.client()), map[string]any{"favorite_id": 83468000.0})
		if !res.IsError || !strings.Contains(resultText(res), "another Polar account") {
			t.Fatalf("got %q", resultText(res))
		}
	})
	t.Run("get normalises duration", func(t *testing.T) {
		ff := newFakeFlow(t)
		ff.on("GET", "/api/favoritetarget/5", 200, "application/json", favVolumeJSON)
		res := callTool(t, GetFavoriteHandler(ff.client()), map[string]any{"favorite_id": 5.0})
		if res.IsError || !strings.Contains(resultText(res), `"duration_s": 2700`) {
			t.Fatalf("got %q", resultText(res))
		}
	})
	t.Run("delete", func(t *testing.T) {
		ff := newFakeFlow(t)
		ff.on("GET", "/api/favoritetarget/5", 200, "application/json", favRouteJSON)
		ff.on("DELETE", "/api/favorites/delete/5", 200, textPlain, `{"success":"Favori supprimé"}`)
		res := callTool(t, DeleteFavoriteHandler(ff.client()), map[string]any{"favorite_id": 5.0})
		if res.IsError || !strings.Contains(resultText(res), `Deleted favorite 5 ("probe-route")`) {
			t.Fatalf("got %q", resultText(res))
		}
		if w := ff.onlyWrite(); w.XRequestedWith != "XMLHttpRequest" {
			t.Fatalf("missing X-Requested-With")
		}
	})
	t.Run("delete unknown is not attempted", func(t *testing.T) {
		ff := newFakeFlow(t)
		ff.on("GET", "/api/favoritetarget/1", 404, textPlain, "favorites.notification_oops")
		res := callTool(t, DeleteFavoriteHandler(ff.client()), map[string]any{"favorite_id": 1.0})
		if res.IsError || !strings.Contains(resultText(res), "No favorite") || len(ff.writes()) != 0 {
			t.Fatalf("got %q writes=%v", resultText(res), ff.writes())
		}
	})
	runValidationCases(t, DeleteFavoriteHandler, []validationCase{
		{"id missing", map[string]any{}, "favorite_id is required"},
		{"id zero", map[string]any{"favorite_id": 0.0}, "positive integer id"},
		{"id bool", map[string]any{"favorite_id": true}, "got a boolean"},
	})
}

func TestRenameFavorite(t *testing.T) {
	ff := newFakeFlow(t)
	ff.on("GET", "/api/favoritetarget/9", 200, "application/json", favVolumeJSON)
	ff.on("PUT", "/api/favorites/saveName", 200, textPlain, `{"success":"Modifications enregistrées"}`)
	res := callTool(t, RenameFavoriteHandler(ff.client()), map[string]any{"favorite_id": 9.0, "name": "Long run — Sundays"})
	if res.IsError {
		t.Fatalf("error: %s", resultText(res))
	}
	assertJSONEqual(t, ff.onlyWrite().Body, `{"favoriteId":9,"favoriteName":"Long run — Sundays"}`)

	runValidationCases(t, RenameFavoriteHandler, []validationCase{
		{"name missing", map[string]any{"favorite_id": 9.0}, "name is required"},
		// Polar accepts a whitespace-only name; the tool does not.
		{"name blank", map[string]any{"favorite_id": 9.0, "name": "  "}, "must not be empty"},
		{"name 46", map[string]any{"favorite_id": 9.0, "name": long46}, "limit is 45"},
		{"id missing", map[string]any{"name": "x"}, "favorite_id is required"},
	})

	t.Run("unknown id checked before saveName (which would 500)", func(t *testing.T) {
		ff := newFakeFlow(t)
		ff.on("GET", "/api/favoritetarget/1", 404, textPlain, "favorites.notification_oops")
		res := callTool(t, RenameFavoriteHandler(ff.client()), map[string]any{"favorite_id": 1.0, "name": "x"})
		if res.IsError || !strings.Contains(resultText(res), "No favorite") || len(ff.writes()) != 0 {
			t.Fatalf("got %q writes=%v", resultText(res), ff.writes())
		}
	})
}

func TestSetFavoriteSport(t *testing.T) {
	after := strings.Replace(favVolumeJSON, `"sportId":1`, `"sportId":2`, 1)
	t.Run("single exercise target resolved and verified", func(t *testing.T) {
		ff := newFakeFlow(t)
		ff.on("GET", "/api/favoritetarget/9", 200, "application/json", favVolumeJSON)
		ff.on("GET", "/api/favoritetarget/9", 200, "application/json", after)
		ff.on("PUT", "/api/favorites/saveSport", 200, textPlain, `{"success":"ok"}`)
		res := callTool(t, SetFavoriteSportHandler(ff.client()), map[string]any{"favorite_id": 9.0, "sport_id": 2.0})
		if res.IsError {
			t.Fatalf("error: %s", resultText(res))
		}
		assertJSONEqual(t, ff.onlyWrite().Body, `{"favoriteId":9,"favoriteSportId":2,"exerciseTargetId":1539651613}`)
	})
	t.Run("silent no-op detected on read-back", func(t *testing.T) {
		ff := newFakeFlow(t)
		ff.on("GET", "/api/favoritetarget/9", 200, "application/json", favVolumeJSON)
		ff.on("PUT", "/api/favorites/saveSport", 200, textPlain, `{"success":"ok"}`)
		res := callTool(t, SetFavoriteSportHandler(ff.client()), map[string]any{"favorite_id": 9.0, "sport_id": 2.0})
		if !res.IsError || !strings.Contains(resultText(res), "still reads back") {
			t.Fatalf("got %q", resultText(res))
		}
	})
	for _, tc := range []struct {
		name    string
		fav     string
		args    map[string]any
		wantErr string
	}{
		{"multi-sport needs exercise_target_id", favMultiJSON, map[string]any{"favorite_id": 9.0, "sport_id": 2.0}, "pass exercise_target_id"},
		{"foreign exercise_target_id", favVolumeJSON, map[string]any{"favorite_id": 9.0, "sport_id": 2.0, "exercise_target_id": 1.0}, "is not part of this favorite"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ff := newFakeFlow(t)
			ff.on("GET", "/api/favoritetarget/9", 200, "application/json", tc.fav)
			res := callTool(t, SetFavoriteSportHandler(ff.client()), tc.args)
			if !res.IsError || !strings.Contains(resultText(res), tc.wantErr) || len(ff.writes()) != 0 {
				t.Fatalf("got %q writes=%v", resultText(res), ff.writes())
			}
		})
	}
	t.Run("unknown sport rejected (Polar would accept it)", func(t *testing.T) {
		ff := newFakeFlow(t)
		res := callTool(t, SetFavoriteSportHandler(ff.client()), map[string]any{"favorite_id": 9.0, "sport_id": 9999.0})
		if !res.IsError || !strings.Contains(resultText(res), "not a Polar sport") || len(ff.requests()) != 0 {
			t.Fatalf("got %q", resultText(res))
		}
	})
	runValidationCases(t, SetFavoriteSportHandler, []validationCase{
		// Polar clears the sport when favoriteSportId is missing.
		{"sport_id missing", map[string]any{"favorite_id": 9.0}, "sport_id is required"},
		{"sport_id zero", map[string]any{"favorite_id": 9.0, "sport_id": 0.0}, "sport_id must be a positive integer id"},
	})
}

const targetJSON = `{"datetime":"2026-10-15T09:00","sourceProgramType":"UNKNOWN","isUserEditable":true,"name":"probe-fav-src","exerciseTargets":[{"id":1539651413,"duration":"00:10:00","distance":2000.0,"calories":null,"index":0,"sportId":1,"phases":[{"id":2929451679,"lowerZone":1,"upperZone":2,"intensityType":"HEART_RATE_ZONES","phaseChangeType":"AUTOMATIC","goalType":"DURATION","duration":"00:10:00","name":"Warm-up","phaseType":"PHASE"}]}],"description":"src desc","type":"PHASED"}`

func TestSaveTargetAsFavorite(t *testing.T) {
	t.Run("copies the target like the web UI", func(t *testing.T) {
		ff := newFakeFlow(t)
		ff.on("GET", "/api/trainingtarget/1461116593", 200, "application/json", targetJSON)
		ff.on("POST", "/api/favoritetarget", 201, textPlain, "83468869")
		ff.on("GET", "/api/favoritetarget/83468869", 200, "application/json", favVolumeJSON)
		res := callTool(t, SaveTargetAsFavoriteHandler(ff.client()), map[string]any{"target_id": 1461116593.0})
		if res.IsError {
			t.Fatalf("error: %s", resultText(res))
		}
		assertJSONEqual(t, ff.onlyWrite().Body, `{"type":"PHASED","name":"probe-fav-src","description":"src desc","exerciseTargets":[{"sportId":1,"duration":null,"distance":null,"calories":null,"phases":[{"lowerZone":1,"upperZone":2,"intensityType":"HEART_RATE_ZONES","phaseChangeType":"AUTOMATIC","goalType":"DURATION","duration":"00:10:00","name":"Warm-up","phaseType":"PHASE"}],"id":null}]}`)
	})
	t.Run("long target name needs an override", func(t *testing.T) {
		ff := newFakeFlow(t)
		ff.on("GET", "/api/trainingtarget/1", 200, "application/json", strings.Replace(targetJSON, `"name":"probe-fav-src"`, `"name":"`+long46+`"`, 1))
		res := callTool(t, SaveTargetAsFavoriteHandler(ff.client()), map[string]any{"target_id": 1.0})
		if !res.IsError || !strings.Contains(resultText(res), "pass a shorter name") || len(ff.writes()) != 0 {
			t.Fatalf("got %q", resultText(res))
		}
	})
	t.Run("route-type target refused", func(t *testing.T) {
		ff := newFakeFlow(t)
		ff.on("GET", "/api/trainingtarget/3", 200, "application/json",
			`{"datetime":"2026-11-20T07:00","name":"r","exerciseTargets":[{"id":1,"duration":null,"distance":null,"calories":null,"index":0,"sportId":1,"phases":[]}],"description":null,"type":"ROUTE"}`)
		res := callTool(t, SaveTargetAsFavoriteHandler(ff.client()), map[string]any{"target_id": 3.0})
		if !res.IsError || !strings.Contains(resultText(res), "ROUTE-type target") || len(ff.writes()) != 0 {
			t.Fatalf("got %q", resultText(res))
		}
	})
	t.Run("unknown target", func(t *testing.T) {
		ff := newFakeFlow(t)
		ff.on("GET", "/api/trainingtarget/2", 404, textPlain, "trainingTarget.targetNotFound")
		res := callTool(t, SaveTargetAsFavoriteHandler(ff.client()), map[string]any{"target_id": 2.0})
		if res.IsError || !strings.Contains(resultText(res), "No target with id 2") {
			t.Fatalf("got %q", resultText(res))
		}
	})
	runValidationCases(t, SaveTargetAsFavoriteHandler, []validationCase{
		{"target_id missing", map[string]any{}, "target_id is required"},
		{"name override too long", map[string]any{"target_id": 1.0, "name": long46}, "limit is 45"},
		{"name override blank", map[string]any{"target_id": 1.0, "name": " "}, "must not be empty"},
	})
}

func TestScheduleFavorite(t *testing.T) {
	const resp = `{"id":1461116733,"name":"probe-fav-src","sport":{"id":1,"name":"RUNNING"},"iconUrl":null,"description":"src desc","date":"20-10-2026","time":"07:30","distance":"2000.0","duration":"600000","kiloCalories":null,"url":"/target/1461116733"}`
	t.Run("wire body and text/plain JSON response", func(t *testing.T) {
		ff := newFakeFlow(t)
		ff.on("GET", "/api/favoritetarget/83468869", 200, "application/json", favVolumeJSON)
		ff.on("POST", "/training/target/createTargetFromFavourite", 200, textPlain, resp)
		res := callTool(t, ScheduleFavoriteHandler(ff.client()), map[string]any{"favorite_id": 83468869.0, "date": "2026-10-20", "time": "07:30"})
		if res.IsError {
			t.Fatalf("error: %s", resultText(res))
		}
		w := ff.onlyWrite()
		assertJSONEqual(t, w.Body, `{"id":"83468869","to":"2026-10-20T07:30:00.000+00:00"}`)
		if w.XRequestedWith != "XMLHttpRequest" {
			t.Fatalf("missing X-Requested-With")
		}
		text := resultText(res)
		if !strings.Contains(text, "training target 1461116733 on 2026-10-20 at 07:30") || strings.Contains(text, "moved") {
			t.Fatalf("result = %q", text)
		}
	})
	t.Run("collision shift reported", func(t *testing.T) {
		ff := newFakeFlow(t)
		ff.on("GET", "/api/favoritetarget/83468869", 200, "application/json", favVolumeJSON)
		ff.on("POST", "/training/target/createTargetFromFavourite", 200, textPlain, strings.Replace(resp, `"07:30"`, `"07:31"`, 1))
		res := callTool(t, ScheduleFavoriteHandler(ff.client()), map[string]any{"favorite_id": 83468869.0, "date": "2026-10-20", "time": "07:30"})
		if !strings.Contains(resultText(res), "moved it from 07:30") {
			t.Fatalf("result = %q", resultText(res))
		}
	})
	t.Run("default time is 18:00, not midnight", func(t *testing.T) {
		ff := newFakeFlow(t)
		ff.on("GET", "/api/favoritetarget/83468869", 200, "application/json", favVolumeJSON)
		ff.on("POST", "/training/target/createTargetFromFavourite", 200, textPlain, strings.Replace(resp, `"07:30"`, `"18:00"`, 1))
		callTool(t, ScheduleFavoriteHandler(ff.client()), map[string]any{"favorite_id": 83468869.0, "date": "2026-10-20"})
		assertJSONEqual(t, ff.onlyWrite().Body, `{"id":"83468869","to":"2026-10-20T18:00:00.000+00:00"}`)
	})
	t.Run("route refused", func(t *testing.T) {
		ff := newFakeFlow(t)
		ff.on("GET", "/api/favoritetarget/5", 200, "application/json", favRouteJSON)
		res := callTool(t, ScheduleFavoriteHandler(ff.client()), map[string]any{"favorite_id": 5.0, "date": "2026-10-20"})
		if !res.IsError || !strings.Contains(resultText(res), "is a ROUTE") || len(ff.writes()) != 0 {
			t.Fatalf("got %q", resultText(res))
		}
	})
	t.Run("foreign favorite", func(t *testing.T) {
		ff := newFakeFlow(t)
		ff.on("GET", "/api/favoritetarget/83468000", 403, textPlain, "favorites.notification_oops")
		res := callTool(t, ScheduleFavoriteHandler(ff.client()), map[string]any{"favorite_id": 83468000.0, "date": "2026-10-20"})
		if !res.IsError || !strings.Contains(resultText(res), "another Polar account") {
			t.Fatalf("got %q", resultText(res))
		}
	})
	runValidationCases(t, ScheduleFavoriteHandler, []validationCase{
		{"date missing", map[string]any{"favorite_id": 1.0}, "date is required"},
		{"date wrong format", map[string]any{"favorite_id": 1.0, "date": "20.10.2026"}, "YYYY-MM-DD"},
		{"date impossible", map[string]any{"favorite_id": 1.0, "date": "2026-02-30"}, "YYYY-MM-DD"},
		{"time wrong format", map[string]any{"favorite_id": 1.0, "date": "2026-10-20", "time": "7h30"}, "HH:MM"},
		{"time 24:00", map[string]any{"favorite_id": 1.0, "date": "2026-10-20", "time": "24:00"}, "HH:MM"},
		{"midnight", map[string]any{"favorite_id": 1.0, "date": "2026-10-20", "time": "00:00"}, "00:00 cannot be scheduled"},
		{"date number", map[string]any{"favorite_id": 1.0, "date": 20261020.0}, "date must be a string"},
		{"favorite_id missing", map[string]any{"date": "2026-10-20"}, "favorite_id is required"},
	})
}

func TestListFavorites(t *testing.T) {
	const listing = `{"hasErrors":false,"linkedServices":{},"targets":[
		{"exerciseTargetId":11,"favoriteId":1,"sportName":"RUNNING","sportId":1,"type":"VOLUME","favoriteName":"Easy","favoriteDescription":"","duration":2700000,"calories":null,"distance":null,"supportedDevices":[],"supportedDeviceIds":[]},
		{"exerciseTargetId":22,"favoriteId":2,"sportName":null,"sportId":null,"type":"ROUTE","favoriteName":"Loop","favoriteDescription":null,"duration":null,"calories":null,"distance":5000.0,"supportedDevices":[],"supportedDeviceIds":[],"routeSource":"FLOW_FILE_IMPORT"}]}`
	for _, tc := range []struct {
		kind       string
		want, miss string
	}{
		{"all", `"Loop"`, ""},
		{"templates", `"Easy"`, `"Loop"`},
		{"routes", `"Loop"`, `"Easy"`},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			ff := newFakeFlow(t)
			ff.on("GET", "/api/favorites", 200, "application/json", listing)
			text := resultText(callTool(t, ListFavoritesHandler(ff.client()), map[string]any{"kind": tc.kind}))
			if !strings.Contains(text, tc.want) || (tc.miss != "" && strings.Contains(text, tc.miss)) {
				t.Fatalf("kind %s: %q", tc.kind, text)
			}
		})
	}
	runValidationCases(t, ListFavoritesHandler, []validationCase{
		{"bad kind", map[string]any{"kind": "segments"}, `kind must be "all", "templates" or "routes"`},
	})
}
