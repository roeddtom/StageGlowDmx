package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"stageglowdmx/internal/config"
	"stageglowdmx/internal/engine"
	"stageglowdmx/internal/output"
)

type usbDevice struct {
	frames []output.Frame
}

func (d *usbDevice) WriteFrame(_ context.Context, frame output.Frame) error {
	d.frames = append(d.frames, frame)
	return nil
}
func (d *usbDevice) Close(context.Context) error { return nil }

func TestUSBOutputAPI(t *testing.T) {
	_, store, _ := setup(t)
	c := store.Get()
	c.Output = config.Output{Mode: "enttec", SerialPort: "COM3"}
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
		ID: "red", Name: "Red", BeatsPerChange: 8, Smoothness: 1,
		Groups: []config.MoodGroup{{GroupID: "front", ColorMode: "static", Palette: []string{"#ff0000"}, IntensityMin: 1, IntensityMax: 1}},
	}}
	if err := store.Save(c); err != nil {
		t.Fatal(err)
	}
	device := &usbDevice{}
	controller := output.New(c, engine.New(c, nil, 1), func(string) (output.Device, error) { return device, nil })
	defer controller.Shutdown()
	h := NewWithOutput(store, nil, controller)
	call := func(method, path, body string, status int) {
		t.Helper()
		w := request(h, method, path, body)
		if w.Code != status {
			t.Fatalf("%s %s: %d %s", method, path, w.Code, w.Body.String())
		}
	}
	state := func() struct {
		engine.State
		Output output.Status `json:"output"`
	} {
		t.Helper()
		var result struct {
			engine.State
			Output output.Status `json:"output"`
		}
		if err := json.Unmarshal(request(h, "GET", "/api/state", "").Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	for _, invalid := range []string{`{}`, `{"enabled":null}`, `{"enabled":true,"port":"COM4"}`, `{"enabled":true} {}`} {
		call("POST", "/api/output", invalid, 400)
	}
	call("POST", "/api/lighting", `{"action":"blackout","enabled":false}`, 409)
	call("POST", "/api/simulation", `{"action":"select","moodId":"red"}`, 200)
	call("POST", "/api/simulation", `{"action":"blackout","enabled":false}`, 200)
	call("POST", "/api/output", `{"enabled":true}`, 200)
	if s := state(); !s.Blackout || !s.OutputEnabled || s.EngineStatus != "enttec" || s.Output.State != "enabled" {
		t.Fatal("enable must send blackout and publish hardware status", s)
	}
	if len(device.frames) != 1 || device.frames[0][0] != 0 {
		t.Fatal("unsafe first frame", device.frames)
	}
	call("POST", "/api/simulation", `{"action":"blackout","enabled":false}`, 409)
	call("POST", "/api/simulation", `{"action":"master","value":0.5}`, 409)
	call("POST", "/api/lighting", `{"action":"blackout","enabled":false}`, 200)
	if device.frames[len(device.frames)-1][0] != 255 {
		t.Fatal("hardware release did not send mood")
	}
	call("POST", "/api/simulation", `{"action":"blackout","enabled":true}`, 200)
	if device.frames[len(device.frames)-1][0] != 0 {
		t.Fatal("emergency simulation blackout must also darken USB")
	}
	modified := c.Copy()
	modified.Loadouts[0].Fixtures[0].StartAddress = 10
	call("PUT", "/api/config", payload(t, modified), 409)
	call("POST", "/api/config/import", payload(t, c), 409)
	modified = c.Copy()
	modified.BPM = 60
	call("PUT", "/api/config", payload(t, modified), 200)
	var exported config.Config
	if err := json.Unmarshal(request(h, "GET", "/api/config/export", "").Body.Bytes(), &exported); err != nil {
		t.Fatal(err)
	}
	if exported.Output.Mode != "simulation" || exported.Output.SerialPort != "" || store.Get().Output.SerialPort != "COM3" {
		t.Fatal("export must strip local output without changing saved settings")
	}
	call("POST", "/api/output", `{"enabled":false}`, 200)
	call("POST", "/api/lighting", `{"action":"blackout","enabled":false}`, 409)
	call("POST", "/api/config/import", payload(t, c), 200)
	if s := state(); !s.Blackout || s.OutputEnabled || s.Output.Mode != "simulation" || store.Get().Output.SerialPort != "" {
		t.Fatal("import must reset local hardware settings and latch blackout", s)
	}

	failed := output.New(c, engine.New(c, nil, 1), func(string) (output.Device, error) {
		return nil, errors.New("device unplugged")
	})
	h = NewWithOutput(store, nil, failed)
	call(http.MethodPost, "/api/output", `{"enabled":true}`, 409)
	if s := state(); s.OutputEnabled || !s.Blackout || s.Output.State != "fault" || s.Output.Error == "" {
		t.Fatal("open error must be visible and disarmed", s)
	}
}
