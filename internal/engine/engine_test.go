package engine

import (
	"math"
	"reflect"
	"testing"
	"time"

	"stageglowdmx/internal/config"
)

func testRig() config.Config {
	c := config.Defaults()
	c.Profiles = []config.Profile{{
		ID: "rgb", Name: "RGB", Footprint: 3, WhiteMix: 1, AmberMix: 1,
		Channels: []config.Channel{{Offset: 0, Kind: "red"}, {Offset: 1, Kind: "green"}, {Offset: 2, Kind: "blue"}},
	}}
	c.Groups = []config.Group{{ID: "front", Name: "Front"}, {ID: "back", Name: "Back"}}
	c.Loadouts = []config.Loadout{{ID: "rig", Name: "Rig", Fixtures: []config.Fixture{
		{ID: "front", Name: "Front", ProfileID: "rgb", GroupID: "front", StartAddress: 1},
		{ID: "back-1", Name: "Back 1", ProfileID: "rgb", GroupID: "back", StartAddress: 4},
		{ID: "back-2", Name: "Back 2", ProfileID: "rgb", GroupID: "back", StartAddress: 7},
	}}}
	c.ActiveLoadoutID = "rig"
	c.Moods = []config.Mood{{
		ID: "warm", Name: "Warm", BeatsPerChange: 8, Smoothness: 1, CrossfadeSeconds: 0,
		Groups: []config.MoodGroup{
			{GroupID: "front", ColorMode: "static", Palette: []string{"#ff8000"}, IntensityMin: 0.5, IntensityMax: 0.5},
			{GroupID: "back", ColorMode: "palette", Palette: []string{"#0000ff", "#ff00ff"}, IntensityMin: 0.2, IntensityMax: 0.8},
		},
	}}
	return c
}

