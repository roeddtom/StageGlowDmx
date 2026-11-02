package server

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"stageglowdmx/internal/config"
	"stageglowdmx/internal/engine"
)

func TestSimulationCommandsAndPersistedMoods(t *testing.T) {
	_, store, dir := setup(t)
	c := store.Get()
	c.Profiles = []config.Profile{{
		ID: "rgb", Name: "RGB", Footprint: 3, WhiteMix: 1, AmberMix: 1,
		Channels: []config.Channel{{Offset: 0, Kind: "red"}, {Offset: 1, Kind: "green"}, {Offset: 2, Kind: "blue"}},
	}}
	c.Groups = []config.Group{{ID: "front", Name: "Front"}}
	c.Loadouts = []config.Loadout{{ID: "rig", Name: "Rig", Fixtures: []config.Fixture{{
		ID: "one", Name: "One", ProfileID: "rgb", GroupID: "front", StartAddress: 1,
	}}}}
	c.ActiveLoadoutID = "rig"
	c.Moods = []config.Mood{{
		ID: "warm", Name: "Warm", BeatsPerChange: 8, Smoothness: 1, CrossfadeSeconds: 2,
		Groups: []config.MoodGroup{{GroupID: "front", ColorMode: "static", Palette: []string{"#ff0000"}, IntensityMin: 0.5, IntensityMax: 0.5}},
	}}
	c.Show.Slots[0], c.Show.PauseMoodID = "warm", "warm"
	if err := store.Save(c); err != nil {
		t.Fatal(err)
	}

	t.Run("effect commands and blackout safety", func(t *testing.T) {
		_, store, dir := setup(t)
		c := store.Get()
		c.Groups = []config.Group{{ID: "front", Name: "Front"}}
		mood := config.Mood{ID: "warm", Name: "Warm", BeatsPerChange: 8, Smoothness: 1,
			Groups: []config.MoodGroup{{GroupID: "front", ColorMode: "static", Palette: []string{"#ffffff"}, IntensityMin: 1, IntensityMax: 1}}}
		fx := mood
		fx.ID, fx.Name, fx.Type = "fx", "Accent", "effect"
		fx.Effect = &config.Effect{Pattern: "accent", Activation: "hold", DurationBeats: 4}
		c.Moods = []config.Mood{mood, fx}
		if err := store.Save(c); err != nil {
			t.Fatal(err)
		}
		now := time.Unix(1000, 0)
		h := NewWithEngine(store, nil, engine.New(c, func() time.Time { return now }, 42))
		call := func(body string, status int) engine.State {
			t.Helper()
			w := request(h, "POST", "/api/simulation", body)
			if w.Code != status {
				t.Fatal(w.Code, w.Body.String())
			}
			var s engine.State
			json.Unmarshal(w.Body.Bytes(), &s)
			return s
		}
		call(`{"action":"effect","moodId":"fx"}`, 400)
		call(`{"action":"play","moodId":"fx"}`, 400)
		call(`{"action":"select","moodId":"fx"}`, 400)
		call(`{"action":"play","moodId":"warm"}`, 200)
		for _, invalid := range []string{
			`{"action":"effect"}`, `{"action":"effect","moodId":"warm"}`,
			`{"action":"effect","moodId":"fx","enabled":true}`, `{"action":"stop-effect","value":1}`,
		} {
			call(invalid, 400)
		}
		if s := call(`{"action":"effect","moodId":"fx"}`, 200); s.ActiveEffectID != "fx" || s.ActiveMoodID != "warm" {
			t.Fatal("effect command changed mood selection", s)
		}
		now = now.Add(time.Second)
		if s := call(`{"action":"stop-effect","moodId":"fx"}`, 200); s.ActiveEffectID != "" || s.Blackout {
			t.Fatal("stop effect failed", s)
		}
		call(`{"action":"effect","moodId":"fx"}`, 200)
		if s := call(`{"action":"blackout","enabled":true}`, 200); s.ActiveEffectID != "" || !s.Blackout {
			t.Fatal("blackout retained effect", s)
		}
		reopened, err := config.Open(dir)
		if err != nil || reopened.Get().Moods[1].Effect.Pattern != "accent" {
			t.Fatal("effect settings were not persisted", err)
		}
	})
	now := time.Unix(1000, 0)
	h := NewWithEngine(store, nil, engine.New(store.Get(), func() time.Time { return now }, 42))
	state := func() engine.State {
		t.Helper()
		w := request(h, http.MethodGet, "/api/state", "")
		var result engine.State
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || w.Code != 200 {
			t.Fatal("invalid simulation response", err, w.Body.String())
		}
		return result
	}
	command := func(body string, status int) {
		t.Helper()
		w := request(h, http.MethodPost, "/api/simulation", body)
		if w.Code != status {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	command(`{"action":"blackout","enabled":false}`, 400)
	command(`{"action":"select","moodId":"warm"}`, 200)
	if !state().Blackout {
		t.Fatal("select must not release blackout")
	}
	command(`{"action":"blackout","enabled":false}`, 200)
	now = now.Add(2 * time.Second)
	if s := state(); s.DMX[0] != 128 || s.OutputEnabled {
		t.Fatal("expected simulation only", s.DMX[0], s.OutputEnabled)
	}
	for _, invalid := range []string{
		`{"action":"select","moodId":"missing"}`,
		`{"action":"master","value":2}`,
		`{"action":"master","value":null}`,
		`{"action":"blackout"}`,
		`{"action":"blackout","enabled":false,"moodId":"warm"}`,
		`{"action":"blackout","enabled":true,"outputEnabled":true}`,
		`{"action":"blackout","enabled":true} {}`,
	} {
		command(invalid, 400)
		if state().Blackout {
			t.Fatal("rejected command changed state", invalid)
		}
	}
	command(`{"action":"master","value":0.5}`, 200)
	if state().DMX[0] != 64 {
		t.Fatal("master was not applied")
	}
	command(`{"action":"blackout","enabled":true}`, 200)
	command(`{"action":"play","moodId":"warm"}`, 200)
	if state().Blackout {
		t.Fatal("Show play did not immediately release simulation blackout")
	}
	c.BPM = 60
	w := request(h, http.MethodPut, "/api/config", payload(t, c))
	if w.Code != 200 || state().BPM != 60 || state().Blackout {
		t.Fatal("tempo save failed or reset simulation", w.Body.String())
	}
	w = request(h, http.MethodPost, "/api/config/import", payload(t, c))
	if w.Code != 200 || !state().Blackout {
		t.Fatal("import must latch simulation blackout")
	}
	reopened, err := config.Open(dir)
	if err != nil || len(reopened.Get().Moods) != 1 || reopened.Get().Show.Slots[0] != "warm" || reopened.Get().Show.PauseMoodID != "warm" {
		t.Fatal("mood and show layout were not persisted", err)
	}
	var exported config.Config
	if err := json.Unmarshal(request(h, "GET", "/api/config/export", "").Body.Bytes(), &exported); err != nil ||
		exported.Show.Slots[0] != "warm" || exported.Show.PauseMoodID != "warm" {
		t.Fatal("show layout missing from backup", err)
	}
	restarted := New(reopened, nil)
	var fresh engine.State
	json.Unmarshal(request(restarted, "GET", "/api/state", "").Body.Bytes(), &fresh)
	if !fresh.Blackout || fresh.ActiveMoodID != "" || fresh.OutputEnabled {
		t.Fatal("simulation state must not persist across restart")
	}
}
