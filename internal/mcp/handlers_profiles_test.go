package mcp

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/lmgarret/polar-flow-mcp/internal/flow"
)

// Captured live 2026-09-30: POST /api/sports/profiles/create/2 on an account
// without a cycling profile (201), and the same profile after update-zones set
// free HR and power zones.
const (
	cyclingProfileUUID = "0f000000-0080-0000-0000-000000000002"
	cyclingCreated     = `{"uuid":"0f000000-0080-0000-0000-000000000002","userId":"11111111","created":"2026-09-30T09:35:36Z","modified":"2026-09-30T09:35:36Z","profile":{"sportId":2,"settings":{"sensorBroadcastingHr":false,"hrZoneLockAvailable":true,"speedZoneLockAvailable":false,"powerZoneLockAvailable":false,"volume":{"volume":60},"speedView":"SPEED_VIEW_SPEED","zoneLimits":{"heartRateZones":[{"lowerLimit":95,"higherLimit":114},{"lowerLimit":114,"higherLimit":133},{"lowerLimit":133,"higherLimit":152},{"lowerLimit":152,"higherLimit":171},{"lowerLimit":171,"higherLimit":190}],"speedZones":[{"lowerLimit":10.0,"higherLimit":20.0},{"lowerLimit":20.0,"higherLimit":30.0},{"lowerLimit":30.0,"higherLimit":40.0},{"lowerLimit":40.0,"higherLimit":50.0},{"lowerLimit":50.0,"higherLimit":399.0}],"powerZones":[{"lowerLimit":138,"higherLimit":188},{"lowerLimit":188,"higherLimit":225},{"lowerLimit":225,"higherLimit":263},{"lowerLimit":263,"higherLimit":300},{"lowerLimit":300,"higherLimit":2000}],"heartRateSettingSource":"HEART_RATE_ZONE_SETTING_SOURCE_DEFAULT","powerSettingSource":"POWER_ZONE_SETTING_SOURCE_DEFAULT","speedSettingSource":"SPEED_ZONE_SETTING_SOURCE_DEFAULT","powerZoneCalculationMethod":"POWER_ZONE_CALCULATION_METHOD_FTP_BASED","speedZoneCalculationMethod":"SPEED_ZONE_CALCULATION_METHOD_SPORT_SPECIFIC_PREDEFINED"},"trainingReminder":{"type":"TRAINING_REMINDER_TYPE_OFF","text":""},"powerView":"POWER_VIEW_WATT","strideSpeedSource":"STRIDE_SPEED_SOURCE_STRIDE","swimmingUnits":"SWIMMING_UNITS_METERS","remoteButtonActions":[]},"sportFactor":0.9,"productSettings":[],"subProfileUuids":[],"functionalThresholdPower":{"power":250,"source":"FTP_SOURCE_ESTIMATED"}},"legacyProfiles":{},"legacyId":"453171796"}`
	cyclingFreeZones   = `{"uuid":"0f000000-0080-0000-0000-000000000002","userId":"0","created":"2026-09-30T09:35:36Z","modified":"2026-09-30T09:36:27Z","profile":{"sportId":2,"settings":{"speedView":"SPEED_VIEW_SPEED","zoneLimits":{"heartRateZones":[{"lowerLimit":100,"higherLimit":120},{"lowerLimit":120,"higherLimit":140},{"lowerLimit":140,"higherLimit":155},{"lowerLimit":155,"higherLimit":170},{"lowerLimit":170,"higherLimit":185}],"speedZones":[{"lowerLimit":10.0,"higherLimit":20.0},{"lowerLimit":20.0,"higherLimit":30.0},{"lowerLimit":30.0,"higherLimit":40.0},{"lowerLimit":40.0,"higherLimit":50.0},{"lowerLimit":50.0,"higherLimit":399.0}],"powerZones":[{"lowerLimit":100,"higherLimit":150},{"lowerLimit":150,"higherLimit":200},{"lowerLimit":200,"higherLimit":250},{"lowerLimit":250,"higherLimit":300},{"lowerLimit":300,"higherLimit":2000}],"heartRateSettingSource":"HEART_RATE_ZONE_SETTING_SOURCE_FREE","powerSettingSource":"POWER_ZONE_SETTING_SOURCE_FREE","speedSettingSource":"SPEED_ZONE_SETTING_SOURCE_DEFAULT","powerZoneCalculationMethod":"POWER_ZONE_CALCULATION_METHOD_UNKNOWN","speedZoneCalculationMethod":"SPEED_ZONE_CALCULATION_METHOD_SPORT_SPECIFIC_PREDEFINED"}},"productSettings":[],"subProfileUuids":[]},"legacyProfiles":{}}`
	swimmingProfile    = `{"uuid":"0f000000-0080-0000-0000-000000000017","userId":"0","created":"2026-09-30T09:00:00Z","modified":"2026-09-30T09:00:00Z","profile":{"sportId":23,"settings":{"zoneLimits":{"heartRateZones":[{"lowerLimit":95,"higherLimit":114},{"lowerLimit":114,"higherLimit":133},{"lowerLimit":133,"higherLimit":152},{"lowerLimit":152,"higherLimit":171},{"lowerLimit":171,"higherLimit":190}],"speedZones":[],"powerZones":[],"heartRateSettingSource":"HEART_RATE_ZONE_SETTING_SOURCE_DEFAULT"}},"productSettings":[],"subProfileUuids":[]},"legacyProfiles":{}}`
	lastProfileError   = "Invalid delete sport profile request, status INVALID_REQUEST, message 'Deleting the last active profile is not allowed. UUID: 0f000000-0080-0000-0000-000000000001'"
)