func testEngine(t *testing.T, c config.Config) (*Engine, *time.Time) {
	t.Helper()
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1000, 0)
	e := New(c, func() time.Time { return now }, 42)
	if err := e.Select(c.Moods[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := e.Blackout(false); err != nil {
		t.Fatal(err)
	}
	return e, &now
}

func TestStaticFrontAndIndependentBoundedDrift(t *testing.T) {
	e, now := testEngine(t, testRig())
	initial := e.State()
	front := initial.Fixtures[0]
	if front.Color != "#804000" || front.Intensity != 0.5 {
		t.Fatal("static light was not mapped", front)
	}
	changed, independent := false, false
	for i := 0; i < 100; i++ {
		*now = now.Add(100 * time.Millisecond)
		state := e.State()
		if !reflect.DeepEqual(state.Fixtures[0], front) {
			t.Fatal("static front light drifted")
		}
		changed = changed || !reflect.DeepEqual(initial.Fixtures[1], state.Fixtures[1])
		independent = independent || state.Fixtures[1].Color != state.Fixtures[2].Color
		for _, fixture := range state.Fixtures[1:] {
			if fixture.Intensity < 0.2 || fixture.Intensity > 0.8 {
				t.Fatal("intensity escaped mood bounds", fixture.Intensity)
			}
			if fixture.Channels[1] != 0 || fixture.Channels[0] > fixture.Channels[2] {
				t.Fatal("colour escaped the blue/magenta palette", fixture.Channels)
			}
		}
	}
	if !changed || !independent {
		t.Fatal("drift must change independently across fixtures")
	}
}

func TestSamplingDoesNotControlClockAndTargets(t *testing.T) {
	first, now := testEngine(t, testRig())
	second, later := testEngine(t, testRig())
	for i := 0; i < 500; i++ {
		*now = now.Add(100 * time.Millisecond)
		first.State()
	}
	*later = *now
	if !reflect.DeepEqual(first.State(), second.State()) {
		t.Fatal("a disconnected browser changed mood generation")
	}
	*now = now.Add(24 * time.Hour)
	*later = *now
	if !reflect.DeepEqual(first.State(), second.State()) {
		t.Fatal("long gaps must not change deterministic targets")
	}
}

func TestIntervalBoundaryAndTempoChangeAreContinuous(t *testing.T) {
	c := testRig()
	e, now := testEngine(t, c)
	*now = now.Add(4*time.Second - time.Microsecond)
	before := e.State()
	*now = now.Add(2 * time.Microsecond)
	after := e.State()
	for i := range before.DMX {
		if math.Abs(float64(before.DMX[i]-after.DMX[i])) > 1 {
			t.Fatal("palette/intensity snapped at interval boundary")
		}
	}
	*now = now.Add(time.Second)
	before = e.State()
	c.BPM = 60
	e.Configure(c)
	if !reflect.DeepEqual(before.DMX, e.State().DMX) {
		t.Fatal("tempo change snapped the rendered output")
	}
	// One second at 60 BPM advances as far as half a second at 120 BPM.
	baseline, baselineNow := testEngine(t, testRig())
	*baselineNow = now.Add(500 * time.Millisecond)
	*now = now.Add(time.Second)
	if !reflect.DeepEqual(baseline.State().DMX, e.State().DMX) {
		t.Fatal("tempo did not retime the remaining interval")
	}
}

func TestCrossfadeAndInterruptedFadeUseRenderedLight(t *testing.T) {
	c := testRig()
	c.Moods[0].Groups = c.Moods[0].Groups[:1]
	c.Moods[0].Groups[0].Palette = []string{"#ff0000"}
	c.Moods[0].Groups[0].IntensityMin, c.Moods[0].Groups[0].IntensityMax = 1, 1
	blue := c.Moods[0]
	blue.ID, blue.Name, blue.CrossfadeSeconds = "blue", "Blue", 2
	blue.Groups = []config.MoodGroup{{
		GroupID: "front", ColorMode: "static", Palette: []string{"#0000ff"}, IntensityMin: 1, IntensityMax: 1,
	}}
	c.Moods = append(c.Moods, blue)
	c.Moods[0].CrossfadeSeconds = 2
	e, now := testEngine(t, c)
	*now = now.Add(2 * time.Second)
	e.Select("blue")
	*now = now.Add(time.Second)
	half := e.State()
	if half.DMX[0] != 128 || half.DMX[2] != 128 {
		t.Fatal("expected half red, half blue", half.DMX[:3])
	}
	e.Select("warm")
	if !reflect.DeepEqual(half.DMX, e.State().DMX) {
		t.Fatal("interrupted fade jumped")
	}
}

func TestFadeFromBlackDoesNotSquareDimmerIntensity(t *testing.T) {
	c := testRig()
	c.Moods[0].CrossfadeSeconds = 2
	c.Profiles[0].Channels = append(c.Profiles[0].Channels, config.Channel{Offset: 3, Kind: "dimmer"})
	c.Profiles[0].Footprint = 4
	c.Loadouts[0].Fixtures = c.Loadouts[0].Fixtures[:1]
	e, now := testEngine(t, c)
	*now = now.Add(time.Second)
	state := e.State()
	if state.DMX[0] != 255 || state.DMX[3] != 64 || state.Fixtures[0].Intensity != 0.25 {
		t.Fatal("fade should apply brightness once", state.DMX[:4], state.Fixtures[0])
	}
}

func TestMixingIntensityUVAndSafeBlackout(t *testing.T) {
	for _, dimmer := range []bool{false, true} {
		c := testRig()
		c.Loadouts[0].Fixtures = c.Loadouts[0].Fixtures[:1]
		safe, blackout := 240, 16
		p := &c.Profiles[0]
		for _, kind := range []string{"white", "amber", "uv", "fixed"} {
			p.Channels = append(p.Channels, config.Channel{
				Offset: p.Footprint, Kind: kind, Value: &safe, BlackoutValue: &blackout,
			})
			p.Footprint++
		}
		if dimmer {
			p.Channels = append(p.Channels, config.Channel{Offset: p.Footprint, Kind: "dimmer", BlackoutValue: &safe})
			p.Footprint++
		}
		g := &c.Moods[0].Groups[0]
		g.Palette, g.UV = []string{"#ffffff"}, 0.4
		e, _ := testEngine(t, c)
		e.SetMaster(0.5)
		state := e.State()
		if dimmer {
			if state.DMX[3] != 255 || state.DMX[5] != 102 || state.DMX[7] != 64 {
				t.Fatal("dimmer applied twice or UV incorrect", state.DMX[:8])
			}
		} else if state.DMX[3] != 64 || state.DMX[5] != 26 {
			t.Fatal("white/UV scaling incorrect", state.DMX[:7])
		}
		if state.DMX[0] != 0 || state.DMX[1] != 0 || state.DMX[2] != 0 || state.DMX[6] != safe {
			t.Fatal("white extraction/fixed channel incorrect", state.DMX[:8])
		}
		e.Blackout(true)
		state = e.State()
		for i, value := range state.DMX {
			if i == 6 {
				if value != blackout {
					t.Fatal("fixed blackout value ignored")
				}
			} else if value != 0 {
				t.Fatal("blackout must override emitting channel's configured blackout value", i, value)
			}
		}
		// Amber extraction must preserve nonnegative, bounded RGB fallback.
		g.Palette = []string{"#ff8000"}
		e.Configure(c)
		e.Blackout(false)
		state = e.State()
		if state.DMX[4] == 0 {
			t.Fatal("amber was not extracted")
		}
		for _, value := range state.DMX {
			if value < 0 || value > 255 {
				t.Fatal("DMX out of bounds")
			}
		}
	}
}

func TestBlackoutLatchesAcrossMoodChangesAndRestart(t *testing.T) {
	c := testRig()
	e := New(c, nil, 1)
	if !e.State().Blackout || e.State().OutputEnabled {
		t.Fatal("unsafe startup")
	}
	if e.Blackout(false) == nil || e.Select("missing") == nil {
		t.Fatal("invalid simulation control accepted")
	}
	e.Select("warm")
	if !e.State().Blackout {
		t.Fatal("mood selection released blackout")
	}
	e.Blackout(false)
	e.Blackout(true)
	e.Select("warm")
	if !e.State().Blackout {
		t.Fatal("blackout was not latched")
	}
	e.Blackout(false)
	if e.State().Fixtures[0].Intensity != 0.5 {
		t.Fatal("blackout release did not restore mood")
	}
	restarted := New(c, nil, 1)
	if !restarted.State().Blackout || restarted.State().ActiveMoodID != "" {
		t.Fatal("runtime armed state persisted")
	}
}

func TestConfigurationChangesAndMissingGroupsFailClosed(t *testing.T) {
	c := testRig()
	c.Moods[0].Groups[0].UV = 0.4
	e, now := testEngine(t, c)
	if !e.State().Fixtures[0].UnsupportedUV {
		t.Fatal("missing UV capability must be visible")
	}
	*now = now.Add(time.Second)
	before := e.State()
	e.Configure(c)
	if !reflect.DeepEqual(before, e.State()) {
		t.Fatal("unrelated save reset generation")
	}
	c.Loadouts[0].Fixtures[0].Name = "Renamed"
	e.Configure(c)
	if !e.State().Blackout {
		t.Fatal("patch change must latch blackout")
	}
	e.Blackout(false)
	c.Moods[0].Groups = c.Moods[0].Groups[1:]
	e.Configure(c)
	if e.State().Fixtures[0].Intensity != 0 {
		t.Fatal("unassigned group must be dark")
	}
	c.Moods = []config.Mood{}
	e.Configure(c)
	if !e.State().Blackout || e.State().ActiveMoodID != "" {
		t.Fatal("deleting selected mood must black out")
	}
}
