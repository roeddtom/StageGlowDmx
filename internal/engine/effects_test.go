package engine

import (
	"reflect"
	"testing"
	"time"

	"stageglowdmx/internal/config"
)

func effectRig() config.Config {
	c := layeredRig()
	c.Moods = append(c.Moods, config.Mood{
		ID: "fx", Name: "Effect", Type: "effect", BeatsPerChange: 2, Smoothness: 1, CrossfadeSeconds: 1,
		Effect: &config.Effect{Pattern: "swell", Activation: "timed", DurationBeats: 4, OverrideColour: true},
		Groups: []config.MoodGroup{{GroupID: "front", ColorMode: "static", Palette: []string{"#0000ff"}, IntensityMin: 1, IntensityMax: 1}},
	})
	return c
}

func TestTimedEffectOverlayRestoresRunningLayers(t *testing.T) {
	c := effectRig()
	e, now := testEngine(t, c)
	control, controlNow := testEngine(t, c)
	e.Select("band")
	control.Select("band")
	if err := e.TriggerEffect("fx"); err != nil {
		t.Fatal(err)
	}
	*now = now.Add(time.Second) // Half of four beats at 120 BPM.
	*controlNow = *now
	s, baseline := e.State(), control.State()
	if s.ActiveEffectID != "fx" || s.ActiveBaseMoodID != "band" || s.ActiveMoodID != "warm" ||
		s.DMX[0] != 0 || s.DMX[2] != 255 || !reflect.DeepEqual(s.DMX[3:], baseline.DMX[3:]) {
		t.Fatal("effect did not override only its group", s)
	}
	*now = now.Add(time.Second)
	*controlNow = *now
	if s := e.State(); s.ActiveEffectID != "" || s.DMX != control.State().DMX {
		t.Fatal("timed effect did not restore continuously-running layers", s)
	}
}

func TestEffectIntensityOnlyMasterAndWhiteSpot(t *testing.T) {
	for _, white := range []bool{false, true} {
		c := effectRig()
		c.Moods[3].Effect.OverrideColour = false
		if white {
			c.Profiles[0].Footprint = 1
			c.Profiles[0].Channels = []config.Channel{{Offset: 0, Kind: "white"}}
		}
		e, now := testEngine(t, c)
		e.Select("band")
		e.SetMaster(0.5)
		e.TriggerEffect("fx")
		*now = now.Add(time.Second)
		s := e.State()
		if s.DMX[0] != 128 || s.Fixtures[0].Intensity != 0.5 || (!white && (s.DMX[1] != 0 || s.DMX[2] != 0)) {
			t.Fatal("intensity effect changed colour or applied master twice", s.Fixtures[0])
		}
	}
}

func TestEffectReleaseReplacementBlackoutAndTimeout(t *testing.T) {
	c := effectRig()
	c.Moods[3].Effect.Pattern, c.Moods[3].Effect.Activation = "solo", "hold"
	e, now := testEngine(t, c)
	e.Select("band")
	e.TriggerEffect("fx")
	*now = now.Add(time.Second)
	e.StopEffect("other")
	if e.State().ActiveEffectID != "fx" {
		t.Fatal("unrelated release stopped effect")
	}
	e.StopEffect("fx")
	*now = now.Add(500 * time.Millisecond)
	s := e.State()
	if s.DMX[0] != 64 || s.DMX[2] != 128 {
		t.Fatal("manual release did not blend back", s.DMX[:3])
	}
	*now = now.Add(500 * time.Millisecond)
	if e.State().ActiveEffectID != "" || e.State().DMX[0] != 128 {
		t.Fatal("manual release did not finish")
	}
	e.TriggerEffect("fx")
	*now = now.Add(31 * time.Second)
	if e.State().ActiveEffectID != "" {
		t.Fatal("lost hold release was not bounded by server clock")
	}
	e.TriggerEffect("fx")
	e.Blackout(true)
	if s := e.State(); s.ActiveEffectID != "" || s.DMX != [512]int{} {
		t.Fatal("blackout must immediately cancel effects")
	}
	if e.TriggerEffect("fx") == nil || !e.State().Blackout {
		t.Fatal("effect must not release blackout")
	}
	e.Blackout(false)
	if e.State().ActiveEffectID != "" {
		t.Fatal("blackout restore revived cancelled effect")
	}
	if e.Select("fx") == nil || e.TriggerEffect("warm") == nil {
		t.Fatal("effect/mood command types were not kept separate")
	}
	c.Show.PauseMoodID = "warm"
	e.Configure(c)
	e.TriggerEffect("fx")
	e.Select("warm")
	if e.State().ActiveEffectID != "" {
		t.Fatal("Pause must cancel effects immediately")
	}
}

func TestEffectTempoToggleAndConfigurationChanges(t *testing.T) {
	c := effectRig()
	e, now := testEngine(t, c)
	e.TriggerEffect("fx")
	*now = now.Add(500 * time.Millisecond) // One beat.
	c.BPM = 60
	e.Configure(c)
	if e.State().ActiveEffectID != "fx" {
		t.Fatal("unrelated save cancelled effect")
	}
	*now = now.Add(3 * time.Second) // Three further beats.
	if e.State().ActiveEffectID != "" {
		t.Fatal("effect duration did not follow tempo change")
	}
	c.Moods[3].Effect.Activation = "toggle"
	c.Moods[3].Effect.Pattern = "solo"
	c.Moods[3].CrossfadeSeconds = 0
	e.Configure(c)
	e.TriggerEffect("fx")
	*now = now.Add(time.Hour)
	if e.State().ActiveEffectID != "fx" {
		t.Fatal("toggle unexpectedly timed out")
	}
	e.TriggerEffect("fx")
	if e.State().ActiveEffectID != "" {
		t.Fatal("second trigger did not toggle off")
	}
	e.TriggerEffect("fx")
	c.Moods[3].Groups[0].Palette[0] = "#00ff00"
	e.Configure(c)
	if e.State().ActiveEffectID != "" {
		t.Fatal("editing active effect did not cancel it")
	}
	e.TriggerEffect("fx")
	c.Moods = c.Moods[:3]
	e.Configure(c)
	if e.State().ActiveEffectID != "" || e.State().Blackout {
		t.Fatal("deleting an effect must cancel it without disturbing underlying moods")
	}
}

func TestEffectPatternsAndSingleActiveEffect(t *testing.T) {
	for _, pattern := range []string{"swell", "accent", "solo", "pulse"} {
		t.Run(pattern, func(t *testing.T) {
			c := effectRig()
			c.Moods[3].Effect.Pattern = pattern
			e, now := testEngine(t, c)
			e.Select("band")
			e.TriggerEffect("fx")
			*now = now.Add(500 * time.Millisecond)
			if e.State().DMX[2] == 0 {
				t.Fatal("pattern produced no accent")
			}
			other := c.Copy().Moods[3]
			other.ID = "other"
			c.Moods = append(c.Moods, other)
			e.Configure(c)
			e.TriggerEffect("other")
			e.StopEffect("fx")
			if e.State().ActiveEffectID != "other" {
				t.Fatal("new effect was not independent of stale old release")
			}
			*now = now.Add(3 * time.Second)
			if s := e.State(); s.ActiveEffectID != "" || s.DMX[0] != 128 {
				t.Fatal("pattern did not end at underlying base")
			}
		})
	}
}