func TestCreateSportProfile(t *testing.T) {
	for _, tc := range []struct {
		name, want string
		status     int
	}{
		{"new", "Created the CYCLING (sport_id 2) sport profile", 201},
		{"already exists", "already exists — nothing was changed", 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ff := newFakeFlow(t)
			ff.on("POST", "/api/sports/profiles/create/2", tc.status, "application/json", cyclingCreated)
			res := callTool(t, CreateSportProfileHandler(ff.client()), map[string]any{"sport_id": 2.0})
			if res.IsError {
				t.Fatalf("error: %s", resultText(res))
			}
			if !strings.Contains(resultText(res), tc.want) {
				t.Fatalf("text = %q, want %q", resultText(res), tc.want)
			}
			if w := ff.onlyWrite(); w.Body != "" || w.XRequestedWith != "XMLHttpRequest" {
				t.Fatalf("create request = %+v", w)
			}
			sports := zonesPayload(t, res)
			if len(sports) != 1 || sports[0]["source"] != "profile" || sports[0]["sport_id"] != 2.0 {
				t.Fatalf("payload = %v", sports)
			}
			assertJSONEqual(t, zoneAt(t, sports[0], "power", 5), `{"zone":5,"min_w":300,"max_w":null}`)
		})
	}
}

func TestCreateSportProfile_Validation(t *testing.T) {
	runValidationCases(t, CreateSportProfileHandler, []validationCase{
		{"missing sport", map[string]any{}, "sport_id is required"},
		{"unknown sport", map[string]any{"sport_id": 999.0}, "not a Polar sport"},
		{"string id", map[string]any{"sport_id": "2"}, "must be a number"},
	})
}

// updateBody decodes the update-zones request body.
func updateBody(t *testing.T, body string) (zl map[string]any, env map[string]any) {
	t.Helper()
	if err := json.Unmarshal([]byte(body), &env); err != nil {
		t.Fatalf("body is not JSON: %v\n%s", err, body)
	}
	return obj(obj(obj(env["profile"])["settings"])["zoneLimits"]), env
}

