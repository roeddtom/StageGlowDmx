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
	if err := store.Save(c); err != nil {
		t.Fatal(err)
	}
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
	if err != nil || len(reopened.Get().Moods) != 1 {
		t.Fatal("mood was not persisted", err)
	}
	restarted := New(reopened, nil)
	var fresh engine.State
	json.Unmarshal(request(restarted, "GET", "/api/state", "").Body.Bytes(), &fresh)
	if !fresh.Blackout || fresh.ActiveMoodID != "" || fresh.OutputEnabled {
		t.Fatal("simulation state must not persist across restart")
	}
}
