// Package mcp registers Polar Flow MCP tools against a shared *flow.Client.
//
// Single-user mode: the server acts as the single Polar account configured in
// POLAR_EMAIL / POLAR_PASSWORD. The proxy-identity layer that lived here
// previously (when the project wrapped the official AccessLink OAuth API) is
// gone. To serve multiple accounts, run one instance per account.
package mcp

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/lmgarret/polar-flow-mcp/internal/flow"
)

// sensitiveArgKeys holds arg names whose values must not appear in logs.
var sensitiveArgKeys = map[string]bool{
	"name": true, "note": true, "description": true, "phases": true,
}

func safeArgs(args map[string]any) map[string]any {
	if len(args) == 0 {
		return nil
	}
	out := make(map[string]any, len(args))
	for k, v := range args {
		if sensitiveArgKeys[k] {
			out[k] = "[redacted]"
		} else {
			out[k] = v
		}
	}
	return out
}

func withLogging(name string, h func(context.Context, mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error)) func(context.Context, mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	return func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		start := time.Now()
		slog.Info("tool: call", "tool", name, "args", safeArgs(req.GetArguments()))
		result, err := h(ctx, req)
		ms := time.Since(start).Milliseconds()
		switch {
		case err != nil:
			slog.Warn("tool: error", "tool", name, "duration_ms", ms, "error", err)
		case result != nil && result.IsError:
			msg := ""
			if len(result.Content) > 0 {
				if t, ok := result.Content[0].(mcpgo.TextContent); ok {
					msg = t.Text
				}
			}
			slog.Warn("tool: tool_error", "tool", name, "duration_ms", ms, "error", msg)
		default:
			slog.Info("tool: done", "tool", name, "duration_ms", ms)
		}
		return result, err
	}
}

func uiMeta(resourceURI string) *mcpgo.Meta {
	return mcpgo.NewMetaFromMap(map[string]any{
		"ui": map[string]any{"resourceUri": resourceURI},
	})
}

// widgetOutputSchema is a permissive object schema advertised by every
// UI-bound tool. Declaring an outputSchema is what makes the host (Claude
// Desktop) forward a result's structuredContent to the MCP-app iframe: without
// one, the host drops structuredContent from the ui/notifications/tool-result
// it delivers, and the app hangs on "Loading…" with no data. It is deliberately
// permissive ({"type":"object"}) — we do not enable server-side output
// validation (WithOutputSchemaValidation), so this only advertises "returns an
// object" and never rejects a payload.
var widgetOutputSchema = json.RawMessage(`{"type":"object"}`)

// bindUI links a tool to its MCP-app UI resource and advertises structured
// output so the host delivers the result's structuredContent to that UI.
func bindUI(t *mcpgo.Tool, resourceURI string) {
	t.Meta = uiMeta(resourceURI)
	t.RawOutputSchema = widgetOutputSchema
}

// Delivering data to the MCP-app UIs: current hosts (Claude Desktop, tested
// 2026-07) drop the result's structuredContent from the ui/notifications/tool-
// result they forward to the iframe, even when the tool advertises an
// outputSchema — so a text content block is the only channel the app can read.
// The helpers below always set StructuredContent (correct per spec; preferred by
// hosts that honour it) AND expose the same tagged payload as text. Every widget
// payload is a map with a "type" discriminator; the UIs parse the first text
// block that is a JSON object carrying that "type" field. See internal/mcp/ui/.

// widgetResult marshals the payload to the sole text block and mirrors it into
// StructuredContent. Use for read tools whose payload already is the full data.
func widgetResult(payload map[string]any) *mcpgo.CallToolResult {
	b, _ := json.MarshalIndent(payload, "", "  ")
	result := mcpgo.NewToolResultText(string(b))
	result.StructuredContent = payload
	return result
}

