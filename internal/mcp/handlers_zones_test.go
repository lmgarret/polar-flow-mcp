package mcp

import (
	"encoding/json"
	"strings"
	"testing"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
)

// Wire bodies captured live 2026-09-29 (test account: age 30, FTP 250).
const (
	runningProfileUUID = "0f000000-0080-0000-0000-000000000001"
	profilesRunning    = `[{"uuid":"0f000000-0080-0000-0000-000000000001","userId":"11111111","created":"2026-09-29T09:09:24Z","modified":"2026-09-29T09:09:24Z","profile":{"sportId":1,"productSettings":[],"subProfileUuids":[]},"legacyProfiles":{}}]`
	runningProfile     = `{"uuid":"0f000000-0080-0000-0000-000000000001","userId":"0","created":"2026-09-29T09:09:24Z","modified":"2026-09-29T09:09:24Z","profile":{"sportId":1,"settings":{"sensorBroadcastingHr":false,"hrZoneLockAvailable":true,"speedZoneLockAvailable":false,"powerZoneLockAvailable":false,"volume":{"volume":60},"speedView":"SPEED_VIEW_PACE","zoneLimits":{"heartRateZones":[{"lowerLimit":95,"higherLimit":114},{"lowerLimit":114,"higherLimit":133},{"lowerLimit":133,"higherLimit":152},{"lowerLimit":152,"higherLimit":171},{"lowerLimit":171,"higherLimit":190}],"speedZones":[{"lowerLimit":9.451075,"higherLimit":12.02864},{"lowerLimit":12.02864,"higherLimit":14.606206},{"lowerLimit":14.606206,"higherLimit":17.183771},{"lowerLimit":17.183771,"higherLimit":19.761337},{"lowerLimit":19.761337,"higherLimit":399.0}],"powerZones":[{"lowerLimit":221,"higherLimit":283},{"lowerLimit":283,"higherLimit":343},{"lowerLimit":343,"higherLimit":403},{"lowerLimit":403,"higherLimit":464},{"lowerLimit":464,"higherLimit":2000}],"heartRateSettingSource":"HEART_RATE_ZONE_SETTING_SOURCE_DEFAULT","powerSettingSource":"POWER_ZONE_SETTING_SOURCE_DEFAULT","speedSettingSource":"SPEED_ZONE_SETTING_SOURCE_DEFAULT","powerZoneCalculationMethod":"POWER_ZONE_CALCULATION_METHOD_MAP_BASED","speedZoneCalculationMethod":"SPEED_ZONE_CALCULATION_METHOD_MAS_BASED"},"trainingReminder":{"type":"TRAINING_REMINDER_TYPE_OFF","text":""},"powerView":"POWER_VIEW_WATT","strideSpeedSource":"STRIDE_SPEED_SOURCE_STRIDE","swimmingUnits":"SWIMMING_UNITS_METERS","remoteButtonActions":[]},"sportFactor":1.4,"productSettings":[],"subProfileUuids":[],"maximumAerobicSpeed":{"speed":17.183771,"source":"MAS_SOURCE_ESTIMATED"},"maximumAerobicPower":{"power":403,"source":"MAP_SOURCE_ESTIMATED"}},"legacyProfiles":{}}`
	cyclingRecalc      = `{"uuid":"0f000000-0080-0000-0000-000000000002","userId":"11111111","profile":{"sportId":2,"settings":{"zoneLimits":{"heartRateZones":[{"lowerLimit":95,"higherLimit":114},{"lowerLimit":114,"higherLimit":133},{"lowerLimit":133,"higherLimit":152},{"lowerLimit":152,"higherLimit":171},{"lowerLimit":171,"higherLimit":190}],"speedZones":[{"lowerLimit":10.0,"higherLimit":20.0},{"lowerLimit":20.0,"higherLimit":30.0},{"lowerLimit":30.0,"higherLimit":40.0},{"lowerLimit":40.0,"higherLimit":50.0},{"lowerLimit":50.0,"higherLimit":399.0}],"powerZones":[{"lowerLimit":138,"higherLimit":188},{"lowerLimit":188,"higherLimit":225},{"lowerLimit":225,"higherLimit":263},{"lowerLimit":263,"higherLimit":300},{"lowerLimit":300,"higherLimit":2000}],"heartRateSettingSource":"HEART_RATE_ZONE_SETTING_SOURCE_DEFAULT","powerSettingSource":"POWER_ZONE_SETTING_SOURCE_DEFAULT","speedSettingSource":"SPEED_ZONE_SETTING_SOURCE_DEFAULT","powerZoneCalculationMethod":"POWER_ZONE_CALCULATION_METHOD_FTP_BASED","speedZoneCalculationMethod":"SPEED_ZONE_CALCULATION_METHOD_SPORT_SPECIFIC_PREDEFINED"},"remoteButtonActions":[]},"productSettings":[],"subProfileUuids":[],"functionalThresholdPower":{"power":250,"source":"FTP_SOURCE_ESTIMATED"}},"legacyProfiles":{}}`
	currentUser        = `{"user":{"userName":"test@example.com","firstName":"Test","lastName":"User","nickName":null,"id":11111111,"pictureUrl":null,"country":"FR"},"localizationInfo":{"firstDayOfWeek":"MONDAY","measurementUnit":"METRIC","dateFormat":"DAY_MONTH_YEAR","dateSeparator":"HYPHEN","timeFormat":"TIMEFORMAT_24","timeFormatSeparator":"COLON","timeZone":"-02:00","flowLanguage":"fr","userAgentLanguage":"en"},"physicalInfo":{"birthday":[1995,11,2],"sex":"MALE","trainingBackground":"REGULAR","typicalDay":null,"weight":65.0,"height":183.0,"maximumHeartRate":null,"restingHeartRate":null,"aerobicThreshold":null,"anaerobicThreshold":null,"vo2Max":null,"metThreshold":null,"weeklyRecoveryTimeSum":null,"functionalThresholdPower":250,"speedCalibrationOffset":null,"sleepGoal":null,"maximumAerobicPower":403},"sportProfiles":[],"applicationRoles":[],"capabilities":[],"productCapabilities":[]}`
)

