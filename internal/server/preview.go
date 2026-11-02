package server

import (
	"bytes"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"time"

	"stageglowdmx/internal/config"
	"stageglowdmx/internal/engine"
)

// Each frame uses an isolated engine. Preview requests cannot reach USB or alter
// the live engine, saved settings, selections, master level or blackout.
func previewEffect(w http.ResponseWriter, r *http.Request, c config.Config) {
	var request struct {
		Mood         json.RawMessage `json:"mood"`
		BaseID       string          `json:"baseMoodId"`
		AtmosphereID string          `json:"atmosphereMoodId"`
		Elapsed      *float64        `json:"elapsedSeconds"`
		ReleaseAt    *float64        `json:"releaseAtSeconds,omitempty"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, MaxConfigBytes)
	defer r.Body.Close()
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(&request); err != nil {
		fail(w, http.StatusBadRequest, "invalid effect preview request")
		return
	}
	if err := d.Decode(new(any)); err != io.EOF {
		fail(w, http.StatusBadRequest, "preview requires exactly one JSON object")
		return
	}
	validTime := func(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0 && v <= 3600 }
	if request.Elapsed == nil || !validTime(*request.Elapsed) ||
		(request.ReleaseAt != nil && (!validTime(*request.ReleaseAt) || *request.ReleaseAt > *request.Elapsed)) {
		fail(w, http.StatusBadRequest, "preview elapsed/release times must be 0-3600 seconds, with release no later than elapsed")
		return
	}
	// Decode through the normal configuration path so drafts receive the same
	// strict field and group validation as saved effects, without saving them.
	var mood config.Mood
	if err := json.Unmarshal(request.Mood, &mood); err != nil || !mood.IsEffect() {
		fail(w, http.StatusBadRequest, "preview requires an effect mood")
		return
	}
	data, _ := json.Marshal(c)
	var document map[string]json.RawMessage
	_ = json.Unmarshal(data, &document)
	var moods []json.RawMessage
	_ = json.Unmarshal(document["moods"], &moods)
	found := false
	for i := range c.Moods {
		if c.Moods[i].ID == mood.ID {
			moods[i], found = request.Mood, true
		}
	}
	if !found {
		moods = append(moods, request.Mood)
	}
	document["moods"], _ = json.Marshal(moods)
	data, _ = json.Marshal(document)
	preview, err := config.Decode(bytes.NewReader(data))
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	for _, selection := range []struct {
		id   string
		base bool
	}{{request.BaseID, true}, {request.AtmosphereID, false}} {
		if selection.id == "" {
			continue
		}
		valid := false
		for i := range preview.Moods {
			m := &preview.Moods[i]
			if m.ID == selection.id && !m.IsEffect() && m.IsBase() == selection.base {
				valid = true
				m.CrossfadeSeconds = 0
			}
		}
		if !valid {
			fail(w, http.StatusBadRequest, "preview background must reference a saved Base or Atmosphere of the matching type")
			return
		}
	}
	// A temporary empty Base allows an explicit effect-only preview from darkness
	// without weakening the live engine's blackout-release requirements.
	darkID := "_preview-dark"
	ids := make(map[string]bool)
	for _, m := range preview.Moods {
		ids[m.ID] = true
	}
	for ids[darkID] {
		darkID += "_"
	}
	preview.Moods = append(preview.Moods, config.Mood{ID: darkID, Name: "Preview darkness", Type: "base", BeatsPerChange: 8, Smoothness: 1, Groups: []config.MoodGroup{}})
	start := time.Unix(1000, 0)
	now := start
	isolated := engine.New(preview, func() time.Time { return now }, 42)
	_ = isolated.Select(darkID)
	if request.AtmosphereID != "" {
		_ = isolated.Select(request.AtmosphereID)
	}
	_ = isolated.Blackout(false)
	isolated.ClearBase()
	if request.BaseID != "" {
		_ = isolated.Select(request.BaseID)
	}
	if err := isolated.TriggerEffect(mood.ID); err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	if request.ReleaseAt != nil {
		now = start.Add(time.Duration(*request.ReleaseAt * float64(time.Second)))
		isolated.StopEffect(mood.ID)
	}
	now = start.Add(time.Duration(*request.Elapsed * float64(time.Second)))
	writeJSON(w, http.StatusOK, isolated.State())
}