// widgetResultText keeps a human-readable summary as the primary text block (for
// the model) and appends the tagged payload as a second, machine-readable block
// (for the UI). Use for confirmation / list tools whose sentence carries meaning.
func widgetResultText(modelText string, payload map[string]any) *mcpgo.CallToolResult {
	result := mcpgo.NewToolResultText(modelText)
	result.StructuredContent = payload
	if b, err := json.Marshal(payload); err == nil {
		result.Content = append(result.Content, mcpgo.NewTextContent(string(b)))
	}
	return result
}

// RegisterTools registers all Polar Flow MCP tools with the given server.
//
// Documentation convention (read before adding or editing a tool):
// the vendored OpenAPI spec, internal/flow/openapi.yaml, is the source of truth
// for endpoint behaviour, units, quirks, and worked request examples — and ogen
// mirrors every schema `description` as a Go doc comment in
// internal/flow/gen/oas_schemas_gen.go. When writing a tool or parameter
// description here, lift the relevant detail from those (units, conventions,
// edge cases) and translate it to this layer's simplified, user-facing
// parameters (e.g. flat phases, metres, seconds, intensity labels) rather than
// the raw wire fields. Aim for descriptions rich enough that a caller never has
// to read the spec: state units explicitly, and give a concrete worked example
// in the tool description for any tool with non-trivial inputs.
func RegisterTools(s *server.MCPServer, fc *flow.Client) {
	t := mcpgo.NewTool("get_user_info",
		mcpgo.WithDescription(
			"Return identity, country, and basic profile info for the linked Polar Flow "+
				"account. Returns the account's numeric user id (used internally to scope "+
				"session queries), email, first/last name, and country. Takes no arguments. "+
				"Good first call to confirm which account the server is driving.",
		),
	)
	bindUI(&t, "ui://polar-flow/user.html")
	s.AddTool(t, withLogging("get_user_info", GetUserInfoHandler(fc)))

	s.AddTool(mcpgo.NewTool("list_sports",
		mcpgo.WithDescription(
			"Return the full Polar sport catalogue as a map of numeric sport id to its "+
				"name constant (e.g. \"1\": \"RUNNING\", \"2\": \"CYCLING\", \"23\": \"SWIMMING\"). "+
				"These ids are the sport_id values accepted by create_training_target and "+
				"create_training_session. Call this to discover the id for any non-running "+
				"sport before creating a workout. Takes no arguments. The catalogue is a "+
				"moving snapshot — Polar adds sports over time, so re-fetch rather than "+
				"hard-coding ids.",
		),
	), withLogging("list_sports", ListSportsHandler(fc)))

	ct := mcpgo.NewTool("create_training_target",
		mcpgo.WithDescription(
			"Create a scheduled Polar Flow training target (a planned workout in the diary). "+
				"Returns the new target's numeric id.\n\n"+
				"Two shapes:\n"+
				"  • VOLUME target (no phases): set duration_s OR distance_m at the top level. "+
				"This creates a single open-goal block — use for easy/general runs.\n"+
				"  • PHASED target (with phases): omit duration_s/distance_m and provide an "+
				"ordered phases list of warmup / repeat / cooldown blocks.\n\n"+
				"Units: distances are METRES (the Flow UI shows km — 5 km = 5000), durations "+
				"are SECONDS. Intensity is expressed as HR zones 1–5, either directly "+
				"(intensity.hr_zone) or via an effort label that maps to a zone "+
				"(easy→1–2, aerobic→2, tempo→3, threshold→4, vo2max→5).\n\n"+
				"Example A — 35-minute easy run (VOLUME):\n"+
				"  name: \"Easy 35 min\", date: \"2026-06-14\", duration_s: 2100, sport_id: 1\n\n"+
				"Example B — 10 min warmup, 5×1 km @ threshold with 2 min jog recovery, 10 min cooldown:\n"+
				"  name: \"5x1km Threshold\", date: \"2026-06-02\", time: \"09:00\", sport_id: 1,\n"+
				"  phases: [\n"+
				"    {\"type\": \"warmup\", \"duration_s\": 600},\n"+
				"    {\"type\": \"repeat\", \"reps\": 5, \"name\": \"1km rep\", \"goal\": {\"distance_m\": 1000}, "+
				"\"intensity\": {\"label\": \"threshold\"}, \"recovery\": {\"duration_s\": 120}},\n"+
				"    {\"type\": \"cooldown\", \"duration_s\": 600}\n"+
				"  ]\n"+
				"Each phase accepts an optional name (persisted verbatim by Polar); it defaults to "+
				"a type-derived label (\"Warm-up\", \"Work\", \"Recovery\", \"Cool-down\") when omitted.",
		),
		mcpgo.WithString("name", mcpgo.Required(),
			mcpgo.Description("Display name shown in the diary (e.g. \"5x1km Threshold\").")),
		mcpgo.WithString("date", mcpgo.Required(),
			mcpgo.Description("Scheduled local date, ISO 8601 YYYY-MM-DD (e.g. 2026-06-02). "+
				"The server applies the account's timezone.")),
		mcpgo.WithString("time", mcpgo.DefaultString("18:00"),
			mcpgo.Description("Scheduled local start time, 24h HH:MM (default: 18:00).")),
		mcpgo.WithNumber("sport_id", mcpgo.DefaultNumber(1),
			mcpgo.Description("Polar sport id (default 1 = running). Common ids: 2=cycling, "+
				"23=swimming, 15=strength_training, 11=hiking, 68=triathlon. "+
				"Call list_sports for the full id→name catalogue.")),
		mcpgo.WithString("description",
			mcpgo.Description("Free-text notes for the workout (optional).")),
		mcpgo.WithNumber("duration_s",
			mcpgo.Description("VOLUME target total duration in seconds (e.g. 2100 = 35 min). "+
				"Required when phases is omitted and distance_m is not set. Ignored when phases is provided.")),
		mcpgo.WithNumber("distance_m",
			mcpgo.Description("VOLUME target total distance in metres (e.g. 10000 = 10 km). "+
				"Required when phases is omitted and duration_s is not set. Ignored when phases is provided.")),
		mcpgo.WithArray("phases",
			mcpgo.Description(
				"Ordered list of workout phases. Omit (and set duration_s or distance_m) for a VOLUME target. Each phase "+
					"is one of three types:\n"+
					"  • warmup / cooldown — needs duration_s (seconds, > 0); run at easy intensity.\n"+
					"  • repeat — an interval block repeated reps times (reps ≥ 2). Needs goal "+
					"(distance_m OR duration_s), optional intensity (hr_zone 1–5 or label), and "+
					"optional recovery (duration_s) inserted between reps.\n"+
					"Note: structured work is only expressible via repeat (reps must be ≥ 2). For a "+
					"single continuous effort with no intervals, omit phases and use a VOLUME target."),
			mcpgo.Items(phaseItemSchema()),
		),
	)
	bindUI(&ct, "ui://polar-flow/targets.html")
	s.AddTool(ct, withLogging("create_training_target", CreateTrainingTargetHandler(fc)))

	ltt := mcpgo.NewTool("list_training_targets",
		mcpgo.WithDescription(
			"List scheduled (future or planned) Polar Flow training targets in a date range, "+
				"each with its numeric id, title, and start time. Use the ids with "+
				"get_training_target / update_training_target / delete_training_target. "+
				"Default range: today through +30 days.",
		),
		mcpgo.WithString("from_date",
			mcpgo.Description("Start date, ISO 8601 YYYY-MM-DD (default: today).")),
		mcpgo.WithString("to_date",
			mcpgo.Description("End date, ISO 8601 YYYY-MM-DD (default: today + 30 days).")),
	)
	bindUI(&ltt, "ui://polar-flow/targets.html")
	s.AddTool(ltt, withLogging("list_training_targets", ListTrainingTargetsHandler(fc)))

	s.AddTool(mcpgo.NewTool("delete_training_target",
		mcpgo.WithDescription(
			"Permanently delete a scheduled training target by its numeric id. Irreversible. "+
				"Use list_training_targets to find the id. No-ops (reports \"no target with id\") "+
				"if the id does not exist or belongs to another account.",
		),
		mcpgo.WithNumber("target_id", mcpgo.Required(),
			mcpgo.Description("Numeric target id from create_training_target or list_training_targets.")),
	), withLogging("delete_training_target", DeleteTrainingTargetHandler(fc)))

	gtt := mcpgo.NewTool("get_training_target",
		mcpgo.WithDescription(
			"Return the full, server-normalized JSON of a single training target by id — "+
				"name, datetime, and the per-sport exercise targets with their rolled-up "+
				"phases. Always read with this before update_training_target so you can "+
				"preserve the existing structure (especially the exerciseTargets ids). "+
				"Note: on read-back the server fills phase durations (a DISTANCE phase shows "+
				"duration \"00:00:00\") and rolls each exerciseTarget's duration up from its phases. "+
				"description reads back as null when the target was created without one.",
		),
		mcpgo.WithNumber("target_id", mcpgo.Required(),
			mcpgo.Description("Numeric target id from list_training_targets.")),
	)
	bindUI(&gtt, "ui://polar-flow/targets.html")
	s.AddTool(gtt, withLogging("get_training_target", GetTrainingTargetHandler(fc)))

	s.AddTool(mcpgo.NewTool("update_training_target",
		mcpgo.WithDescription(
			"Full-replace edit of an existing training target. The whole target is overwritten "+
				"by the supplied fields, so always call get_training_target first, modify the "+
				"result, then send the complete body — anything omitted is cleared.\n\n"+
				"Same field semantics, units, and phases shape as create_training_target "+
				"(distances in metres, durations in seconds, intensity as HR zones 1–5). "+
				"You do not need to manage server-side ids: the tool reads the live target and "+
				"carries its exercise-target id over for you, so the edit lands on the existing "+
				"target rather than colliding with it. Succeeds with a confirmation; re-read "+
				"with get_training_target to confirm the change landed.",
		),
		mcpgo.WithNumber("target_id", mcpgo.Required(),
			mcpgo.Description("Numeric id of the target to update (from list_training_targets).")),
		mcpgo.WithString("name", mcpgo.Required(),
			mcpgo.Description("Display name shown in the diary.")),
		mcpgo.WithString("date", mcpgo.Required(),
			mcpgo.Description("Scheduled local date, ISO 8601 YYYY-MM-DD.")),
		mcpgo.WithString("time", mcpgo.DefaultString("18:00"),
			mcpgo.Description("Scheduled local start time, 24h HH:MM (default: 18:00).")),
		mcpgo.WithNumber("sport_id", mcpgo.DefaultNumber(1),
			mcpgo.Description("Polar sport id (default 1 = running). Common ids: 2=cycling, "+
				"23=swimming, 15=strength_training, 11=hiking, 68=triathlon. "+
				"Call list_sports for the full id→name catalogue.")),
		mcpgo.WithString("description",
			mcpgo.Description("Free-text notes for the workout (optional).")),
		mcpgo.WithNumber("duration_s",
			mcpgo.Description("VOLUME target total duration in seconds. Required when replacing with a VOLUME target (no phases) and distance_m is not set.")),
		mcpgo.WithNumber("distance_m",
			mcpgo.Description("VOLUME target total distance in metres. Required when replacing with a VOLUME target (no phases) and duration_s is not set.")),
		mcpgo.WithArray("phases",
			mcpgo.Description("Ordered workout phases — identical shape to create_training_target.phases "+
				"(warmup / repeat / cooldown; distances in metres, durations in seconds). "+
				"Omit (and set duration_s or distance_m) to replace with a VOLUME target."),
			mcpgo.Items(phaseItemSchema()),
		),
	), withLogging("update_training_target", UpdateTrainingTargetHandler(fc)))

	cws := mcpgo.NewTool("get_calendar_week_summary",
		mcpgo.WithDescription(
			"Return one summary entry per ISO week intersecting [from, to] — the right-hand "+
				"\"week totals\" strip in the Polar Flow diary. Use for weekly volume trends "+
				"rather than per-session detail. The range must be ≤ 45 days (wider is rejected "+
				"by the server); from_date must be on or before to_date. "+
				"Default range: today - 28 days through today.",
		),
		mcpgo.WithString("from_date",
			mcpgo.Description("Start date, ISO 8601 YYYY-MM-DD (default: today - 28 days).")),
		mcpgo.WithString("to_date",
			mcpgo.Description("End date, ISO 8601 YYYY-MM-DD (default: today). Must be within 45 days of from_date.")),
	)
	bindUI(&cws, "ui://polar-flow/calendar.html")
	s.AddTool(cws, withLogging("get_calendar_week_summary", GetCalendarWeekSummaryHandler(fc)))

	ps := mcpgo.NewTool("get_progress_summary",
		mcpgo.WithDescription(
			"Aggregated training totals over [from, to]: session count, total distance "+
				"(metres) and duration, time-in-HR-zone, per-sport distribution, and "+
				"training-benefit distribution. The headline tool for coaching context like "+
				"\"how is my training going this month?\". Accounts with no recorded sessions "+
				"return zeros rather than an error. Default range: today - 90 days through today.",
		),
		mcpgo.WithString("from_date",
			mcpgo.Description("Start date, ISO 8601 YYYY-MM-DD (default: today - 90 days).")),
		mcpgo.WithString("to_date",
			mcpgo.Description("End date, ISO 8601 YYYY-MM-DD (default: today).")),
		mcpgo.WithString("group", mcpgo.DefaultString("MONTH"),
			mcpgo.Description("Time-bucket granularity for the breakdown (NOT a sport filter): "+
				"\"MONTH\" (default; the only value verified live), likely also \"DAY\", \"WEEK\", "+
				"\"YEAR\". Affects how the per-time-slice breakdown is bucketed, not the headline totals.")),
		mcpgo.WithString("time_frame", mcpgo.DefaultString("3m"), mcpgo.Enum("6w", "3m", "1y"),
			mcpgo.Description("Bucket size for the per-time-slice breakdowns: \"6w\" (6 weeks), "+
				"\"3m\" (3 months), or \"1y\" (1 year). Default: \"3m\". Affects only how the "+
				"breakdown is grouped, not the headline totals.")),
	)
	bindUI(&ps, "ui://polar-flow/progress.html")
	s.AddTool(ps, withLogging("get_progress_summary", GetProgressSummaryHandler(fc)))

	ce := mcpgo.NewTool("get_calendar_events",
		mcpgo.WithDescription(
			"Return raw Polar Flow diary events in a date range — completed sessions, "+
				"scheduled training targets (type \"TRAININGTARGET\"), and other diary items. "+
				"Lower-level than list_training_targets / list_training_sessions; reach for "+
				"those typed tools first and use this when you need the unfiltered calendar. "+
				"Note: fields are polymorphic by event type — `start` is an ISO 8601 string "+
				"for TRAININGTARGET events but a numeric epoch (seconds) for EXERCISE events, "+
				"and `allDay` may be a boolean or a string. "+
				"Default range: today through +30 days.",
		),
		mcpgo.WithString("from_date",
			mcpgo.Description("Start date, ISO 8601 YYYY-MM-DD (default: today).")),
		mcpgo.WithString("to_date",
			mcpgo.Description("End date, ISO 8601 YYYY-MM-DD (default: today + 30 days).")),
	)
	bindUI(&ce, "ui://polar-flow/calendar.html")
	s.AddTool(ce, withLogging("get_calendar_events", GetCalendarEventsHandler(fc)))

	lts := mcpgo.NewTool("list_training_sessions",
		mcpgo.WithDescription(
			"List completed (already-performed) Polar Flow training sessions in a date range, "+
				"with each session's numeric id, sport, start time, distance, and duration. "+
				"Pass an id to get_training_session_summary or get_training_session_details "+
				"for more. For planned/future workouts use list_training_targets instead. "+
				"Default range: last 30 days.",
		),
		mcpgo.WithString("from_date",
			mcpgo.Description("Start date, ISO 8601 YYYY-MM-DD (default: today - 30 days).")),
		mcpgo.WithString("to_date",
			mcpgo.Description("End date, ISO 8601 YYYY-MM-DD (default: today).")),
	)
	bindUI(&lts, "ui://polar-flow/sessions.html")
	s.AddTool(lts, withLogging("list_training_sessions", ListTrainingSessionsHandler(fc)))

	gss := mcpgo.NewTool("get_training_session_summary",
		mcpgo.WithDescription(
			"Return the summary view of one completed training session by id: totals such as "+
				"duration, distance, average/max HR, calories, and sport. Use "+
				"get_training_session_details for lap and sample-level data.",
		),
		mcpgo.WithNumber("session_id", mcpgo.Required(),
			mcpgo.Description("Numeric session id from list_training_sessions.")),
	)
	bindUI(&gss, "ui://polar-flow/sessions.html")
	s.AddTool(gss, withLogging("get_training_session_summary", GetTrainingSessionSummaryHandler(fc)))

	s.AddTool(mcpgo.NewTool("create_training_session",
		mcpgo.WithDescription(
			"Log a manually-entered, already-completed training session (the \"Manual "+
				"training result\" form in Polar Flow).\n\n"+
				"⚠ WRITES REAL DATA — the session lands in the diary and counts toward weekly "+
				"volume, progress summaries, and Polar's training-load model. It is NOT for "+
				"synthesizing test data on a production account. Only call this when the user "+
				"explicitly asks to log a session; never to fabricate history.\n\n"+
				"This logs an actual session (past). To plan a future workout, use "+
				"create_training_target instead.\n\n"+
				"Units: duration_s in SECONDS, distance_m in METRES, speed_kmh in km/h, "+
				"hr_avg / hr_max in bpm.\n\n"+
				"Example — a 30 min, 5 km easy run at avg 142 bpm on 25 May 2026 08:48:\n"+
				"  name: \"Easy 5km\", date: \"2026-05-25\", time: \"08:48\", duration_s: 1800,\n"+
				"  distance_m: 5000, hr_avg: 142, sport_id: 1",
		),
		mcpgo.WithString("name", mcpgo.Required(),
			mcpgo.Description("Display name shown in the diary (e.g. \"Easy 5km\").")),
		mcpgo.WithString("date", mcpgo.Required(),
			mcpgo.Description("Local date the session happened, ISO 8601 YYYY-MM-DD.")),
		mcpgo.WithString("time", mcpgo.DefaultString("18:00"),
			mcpgo.Description("Local start time, 24h HH:MM (default: 18:00).")),
		mcpgo.WithNumber("duration_s", mcpgo.Required(),
			mcpgo.Description("Elapsed duration in SECONDS (e.g. 1800 = 30 min).")),
		mcpgo.WithNumber("distance_m", mcpgo.DefaultNumber(0),
			mcpgo.Description("Distance in METRES (e.g. 5000 = 5 km; default: 0).")),
		mcpgo.WithNumber("kcal", mcpgo.DefaultNumber(0),
			mcpgo.Description("Kilocalories burned (default: 0).")),
		mcpgo.WithNumber("hr_avg",
			mcpgo.Description("Average heart rate in bpm. Omit or 0 to leave unset.")),
		mcpgo.WithNumber("hr_max",
			mcpgo.Description("Max heart rate in bpm. Omit or 0 to leave unset.")),
		mcpgo.WithNumber("speed_kmh",
			mcpgo.Description("Average speed in km/h. Omit or 0 to leave unset.")),
		mcpgo.WithNumber("sport_id", mcpgo.DefaultNumber(1),
			mcpgo.Description("Polar sport id (default 1 = running). Common ids: 2=cycling, "+
				"23=swimming, 15=strength_training, 11=hiking, 68=triathlon. "+
				"Call list_sports for the full id→name catalogue.")),
		mcpgo.WithString("note",
			mcpgo.Description("Free-text note for the session (optional).")),
	), withLogging("create_training_session", CreateTrainingSessionHandler(fc)))

	gsd := mcpgo.NewTool("get_training_session_details",
		mcpgo.WithDescription(
			"Return lap-level detail for one completed training session: per-lap splits "+
				"(time, distance, average/max HR) and the HR time-in-zone distribution behind "+
				"the summary. Use it when you need splits or the zone breakdown, not just totals. "+
				"(Raw per-second sample traces are omitted — they are large and not generally useful.)",
		),
		mcpgo.WithNumber("session_id", mcpgo.Required(),
			mcpgo.Description("Numeric session id from list_training_sessions.")),
	)
	bindUI(&gsd, "ui://polar-flow/sessions.html")
	s.AddTool(gsd, withLogging("get_training_session_details", GetTrainingSessionDetailsHandler(fc)))
}

