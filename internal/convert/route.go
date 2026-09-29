package convert

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

// Route import: Flow's web UI parses GPX/TCX files in the browser and uploads
// JSON trackpoints to POST /api/favorites/trainingTargets/importRoute. This file
// reproduces that parsing (captured 2026-09-28, see
// polar-openapi-maker/docs/endpoints/routes.md) and adds the validation Flow
// skips: the server silently truncates long names, drops out-of-range points,
// and accepts a one-point route, so all of that is rejected here instead.

// Route limits enforced before upload.
const (
	// MaxRouteFileBytes is the web UI's documented upload limit (25 MB).
	MaxRouteFileBytes = 25 << 20
	// MaxRouteNameRunes is the length Flow truncates route names to.
	MaxRouteNameRunes = 45
	// earthRadiusM is the radius the web UI's haversine uses (the captured
	// cumulative distances match R = 6 371 000 m to 1e-10 m).
	earthRadiusM = 6371000.0
)

// RouteFormat names a supported route file format.
type RouteFormat string

// Supported route formats.
const (
	RouteFormatGPX RouteFormat = "gpx"
	RouteFormatTCX RouteFormat = "tcx"
)

// RoutePoint is one parsed trackpoint in canonical units.
type RoutePoint struct {
	Lat       float64  // WGS84 degrees
	Lon       float64  // WGS84 degrees
	AltitudeM *float64 // metres; nil when the source has no elevation
	DistanceM float64  // cumulative metres from the first point
	Time      string   // TCX Activity points only (ISO 8601), else ""
}

// ParsedRoute is the result of parsing a GPX/TCX file.
type ParsedRoute struct {
	Format    RouteFormat
	Name      string // name found in the file ("" when none)
	Points    []RoutePoint
	DistanceM float64 // total length in metres (last point's DistanceM)
}

// ParseRoute parses GPX or TCX content. format may be "", "auto", "gpx" or
// "tcx"; auto-detection looks at the XML root element. The returned route has
// at least two points, all coordinates in range, and a positive length.
func ParseRoute(content []byte, format string) (*ParsedRoute, error) {
	if len(bytes.TrimSpace(content)) == 0 {
		return nil, errors.New("route file content is empty")
	}
	if len(content) > MaxRouteFileBytes {
		return nil, fmt.Errorf("route file is %d bytes; Polar's limit is 25 MB", len(content))
	}
	root, err := xmlRoot(content)
	if err != nil {
		return nil, fmt.Errorf("route file is not valid XML (expected GPX or TCX): %w", err)
	}
	detected := RouteFormat("")
	switch strings.ToLower(root) {
	case "gpx":
		detected = RouteFormatGPX
	case "trainingcenterdatabase":
		detected = RouteFormatTCX
	}
	want := RouteFormat(strings.ToLower(strings.TrimSpace(format)))
	switch want {
	case "", "auto":
		if detected == "" {
			return nil, fmt.Errorf("unrecognised route file: root element <%s> is neither <gpx> nor <TrainingCenterDatabase>", root)
		}
	case RouteFormatGPX, RouteFormatTCX:
		if detected != want {
			return nil, fmt.Errorf("format %q was requested but the file's root element is <%s>", want, root)
		}
	default:
		return nil, fmt.Errorf("format must be \"auto\", \"gpx\" or \"tcx\", got %q", format)
	}

	var r *ParsedRoute
	if detected == RouteFormatGPX {
		r, err = parseGPX(content)
	} else {
		r, err = parseTCX(content)
	}
	if err != nil {
		return nil, err
	}
	if err := validateRoutePoints(r.Points); err != nil {
		return nil, err
	}
	r.DistanceM = r.Points[len(r.Points)-1].DistanceM
	if !(r.DistanceM > 0) {
		return nil, errors.New("route has zero length (all points are at the same position); Polar rejects it")
	}
	return r, nil
}

// xmlRoot returns the local name of the document's root element.
func xmlRoot(content []byte) (string, error) {
	dec := xml.NewDecoder(bytes.NewReader(content))
	for {
		tok, err := dec.Token()
		if err != nil {
			return "", err
		}
		if se, ok := tok.(xml.StartElement); ok {
			return se.Name.Local, nil
		}
	}
}

type gpxPoint struct {
	Lat string   `xml:"lat,attr"`
	Lon string   `xml:"lon,attr"`
	Ele *float64 `xml:"ele"`
}

