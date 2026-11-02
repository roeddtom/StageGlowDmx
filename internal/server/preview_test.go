package server

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"stageglowdmx/internal/config"
	"stageglowdmx/internal/engine"
	"stageglowdmx/internal/output"
)

func TestIsolatedEffectPreview(t *testing.T) {
	_, store, _ := setup(t)
	c := store.Get()
	c.Output = config.Output{Mode: "enttec", SerialPort: "COM3"}
	c.Profiles = []config.Profile{
		{ID: "rgb", Name: "RGB", Footprint: 3, WhiteMix: 1, AmberMix: 1,
			Channels: []config.Channel{{Offset: 0, Kind: "red"}, {Offset: 1, Kind: "green"}, {Offset: 2, Kind: "blue"}}},
		{ID: "white", Name: "White", Footprint: 1, WhiteMix: 1, AmberMix: 1, Channels: []config.Channel{{Offset: 0, Kind: "white"}}},
	}
	c.Groups = []config.Group{{ID: "front", Name: "Front"}}
	c.Loadouts = []config.Loadout{{ID: "rig", Name: "Rig", Fixtures: []config.Fixture{
		{ID: "rgb", Name: "RGB", ProfileID: "rgb", StartAddress: 1, GroupID: "front"},
		{ID: "white", Name: "White", ProfileID: "white", StartAddress: 4, GroupID: "front"},
	}}}
	c.ActiveLoadoutID = "rig"
	base := config.Mood{ID: "base", Name: "Base", Type: "base", BeatsPerChange: 8, Smoothness: 1, CrossfadeSeconds: 3,
		Groups: []config.MoodGroup{{GroupID: "front", ColorMode: "static", Palette: []string{"#ff0000"}, IntensityMin: 0.2, IntensityMax: 0.2}}}
	atmosphere := base
	atmosphere.ID, atmosphere.Name, atmosphere.Type = "atmosphere", "Atmosphere", "atmosphere"
	atmosphere.Groups = []config.MoodGroup{{GroupID: "front", ColorMode: "static", Palette: []string{"#00ff00"}, IntensityMin: 0.4, IntensityMax: 0.4}}
	c.Moods = []config.Mood{base, atmosphere}
	if err := store.Save(c); err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1000, 0)
	device := &usbDevice{}
	controller := output.New(c, engine.New(c, func() time.Time { return now }, 1), func(string) (output.Device, error) { return device, nil })
	defer controller.Shutdown()
	if err := controller.Enable(); err != nil {
		t.Fatal(err)
	}
	if err := controller.Play("base", true); err != nil {
		t.Fatal(err)
	}
	controller.Master(0.25)
	h := NewWithOutput(store, nil, controller)
	before, beforeStatus := controller.State()
	frames := len(device.frames)
	effect := base
	effect.ID, effect.Name, effect.Type, effect.CrossfadeSeconds = "draft", "Unsaved swell", "effect", 1
	effect.Effect = &config.Effect{Pattern: "swell", Activation: "timed", DurationBeats: 4, OverrideColour: true}
	effect.Groups = []config.MoodGroup{{GroupID: "front", ColorMode: "static", Palette: []string{"#0000ff"}, IntensityMin: 1, IntensityMax: 1}}
	preview := func(baseID, atmosphereID string, elapsed float64, release *float64) engine.State {
		t.Helper()
		body, _ := json.Marshal(map[string]any{"mood": effect, "baseMoodId": baseID, "atmosphereMoodId": atmosphereID, "elapsedSeconds": elapsed, "releaseAtSeconds": release})
		w := request(h, "POST", "/api/effects/preview", string(body))
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		var s engine.State
		if err := json.Unmarshal(w.Body.Bytes(), &s); err != nil {
			t.Fatal(err)
		}
		return s
	}
	for _, bg := range [][2]string{{"", ""}, {"base", ""}, {"", "atmosphere"}, {"base", "atmosphere"}} {
		if s := preview(bg[0], bg[1], 1, nil); s.DMX[2] != 255 || s.DMX[3] != 255 || s.OutputEnabled || s.Master != 1 || s.Blackout {
			t.Fatal("effect preview did not peak independently of live output", s)
		}
		s := preview(bg[0], bg[1], 2, nil)
		if s.ActiveEffectID != "" || s.ActiveBaseMoodID != bg[0] || s.ActiveMoodID != bg[1] {
			t.Fatal("background selections did not survive effect ending", s)
		}
		want := [4]int{}
		if bg[0] != "" {
			want = [4]int{51, 0, 0, 51}
		}
		if bg[1] != "" {
			want = [4]int{0, 102, 0, 102}
		}
		if !reflect.DeepEqual(s.DMX[:4], want[:]) {
			t.Fatal("preview did not return to selected background", s.DMX[:4], want)
		}
	}
	release := 1.0
	if s := preview("", "atmosphere", 1.5, &release); s.DMX[1] != 51 || s.DMX[2] != 128 || s.DMX[3] != 179 {
		t.Fatal("preview manual release is incorrect", s.DMX[:4])
	}
	effect.Effect.OverrideColour = false
	if s := preview("", "", 1, nil); s.DMX[0] != 0 || s.DMX[1] != 0 || s.DMX[2] != 0 || s.DMX[3] != 255 {
		t.Fatal("intensity-only darkness behaviour differs between RGB and white spots", s.DMX[:4])
	}
	effect.Effect.Activation = "hold"
	if s := preview("", "", 31, nil); s.ActiveEffectID != "" {
		t.Fatal("preview did not model hold timeout")
	}
	body, _ := json.Marshal(map[string]any{"mood": effect, "elapsedSeconds": 1})
	valid := string(body)
	for _, invalid := range []string{
		`{}`, `{"elapsedSeconds":1,"mood":null}`, valid + `{}`,
		strings.Replace(valid, `"elapsedSeconds":1`, `"elapsedSeconds":-1`, 1),
		strings.Replace(valid, `"elapsedSeconds":1`, `"elapsedSeconds":3601`, 1),
		strings.Replace(valid, `"elapsedSeconds":1`, `"elapsedSeconds":1,"releaseAtSeconds":2`, 1),
		strings.Replace(valid, `"elapsedSeconds":1`, `"elapsedSeconds":1,"baseMoodId":"atmosphere"`, 1),
		strings.Replace(valid, `"elapsedSeconds":1`, `"elapsedSeconds":1,"atmosphereMoodId":"draft"`, 1),
		strings.Replace(valid, `"overrideColour":false`, `"overrideColour":false,"unknown":true`, 1),
		strings.Replace(valid, `"overrideColour":false`, `"overrideColour":null`, 1),
		strings.Replace(valid, `"type":"effect"`, `"type":"base"`, 1),
	} {
		if w := request(h, "POST", "/api/effects/preview", invalid); w.Code != 400 {
			t.Fatal("invalid preview accepted", w.Code, invalid, w.Body.String())
		}
	}
	if w := request(h, "GET", "/api/effects/preview", ""); w.Code != 405 {
		t.Fatal("preview should require POST")
	}
	after, afterStatus := controller.State()
	if !reflect.DeepEqual(before, after) || beforeStatus != afterStatus || len(device.frames) != frames || !reflect.DeepEqual(store.Get(), c) {
		t.Fatal("preview changed live lighting, USB frames or persisted configuration")
	}
}
