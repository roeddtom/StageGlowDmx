package engine

import (
	"reflect"
	"testing"
	"time"

	"stageglowdmx/internal/config"
)

func layeredRig() config.Config {
	c := testRig()
	c.Moods[0].Groups = c.Moods[0].Groups[1:]
	base := config.Mood{ID: "band", Name: "Band", Type: "base", BeatsPerChange: 8, Smoothness: 1,
		Groups: []config.MoodGroup{{GroupID: "front", ColorMode: "static", Palette: []string{"#ff0000"}, IntensityMin: 0.5, IntensityMax: 0.5}}}
	speaker := base
	speaker.ID, speaker.Name = "speaker", "Speaker"
	speaker.Groups = []config.MoodGroup{{GroupID: "front", ColorMode: "static", Palette: []string{"#00ff00"}, IntensityMin: 0.25, IntensityMax: 0.25}}
	c.Moods = append(c.Moods, base, speaker)
	return c
}

func TestBaseFallbackAndIndependentAtmosphereClock(t *testing.T) {
	c := layeredRig()
	e, now := testEngine(t, c)
	control, controlNow := testEngine(t, c)
	e.Select("band")
	if s := e.State(); s.ActiveBaseMoodID != "band" || s.ActiveMoodID != "warm" || s.DMX[0] != 128 {
		t.Fatal("base selection replaced atmosphere or did not fill front", s)
	}
	for i := 0; i < 10; i++ {
		*now = now.Add(time.Second)
		*controlNow = *now
		e.Select("speaker")
		e.Select("band")
		s, other := e.State(), control.State()
		if s.DMX[0] != 128 || !reflect.DeepEqual(s.DMX[3:], other.DMX[3:]) {
			t.Fatal("base changes restarted atmosphere drift or changed static front")
		}
	}
	e.ClearBase()
	s := e.State()
	other := control.State()
	if s.ActiveBaseMoodID != "" || s.ActiveMoodID != "warm" || s.DMX[0] != 0 ||
		!reflect.DeepEqual(s.DMX[3:], other.DMX[3:]) {
		t.Fatal("No base affected atmosphere")
	}
}

func TestAtmosphereOverridesBaseIncludingExplicitDarkness(t *testing.T) {
	c := layeredRig()
	override := c.Moods[0]
	override.ID, override.Name = "dark-front", "Dark front"
	override.Groups = []config.MoodGroup{{GroupID: "front", ColorMode: "static", Palette: []string{"#0000ff"}}}
	c.Moods = append(c.Moods, override)
	e, _ := testEngine(t, c)
	e.Select("band")
	e.Select("dark-front")
	if s := e.State(); s.DMX[0] != 0 || s.Fixtures[0].Intensity != 0 || s.ActiveBaseMoodID != "band" {
		t.Fatal("explicit zero atmosphere must override base, not fall through")
	}
	e.Select("warm")
	if e.State().DMX[0] != 128 {
		t.Fatal("unassigned front must return to base")
	}
}

func TestIndependentLayerCrossfadesAndInterruptedBaseFade(t *testing.T) {
	c := layeredRig()
	c.Moods[1].CrossfadeSeconds, c.Moods[2].CrossfadeSeconds = 2, 2
	override := c.Moods[0]
	override.ID, override.Name, override.CrossfadeSeconds = "blue-front", "Blue front", 2
	override.Groups = []config.MoodGroup{{GroupID: "front", ColorMode: "static", Palette: []string{"#0000ff"}, IntensityMin: 1, IntensityMax: 1}}
	c.Moods[0].CrossfadeSeconds = 2
	c.Moods = append(c.Moods, override)
	e, now := testEngine(t, c)
	*now = now.Add(3 * time.Second)
	e.Select("band")
	*now = now.Add(time.Second)
	if e.State().DMX[0] != 64 {
		t.Fatal("base did not fade in independently")
	}
	before := e.State().DMX
	e.Select("speaker")
	if e.State().DMX != before {
		t.Fatal("interrupted base fade jumped")
	}
	*now = now.Add(2 * time.Second)
	e.Select("band")
	*now = now.Add(2 * time.Second)
	e.Select("blue-front")
	*now = now.Add(time.Second)
	s := e.State()
	if s.DMX[0] != 64 || s.DMX[2] != 128 {
		t.Fatal("atmosphere/base override blend is wrong", s.DMX[:3])
	}
	*now = now.Add(time.Second)
	e.Select("speaker")
	*now = now.Add(2 * time.Second)
	if s := e.State(); s.DMX[0] != 0 || s.DMX[1] != 0 || s.DMX[2] != 255 {
		t.Fatal("base change leaked through assigned atmosphere", s.DMX[:3])
	}
	e.Select("warm")
	*now = now.Add(time.Second)
	if s := e.State(); s.DMX[1] != 32 || s.DMX[2] != 128 {
		t.Fatal("fallback crossfade is wrong", s.DMX[:3])
	}
	*now = now.Add(time.Second)
	e.ClearBase()
	*now = now.Add(time.Second)
	c.BPM = 60
	e.Configure(c)
	if e.State().DMX[1] != 32 {
		t.Fatal("tempo save reset No base fade")
	}
	*now = now.Add(time.Second)
	if e.State().DMX[1] != 0 {
		t.Fatal("No base fade did not finish")
	}
}

