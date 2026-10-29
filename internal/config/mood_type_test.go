package config

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMoodTypesAndStaticBaseValidation(t *testing.T) {
	c := Defaults()
	c.Groups = []Group{{ID: "front", Name: "Front"}}
	mood := Mood{ID: "band", Name: "Band", BeatsPerChange: 8, Smoothness: 1,
		Groups: []MoodGroup{{GroupID: "front", ColorMode: "static", Palette: []string{"#ffbb66"}, IntensityMin: 0.5, IntensityMax: 0.5}}}
	c.Moods = []Mood{mood}
	data, _ := json.Marshal(c)
	legacy, err := Decode(strings.NewReader(string(data)))
	if err != nil || legacy.Moods[0].IsBase() {
		t.Fatal("missing type must remain atmosphere", err)
	}
	for _, kind := range []string{"", "atmosphere", "base"} {
		c.Moods[0].Type = kind
		if err := c.Validate(); err != nil {
			t.Fatal("valid type rejected", kind, err)
		}
	}
	for _, change := range []func(*Config){
		func(c *Config) { c.Moods[0].Type = "other" },
		func(c *Config) { c.Moods[0].Groups[0].ColorMode = "palette" },
		func(c *Config) { c.Moods[0].Groups[0].Palette = []string{"#ffffff", "#000000"} },
		func(c *Config) { c.Moods[0].Groups[0].IntensityMax = 1 },
		func(c *Config) { c.Show.PauseMoodID = "band" },
	} {
		next := c.Copy()
		change(&next)
		if next.Validate() == nil {
			t.Fatal("invalid base accepted")
		}
	}
	c.Show.Slots[0] = "band"
	data, _ = json.Marshal(c)
	if _, err := Decode(strings.NewReader(string(data))); err != nil {
		t.Fatal("base slot did not round-trip", err)
	}
	data = []byte(strings.Replace(string(data), `"type":"base"`, `"type":null`, 1))
	if _, err := Decode(strings.NewReader(string(data))); err == nil {
		t.Fatal("null type accepted")
	}
}
