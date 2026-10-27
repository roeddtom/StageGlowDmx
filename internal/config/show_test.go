package config

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestShowLayoutValidationAndCompatibility(t *testing.T) {
	c := Defaults()
	c.Moods = []Mood{{ID: "pause", Name: "Pause", BeatsPerChange: 8, Smoothness: 1, Groups: []MoodGroup{}}}
	c.Show.Slots[0], c.Show.PauseMoodID = "pause", "pause"
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	copy := c.Copy()
	copy.Show.Slots[0] = ""
	if c.Show.Slots[0] != "pause" {
		t.Fatal("copy shares show assignments")
	}
	dir := testDirectory(t)
	store, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(c); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(dir)
	if err != nil || reopened.Get().Show.Slots[0] != "pause" || reopened.Get().Show.PauseMoodID != "pause" {
		t.Fatal("show layout did not survive restart", err)
	}
	data, _ := json.Marshal(c)
	var document map[string]json.RawMessage
	json.Unmarshal(data, &document)
	delete(document, "show")
	data, _ = json.Marshal(document)
	legacy, err := Decode(strings.NewReader(string(data)))
	if err != nil || len(legacy.Show.Slots) != 8 || legacy.Show.PauseMoodID != "" || legacy.Show.Slots[0] != "" {
		t.Fatal("legacy backup must default to eight unassigned slots", err)
	}
	for _, show := range []string{
		`null`, `{}`, `{"slots":null,"pauseMoodId":""}`, `{"slots":[],"pauseMoodId":""}`,
		`{"slots":["","","","","","","",""],"pauseMoodId":null}`,
		`{"slots":["","","","","","","","missing"],"pauseMoodId":""}`,
		`{"slots":["","","","","","","",""],"pauseMoodId":"missing"}`,
		`{"slots":[null,"","","","","","",""],"pauseMoodId":""}`,
		`{"slots":["","","","","","","",""],"pauseMoodId":"","enabled":true}`,
	} {
		document["show"] = json.RawMessage(show)
		data, _ = json.Marshal(document)
		if _, err := Decode(strings.NewReader(string(data))); err == nil {
			t.Fatal("invalid show layout accepted", show)
		}
	}
	for _, count := range []int{7, 8, 10, 12, 13} {
		c.Show.Slots = make([]string, count)
		if err := c.Validate(); (err == nil) != (count >= 8 && count <= 12) {
			t.Fatal("incorrect slot limit", count, err)
		}
	}
}
