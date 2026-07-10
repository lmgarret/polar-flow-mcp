package convert

import "testing"

func TestSportCategory(t *testing.T) {
	cases := []struct {
		name string
		id   int
		want string
	}{
		// running family
		{"RUNNING", 1, "run"},
		{"TRAIL_RUNNING", 0, "run"}, // "run" is matched before the "trail" hike root
		{"TREADMILL_RUNNING", 0, "run"},
		{"Course à pied", 0, "run"},
		{"ORIENTEERING", 0, "run"},
		// walk / hike
		{"WALKING", 0, "walk"},
		{"NORDIC_WALKING", 0, "walk"},
		{"HIKING", 0, "hike"},
		{"Randonnée", 0, "hike"},
		// wheels — motor must win over cycle
		{"CYCLING", 2, "cycle"},
		{"MOUNTAIN_BIKING", 0, "cycle"},
		{"INDOOR_CYCLING", 0, "cycle"},
		{"MOTORCYCLING", 0, "motor"},
		{"Vélo", 0, "cycle"},
		// water
		{"POOL_SWIMMING", 0, "swim"},
		{"Natation", 0, "swim"},
		{"ROWING", 0, "row"},
		{"KAYAKING", 0, "row"},
		{"STAND_UP_PADDLING", 0, "row"},
		{"SAILING", 0, "water"},
		{"SCUBA_DIVING", 0, "water"},
		// gym / mind-body
		{"STRENGTH_TRAINING", 0, "strength"},
		{"CROSSFIT", 0, "strength"},
		{"YOGA", 0, "yoga"},
		{"PILATES", 0, "yoga"},
		// snow
		{"CROSS-COUNTRY_SKIING", 0, "ski"},
		{"SNOWBOARDING", 0, "snow"},
		{"ICE_SKATING", 0, "skate"},
		// ball / racquet / other
		{"TENNIS", 0, "racquet"},
		{"SOCCER", 0, "ball"},
		{"GOLF", 0, "golf"},
		{"BOXING", 0, "combat"},
		{"CLIMBING", 0, "climb"},
		{"DANCING", 0, "dance"},
		{"HORSEBACK_RIDING", 0, "horse"},
		// fallbacks
		{"", 1, "run"},          // id fallback when name absent
		{"", 2, "cycle"},        // id fallback
		{"", 0, "generic"},      // nothing to go on
		{"PARKOUR", 0, "generic"}, // no recognisable root
	}
	for _, c := range cases {
		if got := SportCategory(c.name, c.id); got != c.want {
			t.Errorf("SportCategory(%q, %d) = %q, want %q", c.name, c.id, got, c.want)
		}
	}
}