//nolint:gocyclo // one sequential scenario
func TestUpdateTrainingZones_SetAndReset(t *testing.T) {
	ff := newFakeFlow(t)
	ff.on("GET", "/api/sports/profiles/"+cyclingProfileUUID, 200, "application/json", cyclingCreated)
	ff.on("GET", "/api/sports/profiles/"+cyclingProfileUUID, 200, "application/json", cyclingFreeZones)
	ff.on("GET", "/api/account/users/current/user", 200, "application/json", currentUser)
	ff.on("POST", "/api/sports/profiles/"+cyclingProfileUUID+"/update-zones", 200, "", "")
	res := callTool(t, UpdateTrainingZonesHandler(ff.client()), map[string]any{
		"sport_id":       2.0,
		"heart_rate_bpm": []any{100.0, 120.0, 140.0, 155.0, 170.0, 185.0},
		"power_w":        []any{100.0, 150.0, 200.0, 250.0, 300.0},
		"reset":          []any{"speed"},
	})
	if res.IsError {
		t.Fatalf("error: %s", resultText(res))
	}
	w := ff.onlyWrite()
	if w.XRequestedWith != "XMLHttpRequest" {
		t.Fatalf("missing X-Requested-With")
	}
	zl, env := updateBody(t, w.Body)
	if env["uuid"] != cyclingProfileUUID || env["userId"] != 11111111.0 || env["modified"] == nil {
		t.Fatalf("envelope = %v", env)
	}
	// Flow requires all three sources; edited lists become FREE, the reset one DEFAULT.
	if zl["heartRateSettingSource"] != "HEART_RATE_ZONE_SETTING_SOURCE_FREE" ||
		zl["powerSettingSource"] != "POWER_ZONE_SETTING_SOURCE_FREE" ||
		zl["speedSettingSource"] != "SPEED_ZONE_SETTING_SOURCE_DEFAULT" {
		t.Fatalf("sources = %v", zl)
	}
	hr, _ := json.Marshal(zl["heartRateZones"])
	assertJSONEqual(t, string(hr), `[{"lowerLimit":100,"higherLimit":120},{"lowerLimit":120,"higherLimit":140},{"lowerLimit":140,"higherLimit":155},{"lowerLimit":155,"higherLimit":170},{"lowerLimit":170,"higherLimit":185}]`)
	// HR and power limits must go out as integers — Flow 400s "Not an int32 value" otherwise.
	if strings.Contains(w.Body, "100.0") || strings.Contains(w.Body, "2000.0") {
		t.Fatalf("integer limits were encoded as decimals: %s", w.Body)
	}
	// Five power values leave the top zone open (the 2000 W sentinel).
	pw, _ := zl["powerZones"].([]any)
	if len(pw) != 5 || obj(pw[4])["higherLimit"] != 2000.0 {
		t.Fatalf("power zones = %v", zl["powerZones"])
	}
	// Only the zones travel: the other stored settings are left to Flow.
	if len(obj(obj(env["profile"])["settings"])) != 1 {
		t.Fatalf("settings sent beyond zoneLimits: %v", obj(env["profile"])["settings"])
	}
	text := resultText(res)
	if !strings.Contains(text, "heart rate set, speed reset to Polar's defaults, power set") {
		t.Fatalf("text = %q", text)
	}
	sp := zonesPayload(t, res)[0]
	assertJSONEqual(t, zoneAt(t, sp, "heart_rate", 1), `{"zone":1,"min_bpm":100,"max_bpm":120}`)
	if obj(sp["power"])["setting"] != "free" {
		t.Fatalf("power = %v", sp["power"])
	}
}