type gpxDoc struct {
	Metadata struct {
		Name string `xml:"name"`
	} `xml:"metadata"`
	Trk []struct {
		Name   string `xml:"name"`
		Trkseg []struct {
			Trkpt []gpxPoint `xml:"trkpt"`
		} `xml:"trkseg"`
	} `xml:"trk"`
	Rte []struct {
		Name  string     `xml:"name"`
		Rtept []gpxPoint `xml:"rtept"`
	} `xml:"rte"`
}

// parseGPX mirrors the web UI: the first <trk> (else the first <rte>), name
// from that track/route (else <metadata><name>), missing <ele> → altitude 0,
// cumulative distance by haversine. One deliberate difference: the UI keeps
// only the first <trkseg> and silently drops the rest; here every segment of
// the first track is used, so a paused recording is not truncated.
func parseGPX(content []byte) (*ParsedRoute, error) {
	var doc gpxDoc
	if err := xml.Unmarshal(content, &doc); err != nil {
		return nil, fmt.Errorf("invalid GPX: %w", err)
	}
	var raw []gpxPoint
	name := ""
	switch {
	case len(doc.Trk) > 0:
		name = doc.Trk[0].Name
		for _, seg := range doc.Trk[0].Trkseg {
			raw = append(raw, seg.Trkpt...)
		}
	case len(doc.Rte) > 0:
		name = doc.Rte[0].Name
		raw = doc.Rte[0].Rtept
	default:
		return nil, errors.New("GPX file has no <trk> or <rte>: nothing to import")
	}
	if strings.TrimSpace(name) == "" {
		name = doc.Metadata.Name
	}
	pts := make([]RoutePoint, 0, len(raw))
	for i, p := range raw {
		lat, lon, err := parseLatLon(p.Lat, p.Lon)
		if err != nil {
			return nil, fmt.Errorf("GPX point %d: %w", i, err)
		}
		alt := 0.0 // the web UI sends 0 for a GPX point without <ele>
		if p.Ele != nil {
			alt = *p.Ele
		}
		pts = append(pts, RoutePoint{Lat: lat, Lon: lon, AltitudeM: &alt})
	}
	fillHaversine(pts)
	return &ParsedRoute{Format: RouteFormatGPX, Name: strings.TrimSpace(name), Points: pts}, nil
}

type tcxTrackpoint struct {
	Time     string `xml:"Time"`
	Position *struct {
		Lat float64 `xml:"LatitudeDegrees"`
		Lon float64 `xml:"LongitudeDegrees"`
	} `xml:"Position"`
	AltitudeMeters *float64 `xml:"AltitudeMeters"`
	DistanceMeters *float64 `xml:"DistanceMeters"`
}

type tcxDoc struct {
	Courses struct {
		Course []struct {
			Name  string `xml:"Name"`
			Track []struct {
				Trackpoint []tcxTrackpoint `xml:"Trackpoint"`
			} `xml:"Track"`
		} `xml:"Course"`
	} `xml:"Courses"`
	Activities struct {
		Activity []struct {
			Lap []struct {
				Track []struct {
					Trackpoint []tcxTrackpoint `xml:"Trackpoint"`
				} `xml:"Track"`
			} `xml:"Lap"`
		} `xml:"Activity"`
	} `xml:"Activities"`
}

// parseTCX mirrors the web UI: the first <Course> (else the first
// <Activity>, all laps), only trackpoints that carry a <Position>, the file's
// <DistanceMeters> copied verbatim (haversine only when the file has none),
// missing altitude → nil, and each Activity point's <Time> passed through.
func parseTCX(content []byte) (*ParsedRoute, error) {
	var doc tcxDoc
	if err := xml.Unmarshal(content, &doc); err != nil {
		return nil, fmt.Errorf("invalid TCX: %w", err)
	}
	var raw []tcxTrackpoint
	name := ""
	isActivity := false
	switch {
	case len(doc.Courses.Course) > 0:
		c := doc.Courses.Course[0]
		name = c.Name
		for _, t := range c.Track {
			raw = append(raw, t.Trackpoint...)
		}
	case len(doc.Activities.Activity) > 0:
		isActivity = true
		for _, lap := range doc.Activities.Activity[0].Lap {
			for _, t := range lap.Track {
				raw = append(raw, t.Trackpoint...)
			}
		}
	default:
		return nil, errors.New("TCX file has no <Course> or <Activity>: nothing to import")
	}
	pts := make([]RoutePoint, 0, len(raw))
	haveDistances := true
	for _, p := range raw {
		if p.Position == nil {
			continue // HR-only samples: the UI skips them too
		}
		rp := RoutePoint{Lat: p.Position.Lat, Lon: p.Position.Lon, AltitudeM: p.AltitudeMeters}
		if p.DistanceMeters != nil {
			rp.DistanceM = *p.DistanceMeters
		} else {
			haveDistances = false
		}
		if isActivity && p.Time != "" {
			rp.Time = normalizeTCXTime(p.Time)
		}
		pts = append(pts, rp)
	}
	if !haveDistances {
		fillHaversine(pts)
	}
	for i := 1; i < len(pts); i++ {
		if pts[i].DistanceM < pts[i-1].DistanceM {
			return nil, fmt.Errorf("TCX point %d: DistanceMeters %.1f decreases from the previous point (%.1f)",
				i, pts[i].DistanceM, pts[i-1].DistanceM)
		}
	}
	return &ParsedRoute{Format: RouteFormatTCX, Name: strings.TrimSpace(name), Points: pts}, nil
}

