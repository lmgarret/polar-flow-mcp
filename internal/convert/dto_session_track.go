package convert

import (
	"math"
	"time"

	"github.com/lmgarret/polar-flow-mcp/internal/flow/gen"
)

// TrackPoint is one GPS fix of a recorded session: its offset from the start
// and its WGS84 position.
type TrackPoint struct {
	TS        int      `json:"t_s"` // seconds from Start
	Lat       float64  `json:"lat"`
	Lon       float64  `json:"lon"`
	AltitudeM *float64 `json:"altitude_m"`
}

// SessionTrack is the get_session_track payload: the GPS track of one exercise
// (COORDINATE in the details response), evenly down-sampled like get_route.
// PointCount is always the full count, Points holds what was returned.
type SessionTrack struct {
	SessionID      int64        `json:"session_id"`
	ExerciseID     int64        `json:"exercise_id"`
	Start          string       `json:"start,omitempty"`
	DurationS      *int         `json:"duration_s"`
	PointCount     int          `json:"point_count"`
	ReturnedPoints int          `json:"returned_points"`
	DistanceM      float64      `json:"distance_m"`
	Points         []TrackPoint `json:"points"`
}

// FromWireSessionTrack maps one exercise's GPS fixes to SessionTrack. start
// dates the track (the exercise start, so t_s counts from it); zero for none.
// The cumulative distance is the haversine sum of consecutive fixes, over the
// full track even when points are thinned. ok=false when the exercise recorded
// no GPS. maxPoints <= 0 means all.
func FromWireSessionTrack(b gen.SampleBlock, sessionID, exerciseID int64, start time.Time, durationS *int, maxPoints int) (SessionTrack, bool) {
	out := SessionTrack{SessionID: sessionID, ExerciseID: exerciseID, DurationS: durationS, Points: []TrackPoint{}}
	fixes, ok := b.COORDINATE.Get()
	if !ok || len(fixes.Samples) == 0 {
		return out, false
	}
	if !start.IsZero() {
		out.Start = start.Format("2006-01-02T15:04:05")
	}
	all := make([]TrackPoint, len(fixes.Samples))
	for i, f := range fixes.Samples {
		if !validLatLon(f.Latitude, f.Longitude) {
			return SessionTrack{Points: []TrackPoint{}}, false
		}
		p := TrackPoint{TS: fixOffsetS(f.Time), Lat: f.Latitude, Lon: f.Longitude}
		if v, has := f.Altitude.Get(); has {
			p.AltitudeM = &v
		}
		all[i] = p
		if i > 0 {
			out.DistanceM += HaversineM(all[i-1].Lat, all[i-1].Lon, p.Lat, p.Lon)
		}
	}
	out.PointCount = len(all)
	for _, i := range sampleIndexes(out.PointCount, maxPoints) {
		out.Points = append(out.Points, all[i])
	}
	out.ReturnedPoints = len(out.Points)
	return out, true
}

// fixOffsetS rounds a fix's ISO 8601 offset to whole seconds.
func fixOffsetS(t string) int {
	secs, ok := ISO8601DurationToSecondsF(t)
	if !ok || math.IsNaN(secs) {
		return 0
	}
	return int(math.Round(secs))
}

func validLatLon(lat, lon float64) bool {
	return !math.IsNaN(lat) && !math.IsNaN(lon) && lat >= -90 && lat <= 90 && lon >= -180 && lon <= 180
}
