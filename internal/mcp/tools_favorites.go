package mcp

import (
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/lmgarret/polar-flow-mcp/internal/flow"
)

// registerFavoriteAndSessionEditTools registers the favorites, route and
// session-edit tools. Descriptions follow the RegisterTools convention: the
// behaviour, limits and quirks come from internal/flow/openapi.yaml (probed
// live 2026-09-28), translated to this layer's canonical units.
//
//nolint:funlen // declarative tool table
func registerFavoriteAndSessionEditTools(s *server.MCPServer, fc *flow.Client) {
	sportIDDesc := "Polar sport id — must exist in list_sports (Polar itself accepts unknown ids " +
		"silently, so they are rejected here). Common: 1=running, 2=cycling, 23=swimming, 11=hiking."

	s.AddTool(mcpgo.NewTool("list_favorites",
		mcpgo.WithDescription(
			"List the account's favorites: reusable training-target templates (type VOLUME, "+
				"STEADY_RACE_PACE or PHASED) and imported GPS routes (type ROUTE). Each entry has a "+
				"favorite_id (use with get_favorite / update_favorite / rename_favorite / "+
				"set_favorite_sport / schedule_favorite / delete_favorite) and an exercise_target_id "+
				"(use with get_route and set_favorite_sport). Durations are seconds, distances metres.\n\n"+
				"Example: {\"kind\": \"routes\"} lists only imported routes.",
		),
		mcpgo.WithString("kind", mcpgo.DefaultString("all"), mcpgo.Enum("all", "templates", "routes"),
			mcpgo.Description("Filter: \"all\" (default), \"templates\" (training-target favorites) or \"routes\".")),
	), withLogging("list_favorites", ListFavoritesHandler(fc)))

	s.AddTool(mcpgo.NewTool("get_favorite",
		mcpgo.WithDescription(
			"Return one favorite by favorite_id: name, description, type and its exercise targets "+
				"(sport_id, duration_s in seconds, distance_m in metres, calories, and the raw phase "+
				"tree for PHASED favorites). For PHASED favorites Polar rolls the phase goals up into "+
				"duration_s / distance_m on read-back. For a ROUTE favorite this is only a metadata "+
				"shell — call get_route for the GPS geometry.\n\nExample: {\"favorite_id\": 83468869}",
		),
		mcpgo.WithInteger("favorite_id", mcpgo.Required(), mcpgo.Min(1),
			mcpgo.Description("favorite_id from list_favorites.")),
	), withLogging("get_favorite", GetFavoriteHandler(fc)))

	favoriteGoalOpts := func(nameRequired bool) []mcpgo.ToolOption {
		nameOpts := []mcpgo.PropertyOption{mcpgo.MinLength(1), mcpgo.MaxLength(maxFavoriteNameRunes),
			mcpgo.Description("Favorite name, 1–45 characters (Polar's limit for favorites — stricter than targets).")}
		if nameRequired {
			nameOpts = append(nameOpts, mcpgo.Required())
		}
		return []mcpgo.ToolOption{
			mcpgo.WithString("name", nameOpts...),
			mcpgo.WithString("description", mcpgo.MaxLength(maxFavoriteDescRunes),
				mcpgo.Description("Free-text notes, at most 500 characters (optional).")),
			mcpgo.WithInteger("sport_id", mcpgo.DefaultNumber(1), mcpgo.Min(1), mcpgo.Description(sportIDDesc+" Default 1.")),
			mcpgo.WithInteger("duration_s", mcpgo.Min(1), mcpgo.Max(maxFavoriteDurationS),
				mcpgo.Description("VOLUME goal: duration in SECONDS, 1–359999 (99:59:59). Give exactly one of "+
					"duration_s, distance_m or phases.")),
			mcpgo.WithNumber("distance_m", mcpgo.Min(0), mcpgo.Max(maxFavoriteDistanceM),
				mcpgo.Description("VOLUME goal: distance in METRES, > 0 and ≤ 9 999 000 (5 km = 5000).")),
			mcpgo.WithArray("phases",
				mcpgo.Description("PHASED goal: ordered warmup / repeat / cooldown phases — identical shape to "+
					"create_training_target.phases (durations in seconds, distances in metres, zones 1–5)."),
				mcpgo.Items(phaseItemSchema())),
		}
	}

	createOpts := append([]mcpgo.ToolOption{mcpgo.WithDescription(
		"Create a favorite: a reusable, date-less training-target template that can later be scheduled " +
			"with schedule_favorite and synced to a watch. Returns the new favorite (read back from Polar).\n\n" +
			"Goal: exactly one of duration_s (seconds), distance_m (metres) or phases (same shape as " +
			"create_training_target). Limits enforced before sending: name 1–45 characters, description " +
			"≤ 500, duration ≤ 99:59:59, distance ≤ 9 999 000 m, sport_id must exist in list_sports.\n\n" +
			"Example A — easy 45 min run: {\"name\": \"Easy 45\", \"duration_s\": 2700, \"sport_id\": 1}\n" +
			"Example B — 6×800 m intervals: {\"name\": \"6x800\", \"sport_id\": 1, \"phases\": [\n" +
			"  {\"type\": \"warmup\", \"duration_s\": 900},\n" +
			"  {\"type\": \"repeat\", \"reps\": 6, \"goal\": {\"distance_m\": 800}, \"intensity\": {\"hr_zone\": 4}, " +
			"\"recovery\": {\"duration_s\": 90}},\n" +
			"  {\"type\": \"cooldown\", \"duration_s\": 600}]}",
	)}, favoriteGoalOpts(true)...)
	s.AddTool(mcpgo.NewTool("create_favorite", createOpts...),
		withLogging("create_favorite", CreateFavoriteHandler(fc)))

	updateOpts := append([]mcpgo.ToolOption{
		mcpgo.WithDescription(
			"Full-replace edit of a single-sport training-target favorite: name, description, sport " +
				"and goal are all overwritten by the supplied arguments (anything omitted is cleared), so " +
				"call get_favorite first and resend what should stay. The tool carries Polar's " +
				"exercise-target id over for you (Polar silently ignores an update without it). ROUTE " +
				"favorites cannot be edited this way (use rename_favorite / set_favorite_sport); " +
				"multi-sport favorites are refused. Same limits and goal rules as create_favorite. " +
				"To change only the name or sport, prefer rename_favorite / set_favorite_sport.\n\n" +
				"Example: {\"favorite_id\": 83468869, \"name\": \"Easy 50\", \"duration_s\": 3000, \"sport_id\": 1}",
		),
		mcpgo.WithInteger("favorite_id", mcpgo.Required(), mcpgo.Min(1),
			mcpgo.Description("favorite_id from list_favorites.")),
	}, favoriteGoalOpts(true)...)
	s.AddTool(mcpgo.NewTool("update_favorite", updateOpts...),
		withLogging("update_favorite", UpdateFavoriteHandler(fc)))

	s.AddTool(mcpgo.NewTool("delete_favorite",
		mcpgo.WithDescription(
			"Permanently delete a favorite — a training-target template or an imported route. "+
				"Irreversible. Scheduled targets created from it are not affected. Reports \"no favorite\" "+
				"if the id does not exist.\n\n"+
				"Hosts that support elicitation put the deletion to the user for confirmation first; a "+
				"result saying the user did not confirm means the favorite is still there, and is not an "+
				"error to retry around.\n\nExample: {\"favorite_id\": 83468869}",
		),
		mcpgo.WithInteger("favorite_id", mcpgo.Required(), mcpgo.Min(1),
			mcpgo.Description("favorite_id from list_favorites.")),
	), withLogging("delete_favorite", DeleteFavoriteHandler(fc)))

	s.AddTool(mcpgo.NewTool("rename_favorite",
		mcpgo.WithDescription(
			"Rename a favorite or route without touching anything else. The name must be 1–45 "+
				"characters and not blank (Polar would otherwise accept a whitespace-only name, or "+
				"silently truncate a route's name). Returns the favorite as read back.\n\n"+
				"Example: {\"favorite_id\": 83468869, \"name\": \"Long run — Sundays\"}",
		),
		mcpgo.WithInteger("favorite_id", mcpgo.Required(), mcpgo.Min(1),
			mcpgo.Description("favorite_id from list_favorites.")),
		mcpgo.WithString("name", mcpgo.Required(), mcpgo.MinLength(1), mcpgo.MaxLength(maxFavoriteNameRunes),
			mcpgo.Description("New name, 1–45 characters.")),
	), withLogging("rename_favorite", RenameFavoriteHandler(fc)))

	s.AddTool(mcpgo.NewTool("set_favorite_sport",
		mcpgo.WithDescription(
			"Change the sport of a favorite (or route) without touching anything else. Polar's endpoint "+
				"validates nothing — it accepts unknown sport ids and exercise-target ids with a success "+
				"reply — so this tool checks the sport against list_sports, checks the exercise target "+
				"belongs to the favorite, and verifies the change by reading the favorite back. "+
				"exercise_target_id is only needed for multi-sport favorites.\n\n"+
				"Example: {\"favorite_id\": 83468869, \"sport_id\": 2}",
		),
		mcpgo.WithInteger("favorite_id", mcpgo.Required(), mcpgo.Min(1),
			mcpgo.Description("favorite_id from list_favorites.")),
		mcpgo.WithInteger("sport_id", mcpgo.Required(), mcpgo.Min(1), mcpgo.Description(sportIDDesc)),
		mcpgo.WithInteger("exercise_target_id", mcpgo.Min(1),
			mcpgo.Description("Which exercise target to change (from get_favorite). Optional when the "+
				"favorite has exactly one.")),
	), withLogging("set_favorite_sport", SetFavoriteSportHandler(fc)))

	s.AddTool(mcpgo.NewTool("save_target_as_favorite",
		mcpgo.WithDescription(
			"Save an existing scheduled training target as a new favorite (the \"Add to favorites\" "+
				"button in the Flow target editor). Copies the name, description, type, sport and phases; "+
				"the date is dropped. The target itself is unchanged and not linked to the favorite, so "+
				"calling this twice creates two favorites. Favorite names are limited to 45 characters: "+
				"pass name when the target's name is longer.\n\n"+
				"Example: {\"target_id\": 1461116593} or {\"target_id\": 1461116593, \"name\": \"5x1km threshold\"}",
		),
		mcpgo.WithInteger("target_id", mcpgo.Required(), mcpgo.Min(1),
			mcpgo.Description("Training-target id from list_training_targets.")),
		mcpgo.WithString("name", mcpgo.MinLength(1), mcpgo.MaxLength(maxFavoriteNameRunes),
			mcpgo.Description("Favorite name (1–45 characters); defaults to the target's name.")),
	), withLogging("save_target_as_favorite", SaveTargetAsFavoriteHandler(fc)))

	s.AddTool(mcpgo.NewTool("schedule_favorite",
		mcpgo.WithDescription(
			"Schedule a favorite as a training target on a date (the diary's \"Add → Favorites\" "+
				"picker). Copies the favorite's name, description, sport and phases into a new target and "+
				"returns its target_id (usable with get_training_target / update_training_target / "+
				"delete_training_target). Past dates are allowed. If another target already starts at "+
				"that minute Polar shifts the new one by +1 minute instead of failing — the result says so. "+
				"00:00 cannot be scheduled (Polar reads midnight as \"no time\" and uses 18:00). Route "+
				"favorites cannot be scheduled.\n\n"+
				"Example: {\"favorite_id\": 83468869, \"date\": \"2026-10-20\", \"time\": \"07:30\"}",
		),
		mcpgo.WithInteger("favorite_id", mcpgo.Required(), mcpgo.Min(1),
			mcpgo.Description("favorite_id from list_favorites (a training-target favorite, not a route).")),
		mcpgo.WithString("date", mcpgo.Required(),
			mcpgo.Description("Local date, ISO 8601 YYYY-MM-DD.")),
		mcpgo.WithString("time", mcpgo.DefaultString("18:00"),
			mcpgo.Description("Local start time, 24h HH:MM (default 18:00; 00:00 is not allowed).")),
	), withLogging("schedule_favorite", ScheduleFavoriteHandler(fc)))

	s.AddTool(mcpgo.NewTool("import_route",
		mcpgo.WithDescription(
			"Import a GPX or TCX file as a route favorite (for watch navigation). Pass the file's text "+
				"in content; it is parsed here and uploaded as trackpoints, the way the Flow web UI does "+
				"(GPX: the first track — all its segments — or the first route, distances by haversine; "+
				"TCX: the first course or activity, the file's own distances). Returns the new favorite_id "+
				"and exercise_target_id, point count and length in metres.\n\n"+
				"Checked before upload (Polar would otherwise silently truncate or drop data): at least 2 "+
				"points, latitudes in [-90, 90] and longitudes in [-180, 180], non-zero length, name 1–45 "+
				"characters, file ≤ 25 MB, sport_id in list_sports.\n\n"+
				"Example: {\"content\": \"<?xml version=\\\"1.0\\\"?><gpx version=\\\"1.1\\\" "+
				"xmlns=\\\"http://www.topografix.com/GPX/1/1\\\"><trk><name>Park loop</name><trkseg>"+
				"<trkpt lat=\\\"48.8566\\\" lon=\\\"2.3522\\\"><ele>35</ele></trkpt>"+
				"<trkpt lat=\\\"48.8576\\\" lon=\\\"2.3532\\\"><ele>36</ele></trkpt></trkseg></trk></gpx>\", "+
				"\"sport_id\": 1}",
		),
		mcpgo.WithString("content", mcpgo.Required(), mcpgo.MinLength(1),
			mcpgo.Description("Full text of the GPX or TCX file (XML).")),
		mcpgo.WithString("format", mcpgo.DefaultString("auto"), mcpgo.Enum("auto", "gpx", "tcx"),
			mcpgo.Description("File format; \"auto\" (default) detects it from the XML root element.")),
		mcpgo.WithString("name", mcpgo.MinLength(1), mcpgo.MaxLength(45),
			mcpgo.Description("Route name, 1–45 characters. Default: the name in the file, else \"Imported route\".")),
		mcpgo.WithInteger("sport_id", mcpgo.Min(1),
			mcpgo.Description("Optional sport to bind the route to. "+sportIDDesc)),
	), withLogging("import_route", ImportRouteHandler(fc)))

	s.AddTool(mcpgo.NewTool("get_route",
		mcpgo.WithDescription(
			"Return a route favorite's geometry: name, total distance_m, sport and the waypoints "+
				"(lat, lon, altitude_m). Polar's endpoint is keyed by the exercise_target_id — NOT the "+
				"favorite_id — so pass exercise_target_id, or pass favorite_id and the tool looks it up. "+
				"Long routes are evenly down-sampled to max_points (default 500; first and last points "+
				"kept); point_count is always the full count.\n\n"+
				"Example: {\"exercise_target_id\": 1539651678} or {\"favorite_id\": 83468958, \"max_points\": 100}",
		),
		mcpgo.WithInteger("exercise_target_id", mcpgo.Min(1),
			mcpgo.Description("exercise_target_id of a ROUTE favorite (from list_favorites or import_route).")),
		mcpgo.WithInteger("favorite_id", mcpgo.Min(1),
			mcpgo.Description("Alternatively, the route's favorite_id.")),
		mcpgo.WithInteger("max_points", mcpgo.DefaultNumber(defaultRouteMaxPoints), mcpgo.Min(2), mcpgo.Max(maxRouteMaxPoints),
			mcpgo.Description("Maximum waypoints to return (2–100000, default 500).")),
	), withLogging("get_route", GetRouteHandler(fc)))

	s.AddTool(mcpgo.NewTool("delete_training_session",
		mcpgo.WithDescription(
			"Permanently delete a completed training session by id. Irreversible: the session leaves "+
				"the diary, weekly totals, progress summaries and training load. Only call this when the "+
				"user asks. Polar's delete reports success even for ids that do not exist, so the tool "+
				"checks the session exists first (reports \"no session\" otherwise) and verifies it is gone "+
				"afterwards.\n\n"+
				"Hosts that support elicitation put the deletion to the user for confirmation first; a "+
				"result saying the user did not confirm means the session is still there, and is not an "+
				"error to retry around.\n\nExample: {\"session_id\": 8429796771}",
		),
		mcpgo.WithInteger("session_id", mcpgo.Required(), mcpgo.Min(1),
			mcpgo.Description("Numeric session id from list_training_sessions.")),
	), withLogging("delete_training_session", DeleteTrainingSessionHandler(fc)))

	s.AddTool(mcpgo.NewTool("edit_training_session",
		mcpgo.WithDescription(
			"Edit a completed training session — what the Flow diary's \"Edit session\" form allows. "+
				"Pass only the fields to change; everything else keeps its current value. The date/time "+
				"cannot be changed. Returns the session summary as read back.\n\n"+
				"note and feeling can be changed on any session. The other fields (name, sport_id, "+
				"duration_s, distance_m, hr_avg, hr_max, kcal, speed_kmh) are only editable on "+
				"single-exercise sessions entered manually; recorded (device) sessions are refused.\n\n"+
				"Units and limits (checked here, because Polar answers any violation with an "+
				"unexplained server error): name 1–100 characters; note ≤ 10 000 characters (\"\" clears "+
				"it); feeling 1 (worst) – 5 (best); duration_s 1–359999 SECONDS; distance_m 0–9 999 000 "+
				"METRES; hr_avg / hr_max 0–240 bpm (0 clears) and hr_max ≥ hr_avg; kcal 0–65535; "+
				"speed_kmh 0–399 km/h; sport_id must exist in list_sports. A feeling cannot be removed "+
				"once set.\n\n"+
				"Example — fix the distance and add a note: {\"session_id\": 8429796771, \"distance_m\": 10200, "+
				"\"note\": \"Windy, legs heavy\", \"feeling\": 3}",
		),
		mcpgo.WithInteger("session_id", mcpgo.Required(), mcpgo.Min(1),
			mcpgo.Description("Numeric session id from list_training_sessions.")),
		mcpgo.WithString("name", mcpgo.MinLength(1), mcpgo.MaxLength(100),
			mcpgo.Description("New session title, 1–100 characters.")),
		mcpgo.WithString("note", mcpgo.MaxLength(10000),
			mcpgo.Description("New note, up to 10 000 characters; \"\" clears it.")),
		mcpgo.WithInteger("feeling", mcpgo.Min(1), mcpgo.Max(5),
			mcpgo.Description("How the session felt: 1 = bad, 2 = not so good, 3 = average, 4 = good, 5 = great.")),
		mcpgo.WithInteger("sport_id", mcpgo.Min(1), mcpgo.Description(sportIDDesc)),
		mcpgo.WithInteger("duration_s", mcpgo.Min(1), mcpgo.Max(359999),
			mcpgo.Description("Duration in SECONDS, 1–359999 (e.g. 3600 = 1 h).")),
		mcpgo.WithNumber("distance_m", mcpgo.Min(0), mcpgo.Max(9999000),
			mcpgo.Description("Distance in METRES, 0–9 999 000 (e.g. 10200 = 10.2 km).")),
		mcpgo.WithInteger("hr_avg", mcpgo.Min(0), mcpgo.Max(240),
			mcpgo.Description("Average heart rate in bpm, 0–240 (0 clears it).")),
		mcpgo.WithInteger("hr_max", mcpgo.Min(0), mcpgo.Max(240),
			mcpgo.Description("Maximum heart rate in bpm, 0–240 (0 clears it); must be ≥ hr_avg.")),
		mcpgo.WithInteger("kcal", mcpgo.Min(0), mcpgo.Max(65535),
			mcpgo.Description("Kilocalories, 0–65535.")),
		mcpgo.WithNumber("speed_kmh", mcpgo.Min(0), mcpgo.Max(399),
			mcpgo.Description("Average speed in km/h, 0–399.")),
	), withLogging("edit_training_session", EditTrainingSessionHandler(fc)))
}
