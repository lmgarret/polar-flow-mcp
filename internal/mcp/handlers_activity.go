package mcp

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	mcpgo "github.com/mark3labs/mcp-go/mcp"

	"github.com/lmgarret/polar-flow-mcp/internal/convert"
	"github.com/lmgarret/polar-flow-mcp/internal/flow"
)

// Ranges the read tools accept. Activity is fetched four days per request, so
// its cap keeps a call to 8 requests; sleep is one request of ≤ 365 days.
const (
	maxActivityDays = 31
	// Intraday samples per series: Flow downsamples to this many points —
	// 10-minute resolution over a day. Sent on every call, even when the
	// samples are dropped (multi-day ranges): on a real account, calls with
	// maxSampleCount 1 failed with a 500 "Failed to load activity timeline
	// data" while single-day calls with 144 worked (2026-10-01). The web UI
	// always sends 200.
	activitySampleCount = 144
)

// dateRangeArgs reads from_date / to_date (defaults: the last defDays days
// through today) and checks order and span.
func dateRangeArgs(req mcpgo.CallToolRequest, defDays, maxDays int) (time.Time, time.Time, error) {
	today := time.Now()
	today = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)
	from, to, err := parseFromTo(req, today.AddDate(0, 0, -(defDays-1)), today)
	if err != nil {
		return from, to, err
	}
	if to.Before(from) {
		return from, to, fmt.Errorf("from_date %s is after to_date %s", from.Format(isoDate), to.Format(isoDate))
	}
	if days := int(to.Sub(from).Hours()/24) + 1; days > maxDays {
		return from, to, fmt.Errorf("the range is %d days; at most %d are allowed", days, maxDays)
	}
	return from, to, nil
}

// GetDailyActivityHandler returns 24/7 activity per day over a date range.
func GetDailyActivityHandler(fc *flow.Client) toolHandler {
	return func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		from, to, err := dateRangeArgs(req, 7, maxActivityDays)
		if err != nil {
			return mcpgo.NewToolResultError(err.Error()), nil
		}
		single := from.Equal(to)
		// loadFour(D) answers [D-2, D+1]: step D by 4 from from+2 until the
		// window reaches to.
		days := map[string]convert.DailyActivity{}
		for d := from.AddDate(0, 0, 2); !d.AddDate(0, 0, -2).After(to); d = d.AddDate(0, 0, 4) {
			byDate, err := fc.ActivityTimelineFour(ctx, d, activitySampleCount)
			if err != nil {
				return mcpgo.NewToolResultError(err.Error()), nil
			}
			for key, day := range byDate {
				date, perr := time.Parse(isoDate, key)
				if perr != nil || date.Before(from) || date.After(to) {
					continue
				}
				days[key] = convert.FromWireActivityDay(date, day, single)
			}
		}
		out := make([]convert.DailyActivity, 0, len(days))
		for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
			key := d.Format(isoDate)
			day, ok := days[key]
			if !ok {
				day = convert.DailyActivity{Date: key}
			}
			out = append(out, day)
		}
		summary := convert.SummarizeActivity(out)
		payload := map[string]any{
			"type":      "daily_activity",
			"from_date": from.Format(isoDate),
			"to_date":   to.Format(isoDate),
			"days":      out,
			"summary":   summary,
		}
		return widgetResultText(activityText(out, summary), payload), nil
	}
}

func activityText(days []convert.DailyActivity, s convert.ActivitySummary) string {
	if s.DaysWithData == 0 {
		return fmt.Sprintf("No activity data between %s and %s. 24/7 activity comes from a Polar "+
			"device synced to Flow; this account has none for these days.", days[0].Date, days[len(days)-1].Date)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Activity %s – %s: %d of %d days with data, %d steps in total.\n",
		days[0].Date, days[len(days)-1].Date, s.DaysWithData, len(days), s.TotalSteps)
	for _, d := range days {
		if !d.HasData {
			fmt.Fprintf(&b, "%s: no data\n", d.Date)
			continue
		}
		fmt.Fprintf(&b, "%s: %d steps, active %s, %d kcal", d.Date, deref(d.Steps),
			convert.HumanDuration(deref(d.ActiveTimeS)), deref(d.KCal))
		if d.GoalPct != nil {
			fmt.Fprintf(&b, ", goal %.0f%%", *d.GoalPct)
		}
		if d.SleepS != nil && *d.SleepS > 0 {
			fmt.Fprintf(&b, ", slept %s", convert.HumanDuration(*d.SleepS))
		}
		if hr := d.HeartRate; hr != nil && hr.NightMinBPM != nil {
			fmt.Fprintf(&b, ", night low HR %d bpm", *hr.NightMinBPM)
		}
		b.WriteString("\n")
	}
	return b.String()
}

// GetSleepHandler returns recorded nights over a date range.
func GetSleepHandler(fc *flow.Client) toolHandler {
	return func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		from, to, err := dateRangeArgs(req, 14, flow.SleepMaxRangeDays)
		if err != nil {
			return mcpgo.NewToolResultError(err.Error()), nil
		}
		wire, err := fc.SleepNights(ctx, from, to)
		if err != nil {
			return mcpgo.NewToolResultError(err.Error()), nil
		}
		nights := make([]convert.SleepNight, 0, len(wire))
		for _, n := range wire {
			if night, ok := convert.FromWireSleepNight(n); ok {
				nights = append(nights, night)
			}
		}
		sort.Slice(nights, func(i, j int) bool { return nights[i].Date < nights[j].Date })
		avg := convert.AverageSleep(nights)
		payload := map[string]any{
			"type":      "sleep_report",
			"from_date": from.Format(isoDate),
			"to_date":   to.Format(isoDate),
			"nights":    nights,
			"averages":  avg,
		}
		return widgetResultText(sleepText(from, to, nights, avg), payload), nil
	}
}

func sleepText(from, to time.Time, nights []convert.SleepNight, avg convert.SleepAverages) string {
	if len(nights) == 0 {
		return fmt.Sprintf("No sleep recorded between %s and %s. Sleep tracking needs a Polar device "+
			"with sleep tracking, synced to Flow.", from.Format(isoDate), to.Format(isoDate))
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Sleep %s – %s: %d nights, average %s asleep", from.Format(isoDate), to.Format(isoDate),
		avg.Nights, convert.HumanDuration(deref(avg.SleepS)))
	if avg.Score != nil {
		fmt.Fprintf(&b, ", score %.0f", *avg.Score)
	}
	b.WriteString(".\n")
	for _, n := range nights {
		fmt.Fprintf(&b, "%s: %s → %s, %s asleep", n.Date, n.FellAsleep[11:16], n.WokeUp[11:16], convert.HumanDuration(n.SleepS))
		if n.Score != nil {
			fmt.Fprintf(&b, ", score %.0f", *n.Score)
		}
		if st := n.Stages; st != nil {
			fmt.Fprintf(&b, " (light %s, deep %s, REM %s)", convert.HumanDuration(st.LightS),
				convert.HumanDuration(st.DeepS), convert.HumanDuration(st.REMS))
		}
		b.WriteString("\n")
	}
	return b.String()
}

func deref(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}
