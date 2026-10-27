package config

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func moodRig() Config {
	c := rig()
	c.Moods = []Mood{{
		ID: "warm", Name: "Warm", BeatsPerChange: 8, Smoothness: 1, CrossfadeSeconds: 3,
		Groups: []MoodGroup{{
			GroupID: "front", ColorMode: "palette", Palette: []string{"#ff8000", "#ffffff"},
			IntensityMin: 0.2, IntensityMax: 0.6, UV: 0,
		}},
	}}
	return c
}

func TestMoodValidation(t *testing.T) {
	if err := moodRig().Validate(); err != nil {
		t.Fatal(err)
	}
	for name, edit := range map[string]func(*Config){
		"duplicate ID":       func(c *Config) { c.Moods = append(c.Moods, c.Moods[0]) },
		"blank name":         func(c *Config) { c.Moods[0].Name = " " },
		"unknown group":      func(c *Config) { c.Moods[0].Groups[0].GroupID = "missing" },
		"duplicate group":    func(c *Config) { c.Moods[0].Groups = append(c.Moods[0].Groups, c.Moods[0].Groups[0]) },
		"mode":               func(c *Config) { c.Moods[0].Groups[0].ColorMode = "strobe" },
		"empty palette":      func(c *Config) { c.Moods[0].Groups[0].Palette = []string{} },
		"invalid colour":     func(c *Config) { c.Moods[0].Groups[0].Palette[0] = "#gg0000" },
		"unordered range":    func(c *Config) { c.Moods[0].Groups[0].IntensityMin = 0.8 },
		"negative intensity": func(c *Config) { c.Moods[0].Groups[0].IntensityMin = -0.1 },
		"UV":                 func(c *Config) { c.Moods[0].Groups[0].UV = 2 },
		"tempo division":     func(c *Config) { c.Moods[0].BeatsPerChange = 0 },
		"snapping":           func(c *Config) { c.Moods[0].Smoothness = 0 },
		"NaN":                func(c *Config) { c.Moods[0].Smoothness = math.NaN() },
		"fade":               func(c *Config) { c.Moods[0].CrossfadeSeconds = math.Inf(1) },
		"null groups":        func(c *Config) { c.Moods[0].Groups = nil },
		"null moods":         func(c *Config) { c.Moods = nil },
	} {
		t.Run(name, func(t *testing.T) {
			c := moodRig()
			edit(&c)
			if c.Validate() == nil {
				t.Fatal("invalid mood accepted")
			}
		})
	}
}

func TestMoodDecodeLegacyAndRequiredFields(t *testing.T) {
	c := moodRig()
	data, _ := json.Marshal(c)
	var document map[string]any
	json.Unmarshal(data, &document)
	delete(document, "moods")
	legacy, _ := json.Marshal(document)
	loaded, err := Decode(strings.NewReader(string(legacy)))
	if err != nil || loaded.Moods == nil || len(loaded.Moods) != 0 {
		t.Fatal("old schema 1 backups must load with empty moods", err)
	}
	for _, field := range []string{"beatsPerChange", "smoothness", "crossfadeSeconds", "groups"} {
		var object map[string]any
		json.Unmarshal(data, &object)
		delete(object["moods"].([]any)[0].(map[string]any), field)
		invalid, _ := json.Marshal(object)
		if _, err := Decode(strings.NewReader(string(invalid))); err == nil {
			t.Fatal("missing mood field accepted", field)
		}
	}
	for _, field := range []string{"palette", "intensityMin", "intensityMax", "uv", "colorMode"} {
		var object map[string]any
		json.Unmarshal(data, &object)
		group := object["moods"].([]any)[0].(map[string]any)["groups"].([]any)[0].(map[string]any)
		group[field] = nil
		invalid, _ := json.Marshal(object)
		if _, err := Decode(strings.NewReader(string(invalid))); err == nil {
			t.Fatal("null mood group field accepted", field)
		}
	}
}

func TestMoodStoreRoundTripAndIsolation(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	c := moodRig()
	if err := store.Save(c); err != nil {
		t.Fatal(err)
	}
	c.Moods[0].Groups[0].Palette[0] = "#000000"
	copy := store.Get()
	if copy.Moods[0].Groups[0].Palette[0] != "#ff8000" {
		t.Fatal("save retained caller's palette")
	}
	copy.Moods[0].Groups[0].Palette[0] = "#000000"
	if store.Get().Moods[0].Groups[0].Palette[0] != "#ff8000" {
		t.Fatal("get leaked palette references")
	}
}
