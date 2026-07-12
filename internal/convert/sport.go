package convert

import "strings"

// Sport categorisation. This is the single source of truth for mapping any of
// Polar's ~150 sports to one of ~20 UI categories; the MCP-app UIs own only the
// presentation for each category (SVG glyph, colour, label). Classification is
// keyword-first on the (possibly localised) sport name — resilient to the sports
// Polar keeps adding, since names are compositional ("trail running", "indoor
// cycling", "pool swimming") — with the two most stable sport ids as a fallback
// when the name is absent.

// sportKeywords pairs each category with the lowercase substrings that select
// it. Order matters: more specific categories precede broader ones so that, for
// example, "motorcycling" resolves to motor rather than cycle. English sport
// constants and French display names are both covered.
var sportKeywords = []struct {
	cat string
	kw  []string
}{
	{"run", []string{"run", "jog", "course", "footing", "treadmill", "tapis", "orient"}},
	{"walk", []string{"walk", "nordic", "marche"}},
	{"hike", []string{"hik", "trek", "trail", "mountaineer", "rando"}},
	{"motor", []string{"motor", "karting", "moto"}},
	{"cycle", []string{"cycl", "bike", "biking", "mtb", "velo", "vélo", "spinning", "handbike", "ride"}},
	{"swim", []string{"swim", "pool", "open water", "natation"}},
	{"row", []string{"row", "canoe", "kayak", "paddl", "stand up", "aviron", "rame"}},
	{"strength", []string{"strength", "gym", "functional", "fonctionnel", "crossfit", "weight", "muscu", "renfor", "core", "circuit"}},
	{"yoga", []string{"yoga", "pilates", "stretch", "mobility", "mobilit", "meditat", "breathing"}},
	{"ski", []string{"ski", "biathlon"}},
	{"snow", []string{"snowboard", "sled", "luge", "snowshoe", "raquette"}},
	{"skate", []string{"skat", "inline", "roller", "patin"}},
	{"racquet", []string{"tennis", "badminton", "squash", "table tennis", "ping", "padel", "racquet", "raquette"}},
	{"ball", []string{"soccer", "football", "basket", "volley", "handball", "rugby", "hockey", "baseball", "cricket", "netball", "frisbee", "disc"}},
	{"golf", []string{"golf"}},
	{"combat", []string{"box", "martial", "kickbox", "judo", "karate", "wrestl", "mma", "fight", "combat"}},
	{"climb", []string{"climb", "boulder", "escalade", "grimpe"}},
	{"dance", []string{"danc", "zumba", "aerobic", "danse"}},
	{"water", []string{"sail", "windsurf", "surf", "kite", "div", "snorkel", "voile", "plong"}},
	{"horse", []string{"horse", "equestr", "riding", "cheval", "équit", "equit"}},
}

// sportIDCategory maps the two most stable Polar sport ids (1 = RUNNING,
// 2 = CYCLING) to a category. Used only as a fallback when the name yields no
// match — ids are a moving snapshot, so we keep the table intentionally tiny.
var sportIDCategory = map[int]string{1: "run", 2: "cycle"}

// SportCategory classifies a Polar sport into one of the UI category keys
// (run, walk, hike, motor, cycle, swim, row, strength, yoga, ski, snow, skate,
// racquet, ball, golf, combat, climb, dance, water, horse) or "generic" when
// nothing matches. name may be a sport constant ("TRAIL_RUNNING") or a localised
// display name ("Course à pied"); id is the Polar sport id (0 when unknown).
func SportCategory(name string, id int) string {
	n := strings.ToLower(strings.TrimSpace(name))
	if n != "" {
		for _, e := range sportKeywords {
			for _, k := range e.kw {
				if strings.Contains(n, k) {
					return e.cat
				}
			}
		}
	}
	if c, ok := sportIDCategory[id]; ok {
		return c
	}
	return "generic"
}