func TestUpdateTrainingZones_Errors(t *testing.T) {
	t.Run("no stored profile", func(t *testing.T) {
		ff := newFakeFlow(t)
		ff.on("GET", "/api/sports/profiles/"+cyclingProfileUUID, 404, "text/plain", "Profile not found")
		res := callTool(t, UpdateTrainingZonesHandler(ff.client()), map[string]any{"sport_id": 2.0, "reset": []any{"heart_rate"}})
		if !res.IsError || !strings.Contains(resultText(res), "create_sport_profile first") {
			t.Fatalf("result = %v %q", res.IsError, resultText(res))
		}
		if w := ff.writes(); len(w) != 0 {
			t.Fatalf("wrote %+v", w)
		}
	})
	t.Run("speed on a heart-rate-only sport", func(t *testing.T) {
		ff := newFakeFlow(t)
		ff.on("GET", "/api/sports/profiles/0f000000-0080-0000-0000-000000000017", 200, "application/json", swimmingProfile)
		res := callTool(t, UpdateTrainingZonesHandler(ff.client()), map[string]any{"sport_id": 23.0, "speed_kmh": []any{2.0, 3.0, 4.0, 5.0, 6.0}})
		if !res.IsError || !strings.Contains(resultText(res), "has no speed zones") {
			t.Fatalf("result = %v %q", res.IsError, resultText(res))
		}
		if w := ff.writes(); len(w) != 0 {
			t.Fatalf("wrote %+v", w)
		}
	})
	t.Run("server validation surfaces its errors", func(t *testing.T) {
		ff := newFakeFlow(t)
		ff.on("GET", "/api/sports/profiles/"+cyclingProfileUUID, 200, "application/json", cyclingCreated)
		ff.on("GET", "/api/account/users/current/user", 200, "application/json", currentUser)
		ff.on("POST", "/api/sports/profiles/"+cyclingProfileUUID+"/update-zones", 400, "text/plain",
			"Invalid update sport profile request for user 11111111 with uuid x. Service replied status INVALID_REQUEST with message 'Invalid request: Profile validation failed with 1 error(s): profile.settings.zone_limits.speed_zones[1]: Speed zone lower limit should be equal to previous zone's higher limit'")
		res := callTool(t, UpdateTrainingZonesHandler(ff.client()), map[string]any{"sport_id": 2.0, "reset": []any{"speed"}})
		if !res.IsError || !strings.Contains(resultText(res), "rejected: profile.settings.zone_limits.speed_zones[1]") {
			t.Fatalf("result = %v %q", res.IsError, resultText(res))
		}
	})
}

func TestUpdateTrainingZones_Validation(t *testing.T) {
	hr := []any{95.0, 114.0, 133.0, 152.0, 171.0, 190.0}
	runValidationCases(t, UpdateTrainingZonesHandler, []validationCase{
		{"nothing to change", map[string]any{"sport_id": 1.0}, "nothing to change"},
		{"missing sport", map[string]any{"heart_rate_bpm": hr}, "sport_id is required"},
		{"unknown sport", map[string]any{"sport_id": 999.0, "heart_rate_bpm": hr}, "not a Polar sport"},
		{"hr not a list", map[string]any{"sport_id": 1.0, "heart_rate_bpm": 150.0}, "must be an array"},
		{"hr five values", map[string]any{"sport_id": 1.0, "heart_rate_bpm": hr[:5]}, "must have 6 values"},
		{"hr descending", map[string]any{"sport_id": 1.0, "heart_rate_bpm": []any{95.0, 114.0, 133.0, 130.0, 171.0, 190.0}}, "strictly ascending"},
		{"hr span too small", map[string]any{"sport_id": 1.0, "heart_rate_bpm": []any{95.0, 96.0, 133.0, 152.0, 171.0, 190.0}}, "at least 2 bpm"},
		{"hr fractional", map[string]any{"sport_id": 1.0, "heart_rate_bpm": []any{95.5, 114.0, 133.0, 152.0, 171.0, 190.0}}, "whole number"},
		{"hr out of range", map[string]any{"sport_id": 1.0, "heart_rate_bpm": []any{95.0, 114.0, 133.0, 152.0, 171.0, 250.0}}, "between 15 and 240"},
		{"hr item not a number", map[string]any{"sport_id": 1.0, "heart_rate_bpm": []any{"95", 114.0, 133.0, 152.0, 171.0, 190.0}}, "heart_rate_bpm[0] must be a number"},
		{"speed too few", map[string]any{"sport_id": 1.0, "speed_kmh": []any{8.0, 10.0, 12.0, 14.0}}, "5 or 6 values"},
		{"speed below 1", map[string]any{"sport_id": 1.0, "speed_kmh": []any{0.5, 10.0, 12.0, 14.0, 16.0}}, "between 1 and 399"},
		{"power fractional", map[string]any{"sport_id": 2.0, "power_w": []any{100.0, 150.5, 200.0, 250.0, 300.0}}, "whole number"},
		{"power span", map[string]any{"sport_id": 2.0, "power_w": []any{100.0, 101.0, 200.0, 250.0, 300.0}}, "at least 2 W"},
		{"power above 2000", map[string]any{"sport_id": 2.0, "power_w": []any{100.0, 150.0, 200.0, 250.0, 300.0, 2500.0}}, "between 0 and 2000"},
		{"reset unknown type", map[string]any{"sport_id": 1.0, "reset": []any{"cadence"}}, "reset entries must be"},
		{"reset not a list", map[string]any{"sport_id": 1.0, "reset": "speed"}, "reset must be an array"},
		{"set and reset", map[string]any{"sport_id": 1.0, "heart_rate_bpm": hr, "reset": []any{"heart_rate"}}, "both set and listed in reset"},
	})
}

