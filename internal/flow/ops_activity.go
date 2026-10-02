package flow

import (
	"context"
	"fmt"
	"time"

	"github.com/lmgarret/polar-flow-mcp/internal/flow/gen"
)

// ActivityTimelineFour reads the 24/7 activity of the four days
// [day-2, day+1] via GET /api/activity-timeline/loadFour, keyed by
// YYYY-MM-DD. maxSamples caps each intraday sample array (Flow downsamples).
// Days past the last synced data come back with a null dataPanelData.
func (c *Client) ActivityTimelineFour(ctx context.Context, day time.Time, maxSamples int) (gen.ActivityTimelineByDate, error) {
	res, err := c.API.GetActivityTimelineFour(ctx, gen.GetActivityTimelineFourParams{
		Day:            day,
		MaxSampleCount: gen.NewOptInt(maxSamples),
	})
	if err != nil {
		return nil, wrapFlowError("activity timeline", err)
	}
	switch v := res.(type) {
	case *gen.ActivityTimelineByDate:
		return *v, nil
	case *gen.GetActivityTimelineFourBadRequest:
		return nil, fmt.Errorf("flow: activity timeline: day %s rejected", day.Format(time.DateOnly))
	case *gen.Unauthorized:
		return nil, ErrLoginFailed
	default:
		return nil, fmt.Errorf("flow: activity timeline: unexpected response %T", res)
	}
}

// Sleep report window limits: Flow only answers ranges of 30–365 days.
const (
	SleepMinRangeDays = 30
	SleepMaxRangeDays = 365
)

// SleepNights returns the recorded nights whose wake-up date lies in
// [from, to], via GET https://sleep-api.flow.polar.com/api/sleep/report. That
// endpoint refuses windows shorter than 30 days, so a narrower request is
// widened backwards to 30 days and filtered here. to - from must be ≤ 365 days.
func (c *Client) SleepNights(ctx context.Context, from, to time.Time) ([]gen.SleepNight, error) {
	if to.Before(from) {
		return nil, fmt.Errorf("flow: sleep report: from is after to")
	}
	if to.Sub(from) > SleepMaxRangeDays*24*time.Hour {
		return nil, fmt.Errorf("flow: sleep report: range exceeds %d days", SleepMaxRangeDays)
	}
	wireFrom := from
	if min := to.AddDate(0, 0, -SleepMinRangeDays); wireFrom.After(min) {
		wireFrom = min
	}
	params := gen.GetSleepReportParams{From: wireFrom, To: to, XRequestedWith: gen.XRequestedWithXMLHttpRequest}
	sctx := gen.WithServerURL(ctx, c.sleepURL)
	res, err := c.API.GetSleepReport(sctx, params)
	if _, unauth := res.(*gen.GetSleepReportUnauthorized); err == nil && unauth && c.cfg.Email != "" {
		// The sleep API's 401 body is not Flow's {"error":"NotAuthenticated"},
		// so the transport does not refresh on it: an expired FLOW_SESSION
		// JWT surfaces here. Refresh once and retry.
		if rerr := c.refresh(ctx); rerr == nil {
			res, err = c.API.GetSleepReport(sctx, params)
		}
	}
	if err != nil {
		return nil, wrapFlowError("sleep report", err)
	}
	switch v := res.(type) {
	case *gen.GetSleepReportOKApplicationJSON:
		var out []gen.SleepNight
		for _, n := range *v {
			if n.Date.Before(from) || n.Date.After(to) {
				continue
			}
			out = append(out, n)
		}
		return out, nil
	case *gen.GetSleepReportBadRequest:
		return nil, fmt.Errorf("flow: sleep report: range %s…%s rejected",
			wireFrom.Format(time.DateOnly), to.Format(time.DateOnly))
	case *gen.GetSleepReportUnauthorized:
		return nil, ErrLoginFailed
	default:
		return nil, fmt.Errorf("flow: sleep report: unexpected response %T", res)
	}
}