// zonesPayload decodes the machine-readable block of a get_training_zones result.
func zonesPayload(t *testing.T, res *mcpgo.CallToolResult) []map[string]any {
	t.Helper()
	if len(res.Content) != 2 {
		t.Fatalf("want a text block and a payload block, got %d blocks", len(res.Content))
	}
	tc, _ := res.Content[1].(mcpgo.TextContent)
	var p struct {
		Type   string           `json:"type"`
		Sports []map[string]any `json:"sports"`
	}
	if err := json.Unmarshal([]byte(tc.Text), &p); err != nil {
		t.Fatalf("payload is not JSON: %v\n%s", err, tc.Text)
	}
	if p.Type != "training_zones" {
		t.Fatalf("type = %q", p.Type)
	}
	return p.Sports
}

// obj asserts a decoded JSON object; nil (and so no keys) otherwise.
func obj(v any) map[string]any { m, _ := v.(map[string]any); return m }

// zoneAt returns sports[i][set].zones[z-1] re-encoded as JSON, for compact asserts.
func zoneAt(t *testing.T, sport map[string]any, set string, z int) string {
	t.Helper()
	s, ok := sport[set].(map[string]any)
	if !ok {
		t.Fatalf("sport has no %s set: %v", set, sport)
	}
	zones, _ := s["zones"].([]any)
	if len(zones) != 5 {
		t.Fatalf("%s has %d zones, want 5", set, len(zones))
	}
	b, _ := json.Marshal(zones[z-1])
	return string(b)
}

func TestGetTrainingZones_StoredProfile(t *testing.T) {
	ff := newFakeFlow(t)
	ff.on("GET", "/api/sports/profiles", 200, "application/json", profilesRunning)
	ff.on("GET", "/api/sports/profiles/"+runningProfileUUID, 200, "application/json", runningProfile)
	res := callTool(t, GetTrainingZonesHandler(ff.client()), map[string]any{"sport_id": 1.0})
	if res.IsError {
		t.Fatalf("error: %s", resultText(res))
	}
	if w := ff.writes(); len(w) != 0 {
		t.Fatalf("a read must not write: %+v", w)
	}
	sports := zonesPayload(t, res)
	if len(sports) != 1 {
		t.Fatalf("sports = %v", sports)
	}
	sp := sports[0]
	if sp["source"] != "profile" || sp["sport_name"] != "RUNNING" || sp["sport_category"] != "run" {
		t.Fatalf("header = %v", sp)
	}
	assertJSONEqual(t, zoneAt(t, sp, "heart_rate", 4), `{"zone":4,"min_bpm":152,"max_bpm":171}`)
	assertJSONEqual(t, zoneAt(t, sp, "speed", 1), `{"zone":1,"min_kmh":9.45,"max_kmh":12.03,"slowest_pace_s_per_km":381,"fastest_pace_s_per_km":299}`)
	// The 399 km/h and 2000 W ceilings are "no upper limit" sentinels.
	assertJSONEqual(t, zoneAt(t, sp, "speed", 5), `{"zone":5,"min_kmh":19.76,"max_kmh":null,"slowest_pace_s_per_km":182,"fastest_pace_s_per_km":null}`)
	assertJSONEqual(t, zoneAt(t, sp, "power", 5), `{"zone":5,"min_w":464,"max_w":null}`)
	speed := obj(sp["speed"])
	if speed["setting"] != "default" || speed["method"] != "mas_based" || speed["speed_view"] != "pace" {
		t.Fatalf("speed meta = %v", speed)
	}
	th, _ := json.Marshal(sp["thresholds"])
	assertJSONEqual(t, string(th), `{"mas_kmh":17.18,"mas_source":"estimated","map_w":403,"map_source":"estimated"}`)
	text := resultText(res)
	for _, want := range []string{"RUNNING (sport_id 1, stored sport profile)", "Z4 152–171", "Z1 9.45–12.03 (6:21–4:59/km)", "Z5 ≥19.76 (faster than 3:02/km)", "Z5 ≥464", "MAS 17.18 km/h (estimated)"} {
		if !strings.Contains(text, want) {
			t.Errorf("text lacks %q:\n%s", want, text)
		}
	}
}