func TestDeleteSportProfile(t *testing.T) {
	t.Run("deletes", func(t *testing.T) {
		ff := newFakeFlow(t)
		ff.on("GET", "/api/sports/profiles/"+cyclingProfileUUID, 200, "application/json", cyclingCreated)
		ff.on("DELETE", "/api/sports/profiles/"+cyclingProfileUUID, 200, "", "")
		res := callTool(t, DeleteSportProfileHandler(ff.client()), map[string]any{"sport_id": 2.0})
		if res.IsError || !strings.Contains(resultText(res), "Deleted the CYCLING (sport_id 2) sport profile") {
			t.Fatalf("result = %v %q", res.IsError, resultText(res))
		}
		if w := ff.onlyWrite(); w.Method != "DELETE" || w.XRequestedWith != "XMLHttpRequest" {
			t.Fatalf("write = %+v", w)
		}
	})
	t.Run("nothing stored is a no-op", func(t *testing.T) {
		ff := newFakeFlow(t)
		ff.on("GET", "/api/sports/profiles/"+cyclingProfileUUID, 404, "text/plain", "Profile not found")
		res := callTool(t, DeleteSportProfileHandler(ff.client()), map[string]any{"sport_id": 2.0})
		if res.IsError || !strings.Contains(resultText(res), "nothing to delete") {
			t.Fatalf("result = %v %q", res.IsError, resultText(res))
		}
		if w := ff.writes(); len(w) != 0 {
			t.Fatalf("wrote %+v", w)
		}
	})
	t.Run("last profile", func(t *testing.T) {
		ff := newFakeFlow(t)
		ff.on("GET", "/api/sports/profiles/"+runningProfileUUID, 200, "application/json", runningProfile)
		ff.on("DELETE", "/api/sports/profiles/"+runningProfileUUID, 400, "text/plain", lastProfileError)
		res := callTool(t, DeleteSportProfileHandler(ff.client()), map[string]any{"sport_id": 1.0})
		if !res.IsError || !strings.Contains(resultText(res), "last sport profile") {
			t.Fatalf("result = %v %q", res.IsError, resultText(res))
		}
	})
}

func TestSportProfileUUIDMatchesFixtures(t *testing.T) {
	if got := flow.SportProfileUUID(2).String(); got != cyclingProfileUUID {
		t.Fatalf("uuid = %s", got)
	}
	if got := flow.SportProfileUUID(23).String(); got != "0f000000-0080-0000-0000-000000000017" {
		t.Fatalf("uuid = %s", got)
	}
}

// Flow 409s a write whose modified is not a full second past the stored one
// (it keeps sub-second precision but reports whole seconds), whatever the
// local clock says.
func TestUpdateTrainingZones_ModifiedStaysAheadOfStored(t *testing.T) {
	ff := newFakeFlow(t)
	future := strings.Replace(cyclingCreated, `"modified":"2026-09-30T09:35:36Z"`, `"modified":"2099-01-01T00:00:00Z"`, 1)
	ff.on("GET", "/api/sports/profiles/"+cyclingProfileUUID, 200, "application/json", future)
	ff.on("GET", "/api/account/users/current/user", 200, "application/json", currentUser)
	ff.on("POST", "/api/sports/profiles/"+cyclingProfileUUID+"/update-zones", 200, "", "")
	callTool(t, UpdateTrainingZonesHandler(ff.client()), map[string]any{"sport_id": 2.0, "reset": []any{"power"}})
	_, env := updateBody(t, ff.onlyWrite().Body)
	if env["modified"] != "2099-01-01T00:00:01Z" {
		t.Fatalf("modified = %v", env["modified"])
	}
}