// normalizeTCXTime renders a TCX timestamp the way the web UI sends it
// (millisecond UTC, "2026-01-01T10:00:00.000Z"); unparseable values pass through.
func normalizeTCXTime(s string) string {
	t, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(s))
	if err != nil {
		return s
	}
	return t.UTC().Format("2006-01-02T15:04:05.000Z")
}

func parseLatLon(latS, lonS string) (float64, float64, error) {
	var lat, lon float64
	if _, err := fmt.Sscan(strings.TrimSpace(latS), &lat); err != nil {
		return 0, 0, fmt.Errorf("lat %q is not a number", latS)
	}
	if _, err := fmt.Sscan(strings.TrimSpace(lonS), &lon); err != nil {
		return 0, 0, fmt.Errorf("lon %q is not a number", lonS)
	}
	return lat, lon, nil
}

// validateRoutePoints rejects what Flow would silently mangle: fewer than two
// points (Flow accepts a one-point route), out-of-range or non-finite
// coordinates (Flow silently drops those points).
func validateRoutePoints(pts []RoutePoint) error {
	if len(pts) < 2 {
		return fmt.Errorf("route has %d point(s) with a position; at least 2 are needed", len(pts))
	}
	for i, p := range pts {
		if math.IsNaN(p.Lat) || math.IsInf(p.Lat, 0) || p.Lat < -90 || p.Lat > 90 {
			return fmt.Errorf("point %d: latitude %v is outside [-90, 90]", i, p.Lat)
		}
		if math.IsNaN(p.Lon) || math.IsInf(p.Lon, 0) || p.Lon < -180 || p.Lon > 180 {
			return fmt.Errorf("point %d: longitude %v is outside [-180, 180]", i, p.Lon)
		}
		if p.AltitudeM != nil && (math.IsNaN(*p.AltitudeM) || math.IsInf(*p.AltitudeM, 0)) {
			return fmt.Errorf("point %d: altitude is not a finite number", i)
		}
	}
	return nil
}

// fillHaversine sets each point's cumulative distance from the first point.
func fillHaversine(pts []RoutePoint) {
	cum := 0.0
	for i := range pts {
		if i > 0 {
			cum += HaversineM(pts[i-1].Lat, pts[i-1].Lon, pts[i].Lat, pts[i].Lon)
		}
		pts[i].DistanceM = cum
	}
}

// HaversineM is the great-circle distance in metres between two WGS84 points.
// It uses the exact formulation of Flow's web UI (atan2 form, deltas taken in
// degrees before conversion, R = 6 371 000 m) so uploaded cumulative distances
// match the browser's bit for bit.
func HaversineM(lat1, lon1, lat2, lon2 float64) float64 {
	toRad := func(d float64) float64 { return d * math.Pi / 180 }
	dLat := toRad(lat2 - lat1)
	dLon := toRad(lon2 - lon1)
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(toRad(lat1))*math.Cos(toRad(lat2))*math.Sin(dLon/2)*math.Sin(dLon/2)
	return earthRadiusM * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

// ValidateRouteName checks a route name against Flow's rules: at least one
// non-whitespace character, at most 45 characters (Flow silently truncates
// longer names, so they are rejected here instead).
func ValidateRouteName(name string) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("route name must contain a non-whitespace character")
	}
	if n := PolarTextLen(name); n > MaxRouteNameRunes {
		return fmt.Errorf("route name is %d characters; Polar's limit is %d", n, MaxRouteNameRunes)
	}
	return nil
}