func TestGetTrainingZones_DefaultsWithoutProfile(t *testing.T) {
	ff := newFakeFlow(t)
	ff.on("GET", "/api/sports/profiles", 200, "application/json", profilesRunning)
	ff.on("GET", "/api/account/users/current/user", 200, "application/json", currentUser)
	ff.on("POST", "/api/sports/profiles/0f000000-0080-0000-0000-000000000002/recalculate", 200, "application/json", cyclingRecalc)
	res := callTool(t, GetTrainingZonesHandler(ff.client()), map[string]any{"sport_id": 2.0})
	if res.IsError {
		t.Fatalf("error: %s", resultText(res))
	}
	w := ff.onlyWrite()
	if w.XRequestedWith != "XMLHttpRequest" {
		t.Fatalf("missing X-Requested-With")
	}
	// recalculate persists nothing; it fills every zone list whose source is set.
	assertJSONEqual(t, w.Body, `{"uuid":"0f000000-0080-0000-0000-000000000002","userId":11111111,"profile":{"sportId":2,"settings":{"zoneLimits":{"heartRateSettingSource":"HEART_RATE_ZONE_SETTING_SOURCE_DEFAULT","speedSettingSource":"SPEED_ZONE_SETTING_SOURCE_DEFAULT","powerSettingSource":"POWER_ZONE_SETTING_SOURCE_DEFAULT"}}}}`)
	sp := zonesPayload(t, res)[0]
	if sp["source"] != "default" || sp["sport_name"] != "CYCLING" {
		t.Fatalf("header = %v", sp)
	}
	assertJSONEqual(t, zoneAt(t, sp, "power", 3), `{"zone":3,"min_w":225,"max_w":263}`)
	assertJSONEqual(t, zoneAt(t, sp, "speed", 2), `{"zone":2,"min_kmh":20,"max_kmh":30,"slowest_pace_s_per_km":180,"fastest_pace_s_per_km":120}`)
	if m := obj(sp["power"])["method"]; m != "ftp_based" {
		t.Fatalf("power method = %v", m)
	}
	if _, has := obj(sp["speed"])["speed_view"]; has {
		t.Fatalf("recalculate output has no speed view; none should be invented")
	}
	if text := resultText(res); !strings.Contains(text, "Polar defaults — no profile stored") || !strings.Contains(text, "FTP 250 W (estimated)") ||
		!strings.Contains(text, "Z2 20–30 ·") {
		t.Fatalf("text = %s", resultText(res))
	}
}

