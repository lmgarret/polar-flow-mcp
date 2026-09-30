package mcp

import (
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/lmgarret/polar-flow-mcp/internal/flow"
)

// MCP-app pages for the tools below.
const (
	zonesUI    = "ui://polar-flow/zones.html"
	activityUI = "ui://polar-flow/activity.html"
	sleepUI    = "ui://polar-flow/sleep.html"
)

// registerProfileAndActivityTools registers the sport-profile writes and the
// 24/7 activity and sleep reads. Behaviour, limits and quirks come from
// internal/flow/openapi.yaml (sport profiles probed live 2026-09-29/30;
// activity and sleep units read from the Flow web UI's bundle).
//
//nolint:funlen // declarative tool table
func registerProfileAndActivityTools(s *server.MCPServer, fc *flow.Client) {
	sportIDOpt := mcpgo.WithInteger("sport_id", mcpgo.Required(), mcpgo.Min(1),
		mcpgo.Description("Polar sport id (1 = running, 2 = cycling, 23 = swimming; list_sports has them all). "+
			"A sport has at most one profile."))

	addWithUI(s, zonesUI, mcpgo.NewTool("create_sport_profile",
		mcpgo.WithDescription(
			"Create the sport profile for a sport, with Polar's default settings and training zones "+
				"(the \"Sport profiles\" page in Flow). A profile is what makes a sport's zones editable "+
				"(update_training_zones) and puts the sport on a paired watch after its next sync. "+
				"Idempotent: if the sport already has a profile, nothing changes and the stored one is "+
				"returned. Returns the profile's heart-rate / speed / power zones like get_training_zones.\n\n"+
				"Example: {\"sport_id\": 2} creates a cycling profile.",
		),
		sportIDOpt,
	), withLogging("create_sport_profile", CreateSportProfileHandler(fc)))

	addWithUI(s, zonesUI, mcpgo.NewTool("update_training_zones",
		mcpgo.WithDescription(
			"Set a sport's heart-rate, speed or power training zones by hand, or reset them to Polar's "+
				"computed defaults. The sport needs a stored profile (create_sport_profile). Zone lists not "+
				"mentioned are left as they are. Returns the saved zones (read back), like get_training_zones. "+
				"A paired watch picks the change up on its next sync.\n\n"+
				"Each list is given as ascending zone BOUNDARIES, Z1 lower first: zone n is "+
				"[value n, value n+1), and zones are contiguous by construction.\n"+
				"  • heart_rate_bpm — exactly 6 whole numbers, 15–240 bpm, each zone ≥ 2 bpm wide.\n"+
				"  • speed_kmh — 5 or 6 numbers, 1–399 km/h (give 5 for an open-ended top zone). "+
				"Always km/h, even for sports the watch shows as pace: 5:00 min/km = 12 km/h.\n"+
				"  • power_w — 5 or 6 whole numbers, 0–2000 W, each zone ≥ 2 W wide (5 = open top zone).\n"+
				"  • reset — zone types to return to Polar's defaults (computed from max HR, MAS, MAP / FTP).\n"+
				"Swimming and strength-type sports have heart-rate zones only.\n\n"+
				"Example — running HR zones from a 192 bpm max, and speed back to defaults:\n"+
				"  {\"sport_id\": 1, \"heart_rate_bpm\": [115, 134, 153, 172, 182, 192], \"reset\": [\"speed\"]}",
		),
		sportIDOpt,
		mcpgo.WithArray("heart_rate_bpm", mcpgo.WithNumberItems(),
			mcpgo.Description("6 ascending whole-number boundaries in bpm (Z1 lower … Z5 upper), 15–240, "+
				"each zone ≥ 2 bpm. E.g. [95, 114, 133, 152, 171, 190].")),
		mcpgo.WithArray("speed_kmh", mcpgo.WithNumberItems(),
			mcpgo.Description("5 or 6 ascending boundaries in km/h (Z1 lower … Z5 upper; 5 values = open-ended "+
				"Z5), 1–399. E.g. [9.5, 12, 14.6, 17.2, 19.8].")),
		mcpgo.WithArray("power_w", mcpgo.WithNumberItems(),
			mcpgo.Description("5 or 6 ascending whole-number boundaries in watts (5 values = open-ended Z5), "+
				"0–2000, each zone ≥ 2 W. E.g. [138, 188, 225, 263, 300].")),
		mcpgo.WithArray("reset",
			mcpgo.Items(map[string]any{"type": "string", "enum": []string{"heart_rate", "speed", "power"}}),
			mcpgo.Description("Zone types to reset to Polar's computed defaults. Cannot be combined with "+
				"setting the same type.")),
	), withLogging("update_training_zones", UpdateTrainingZonesHandler(fc)))

	s.AddTool(mcpgo.NewTool("delete_sport_profile",
		mcpgo.WithDescription(
			"Delete a sport's profile — its training zones and watch settings for that sport (a paired "+
				"watch drops the sport on its next sync). The zones fall back to Polar's defaults, which "+
				"get_training_zones still reports. Polar refuses to delete the account's last profile. "+
				"Reports \"nothing to delete\" when the sport has no profile.\n\n"+
				"Hosts that support elicitation put the deletion to the user for confirmation first; a "+
				"result saying the user did not confirm means the profile is still there, and is not an "+
				"error to retry around.",
		),
		sportIDOpt,
	), withLogging("delete_sport_profile", DeleteSportProfileHandler(fc)))

	addWithUI(s, activityUI, mcpgo.NewTool("get_daily_activity",
		mcpgo.WithDescription(
			"24/7 activity per day from a Polar device (the Flow diary's \"Activity\" view): steps, "+
				"step distance (metres), active time (seconds), calories, % of the daily activity goal, "+
				"inactivity alerts, the night's sleep time (seconds), time per intensity band (sleep / "+
				"sedentary / light / moderate / vigorous, seconds), the day's low / high and night-low "+
				"heart rate (bpm), and Polar's activity-benefit feedback. A single-day call also returns "+
				"the intraday activity and heart-rate curves (10-minute points, local HH:MM).\n\n"+
				"Days without synced device data have has_data false and null metrics — Flow reports "+
				"them as zeros, which are not real readings. Range ≤ 31 days; default: the last 7 days.\n\n"+
				"Example: {\"from_date\": \"2026-09-29\", \"to_date\": \"2026-09-29\"} for one day with curves.",
		),
		mcpgo.WithString("from_date",
			mcpgo.Description("First day, ISO 8601 YYYY-MM-DD (default: today - 6 days).")),
		mcpgo.WithString("to_date",
			mcpgo.Description("Last day, ISO 8601 YYYY-MM-DD (default: today). At most 31 days after from_date.")),
	), withLogging("get_daily_activity", GetDailyActivityHandler(fc)))

	addWithUI(s, sleepUI, mcpgo.NewTool("get_sleep",
		mcpgo.WithDescription(
			"Recorded nights of sleep from a Polar device (Flow's sleep report), each keyed by the date "+
				"the user woke up: fell-asleep and woke-up times (local, ISO 8601), time asleep (seconds), "+
				"Sleep Score (0–100), continuity (1–5), sleep cycles, the user's own rating, time in light / "+
				"deep / REM sleep and interruptions (seconds; stages only on Sleep Plus Stages devices), and "+
				"a hypnogram (state segments in seconds after falling asleep). Also returns averages over "+
				"the range. Empty when no sleep-tracking device is synced.\n\n"+
				"Range ≤ 365 days; default: the last 14 nights. Example: {\"from_date\": \"2026-09-01\", "+
				"\"to_date\": \"2026-09-30\"}.",
		),
		mcpgo.WithString("from_date",
			mcpgo.Description("First wake-up date, ISO 8601 YYYY-MM-DD (default: today - 13 days).")),
		mcpgo.WithString("to_date",
			mcpgo.Description("Last wake-up date, ISO 8601 YYYY-MM-DD (default: today). At most 365 days after from_date.")),
	), withLogging("get_sleep", GetSleepHandler(fc)))
}