// phaseItemSchema returns the JSON-schema for one entry of the `phases` array.
// create_training_target and update_training_target share this identical shape
// (both take a full create-style body), so the schema lives in one place to
// stop the two copies from drifting.
func phaseItemSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"type": map[string]any{
				"type":        "string",
				"enum":        []string{"warmup", "repeat", "cooldown"},
				"description": "Phase kind.",
			},
			"name": map[string]any{
				"type": "string",
				"description": "Optional free-text label for this phase, persisted verbatim by Polar and " +
					"shown per-phase in the workout breakdown. For a repeat phase this names the work " +
					"interval (recovery has its own recovery.name). Defaults to a label derived from the " +
					"phase type when omitted (\"Warm-up\", \"Work\", \"Cool-down\").",
			},
			"duration_s": map[string]any{
				"type":        "integer",
				"description": "Phase length in seconds. Required for warmup/cooldown.",
			},
			"reps": map[string]any{
				"type":        "integer",
				"description": "Repeat count for a repeat phase (≥ 2).",
				"minimum":     2,
			},
			"goal": map[string]any{
				"type":        "object",
				"description": "Per-rep goal for a repeat phase. Set exactly one of distance_m or duration_s.",
				"properties": map[string]any{
					"distance_m": map[string]any{"type": "number", "description": "Goal distance in metres (e.g. 1000 = 1 km)."},
					"duration_s": map[string]any{"type": "integer", "description": "Goal duration in seconds."},
				},
			},
			"intensity": map[string]any{
				"type":        "object",
				"description": "Target intensity for the work portion of a repeat. Set hr_zone or label; hr_zone wins if both given. Omit for no zone (open intensity).",
				"properties": map[string]any{
					"label":   map[string]any{"type": "string", "enum": []string{"easy", "aerobic", "tempo", "threshold", "vo2max"}, "description": "Effort label, mapped to an HR zone."},
					"hr_zone": map[string]any{"type": "integer", "minimum": 1, "maximum": 5, "description": "Polar HR zone 1–5, used as both lower and upper bound."},
				},
			},
			"recovery": map[string]any{
				"type":        "object",
				"description": "Optional easy recovery inserted between reps of a repeat phase.",
				"properties": map[string]any{
					"duration_s": map[string]any{"type": "integer", "description": "Recovery duration in seconds."},
					"name":       map[string]any{"type": "string", "description": "Optional free-text label for the recovery phase (default \"Recovery\")."},
				},
			},
		},
		"required": []string{"type"},
	}
}