func TestGetTrainingZones_HeartRateOnlySport(t *testing.T) {
	ff := newFakeFlow(t)
	ff.on("GET", "/api/sports/profiles", 200, "application/json", `[]`)
	ff.on("GET", "/api/account/users/current/user", 200, "application/json", currentUser)
	ff.on("POST", "/api/sports/profiles/0f000000-0080-0000-0000-000000000017/recalculate", 200, "application/json",
		`{"uuid":"0f000000-0080-0000-0000-000000000017","userId":"11111111","profile":{"sportId":23,"settings":{"zoneLimits":{"heartRateZones":[{"lowerLimit":95,"higherLimit":114},{"lowerLimit":114,"higherLimit":133},{"lowerLimit":133,"higherLimit":152},{"lowerLimit":152,"higherLimit":171},{"lowerLimit":171,"higherLimit":190}],"speedZones":[],"powerZones":[],"heartRateSettingSource":"HEART_RATE_ZONE_SETTING_SOURCE_DEFAULT","powerSettingSource":"POWER_ZONE_SETTING_SOURCE_DEFAULT","speedSettingSource":"SPEED_ZONE_SETTING_SOURCE_DEFAULT","powerZoneCalculationMethod":"POWER_ZONE_CALCULATION_METHOD_UNKNOWN","speedZoneCalculationMethod":"SPEED_ZONE_CALCULATION_METHOD_UNKNOWN"},"remoteButtonActions":[]},"productSettings":[],"subProfileUuids":[]},"legacyProfiles":{}}`)
	res := callTool(t, GetTrainingZonesHandler(ff.client()), map[string]any{"sport_id": 23.0})
	sp := zonesPayload(t, res)[0]
	if _, has := sp["speed"]; has {
		t.Fatalf("swimming has no speed zones: %v", sp)
	}
	if _, has := sp["power"]; has {
		t.Fatalf("swimming has no power zones: %v", sp)
	}
	if _, has := sp["thresholds"]; has {
		t.Fatalf("no thresholds were returned: %v", sp)
	}
	if strings.Contains(resultText(res), "Speed") {
		t.Fatalf("text = %s", resultText(res))
	}
}

func TestGetTrainingZones_AllStoredProfiles(t *testing.T) {
	t.Run("none stored", func(t *testing.T) {
		ff := newFakeFlow(t)
		ff.on("GET", "/api/sports/profiles", 200, "application/json", `[]`)
		res := callTool(t, GetTrainingZonesHandler(ff.client()), map[string]any{})
		if res.IsError || len(zonesPayload(t, res)) != 0 || !strings.Contains(resultText(res), "Pass sport_id") {
			t.Fatalf("got %s", resultText(res))
		}
		if reqs := ff.requests(); len(reqs) != 1 {
			t.Fatalf("only the list should be read, got %+v", reqs)
		}
	})
	t.Run("each stored profile", func(t *testing.T) {
		ff := newFakeFlow(t)
		cycling := strings.Replace(profilesRunning, `"sportId":1`, `"sportId":2`, 1)
		cycling = strings.Replace(cycling, runningProfileUUID, "0f000000-0080-0000-0000-000000000002", 1)
		ff.on("GET", "/api/sports/profiles", 200, "application/json", "["+strings.Trim(profilesRunning, "[]")+","+strings.Trim(cycling, "[]")+"]")
		ff.on("GET", "/api/sports/profiles/"+runningProfileUUID, 200, "application/json", runningProfile)
		ff.on("GET", "/api/sports/profiles/0f000000-0080-0000-0000-000000000002", 404, "text/plain", "Profile not found")
		res := callTool(t, GetTrainingZonesHandler(ff.client()), map[string]any{})
		sports := zonesPayload(t, res)
		// The cycling profile vanished between the list and the read: skipped.
		if len(sports) != 1 || sports[0]["sport_id"] != 1.0 {
			t.Fatalf("sports = %v", sports)
		}
		if len(ff.writes()) != 0 {
			t.Fatalf("listing must not compute defaults")
		}
	})
}

func TestGetTrainingZones_UpstreamError(t *testing.T) {
	ff := newFakeFlow(t)
	ff.on("GET", "/api/sports/profiles", 200, "application/json", `[]`)
	ff.on("GET", "/api/account/users/current/user", 200, "application/json", currentUser)
	ff.on("POST", "/api/sports/profiles/0f000000-0080-0000-0000-000000000001/recalculate", 400, "text/plain; charset=UTF-8",
		"Invalid recalculate sport profile request for user 11111111. Service replied status INVALID_REQUEST")
	res := callTool(t, GetTrainingZonesHandler(ff.client()), map[string]any{"sport_id": 1.0})
	if !res.IsError || !strings.Contains(resultText(res), "INVALID_REQUEST") {
		t.Fatalf("got %s", resultText(res))
	}
}

func TestGetTrainingZones_Validation(t *testing.T) {
	runValidationCases(t, GetTrainingZonesHandler, []validationCase{
		{"sport_id zero", map[string]any{"sport_id": 0.0}, "sport_id must be between 1"},
		{"sport_id fractional", map[string]any{"sport_id": 1.5}, "whole number"},
		{"sport_id wrong type", map[string]any{"sport_id": "running"}, "sport_id"},
		{"sport_id unknown", map[string]any{"sport_id": 9999.0}, "not a Polar sport"},
	})
}