func TestBaseOnlyBlackoutMasterAndConfigurationSafety(t *testing.T) {
	c := layeredRig()
	now := time.Unix(1000, 0)
	e := New(c, func() time.Time { return now }, 42)
	e.Select("band")
	if !e.State().Blackout || e.Blackout(false) != nil {
		t.Fatal("base-only release failed")
	}

	e.SetMaster(0.5)
	if e.State().DMX[0] != 64 {
		t.Fatal("master was not applied once")
	}
	e.Blackout(true)
	e.Select("speaker")
	e.Select("warm")
	if s := e.State(); !s.Blackout || s.DMX[0] != 0 || s.DMX[1] != 0 {
		t.Fatal("selection released blackout")
	}
	e.Blackout(false)
	if e.State().DMX[1] != 32 {
		t.Fatal("blackout did not restore both layers")
	}
	c.Moods[2].Groups[0].IntensityMin, c.Moods[2].Groups[0].IntensityMax = 0.5, 0.5
	e.Configure(c)
	if e.State().DMX[1] != 64 || e.State().ActiveMoodID != "warm" {
		t.Fatal("base edit changed atmosphere selection")
	}
	c.Moods = c.Moods[:2]
	e.Configure(c)
	if s := e.State(); !s.Blackout || s.ActiveBaseMoodID != "" || s.ActiveMoodID != "warm" {
		t.Fatal("deleted base was not cleared safely")
	}
	fresh := New(c, nil, 1).State()
	if !fresh.Blackout || fresh.ActiveBaseMoodID != "" || fresh.ActiveMoodID != "" {
		t.Fatal("layers persisted on restart")
	}
}

func TestAtmosphereChangesPreserveBaseFadeAndNoBaseCanFinishAlone(t *testing.T) {
	c := layeredRig()
	c.Moods[1].CrossfadeSeconds = 2
	e, now := testEngine(t, c)
	e.Select("band")
	*now = now.Add(time.Second)
	e.Select("warm")
	if e.State().DMX[0] != 64 {
		t.Fatal("atmosphere selection reset base fade")
	}
	*now = now.Add(time.Second)
	if e.State().DMX[0] != 128 {
		t.Fatal("base fade did not finish")
	}
	baseOnly := New(c, func() time.Time { return *now }, 42)
	baseOnly.Select("band")
	baseOnly.Blackout(false)
	*now = now.Add(2 * time.Second)
	baseOnly.ClearBase()
	*now = now.Add(time.Second)
	baseOnly.Configure(c)
	if s := baseOnly.State(); s.Blackout || s.DMX[0] != 64 {
		t.Fatal("save interrupted base-only fade out", s.DMX[0])
	}
	*now = now.Add(time.Second)
	if baseOnly.State().DMX[0] != 0 {
		t.Fatal("No base did not reach darkness")
	}
}

func TestBlackoutRestoresCompositeOnceUsingLongerLayerFade(t *testing.T) {
	c := layeredRig()
	c.Moods[0].CrossfadeSeconds, c.Moods[1].CrossfadeSeconds = 1, 2
	now := time.Unix(1000, 0)
	e := New(c, func() time.Time { return now }, 42)
	e.Select("band")
	e.Select("warm")
	e.Blackout(false)
	if s := e.State(); s.DMX[0] != 0 || s.Fixtures[1].Intensity != 0 {
		t.Fatal("restore did not start dark")
	}
	now = now.Add(time.Second)
	if s := e.State(); s.DMX[0] != 64 || s.Fixtures[0].Intensity != 0.25 {
		t.Fatal("restore brightness applied more than once", s.DMX[0])
	}
	now = now.Add(time.Second)
	if e.State().DMX[0] != 128 {
		t.Fatal("restore did not finish")
	}
}
