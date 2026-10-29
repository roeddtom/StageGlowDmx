// Package engine generates simulated lighting frames; it never sends network output.
package engine

import (
	"fmt"
	"hash/fnv"
	"math"
	"reflect"
	"strconv"
	"sync"
	"time"

	"stageglowdmx/internal/config"
)

type light struct {
	RGB       [3]float64
	Intensity float64
	UV        float64
	coverage  float64
}

type moodLayer struct {
	active   string
	start    float64
	fadeFrom map[string]light
	fadeAt   time.Time
	fadeFor  float64
}

type FixtureState struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	GroupID       string  `json:"groupId"`
	StartAddress  int     `json:"startAddress"`
	Color         string  `json:"color"`
	Intensity     float64 `json:"intensity"`
	UV            float64 `json:"uv"`
	UnsupportedUV bool    `json:"unsupportedUV"`
	Channels      []int   `json:"channels"`
}

type State struct {
	Blackout         bool           `json:"blackout"`
	OutputEnabled    bool           `json:"outputEnabled"`
	EngineStatus     string         `json:"engineStatus"`
	ActiveMoodID     string         `json:"activeMoodId"`
	ActiveBaseMoodID string         `json:"activeBaseMoodId"`
	BPM              float64        `json:"bpm"`
	Master           float64        `json:"master"`
	Fixtures         []FixtureState `json:"fixtures"`
	DMX              [512]int       `json:"dmx"`
}

type Engine struct {
	mu         sync.Mutex
	cfg        config.Config
	clock      func() time.Time
	last       time.Time
	epoch      time.Time
	anchor     float64
	beats      float64
	seed       uint64
	blackout   bool
	master     float64
	atmosphere moodLayer
	base       moodLayer
	restoreAt  time.Time
	restoreFor float64
}

func New(c config.Config, clock func() time.Time, seed int64) *Engine {
	if clock == nil {
		clock = time.Now
	}
	now := clock()
	return &Engine{
		cfg: c.Copy(), clock: clock, last: now, epoch: now, seed: uint64(seed),
		blackout: true, master: 1,
	}
}

// Configure preserves the current clock for unrelated saves. Patch changes force
// blackout; changes to the selected mood crossfade from the rendered state.
func (e *Engine) Configure(c config.Config) {
	e.mu.Lock()
	defer e.mu.Unlock()
	now := e.advance()
	beforeAtmosphere := e.layerLights(e.atmosphere, now)
	beforeBase := e.layerLights(e.base, now)
	patchChanged := e.cfg.ActiveLoadoutID != c.ActiveLoadoutID ||
		!reflect.DeepEqual(e.cfg.Loadouts, c.Loadouts) || !reflect.DeepEqual(e.cfg.Profiles, c.Profiles)
	oldMood, oldBase := e.mood(e.atmosphere.active), e.mood(e.base.active)
	if c.BPM != e.cfg.BPM {
		e.anchor, e.epoch = e.beats, now
	}
	e.cfg = c.Copy()
	nextMood, nextBase := e.mood(e.atmosphere.active), e.mood(e.base.active)
	if patchChanged {
		e.blackout = true
		e.atmosphere.fadeFrom, e.base.fadeFrom = nil, nil
		e.atmosphere.fadeFor, e.base.fadeFor = 0, 0
	}
	if nextMood == nil || nextMood.IsBase() {
		if e.atmosphere.active != "" {
			e.blackout = true
		}
		e.atmosphere = moodLayer{}
	} else if !reflect.DeepEqual(oldMood, nextMood) {
		e.atmosphere.start = e.beats
		e.atmosphere.fadeFrom, e.atmosphere.fadeAt, e.atmosphere.fadeFor = beforeAtmosphere, now, nextMood.CrossfadeSeconds
	}
	if nextBase == nil || !nextBase.IsBase() {
		if e.base.active != "" {
			e.blackout = true
			e.base = moodLayer{}
		}
	} else if !reflect.DeepEqual(oldBase, nextBase) {
		e.base.fadeFrom, e.base.fadeAt, e.base.fadeFor = beforeBase, now, nextBase.CrossfadeSeconds
	}
}

// Select updates the matching layer without releasing blackout or restarting the other.
func (e *Engine) Select(id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	var selected *config.Mood
	for i := range e.cfg.Moods {
		if e.cfg.Moods[i].ID == id {
			selected = &e.cfg.Moods[i]
			break
		}
	}
	if selected == nil {
		return fmt.Errorf("select an existing saved mood")
	}
	now := e.advance()
	layer := &e.atmosphere
	if selected.IsBase() {
		layer = &e.base
	}
	layer.fadeFrom = e.layerLights(*layer, now)
	layer.fadeAt, layer.fadeFor = now, selected.CrossfadeSeconds
	layer.active, layer.start = id, e.beats
	return nil
}

func (e *Engine) ClearBase() {
	e.mu.Lock()
	defer e.mu.Unlock()
	now := e.advance()
	duration := e.base.fadeFor
	if mood := e.mood(e.base.active); mood != nil {
		duration = mood.CrossfadeSeconds
	}
	e.base.fadeFrom = e.layerLights(e.base, now)
	e.base.fadeAt, e.base.fadeFor, e.base.active = now, duration, ""
}

func (e *Engine) Blackout(enabled bool) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	now := e.advance()
	mood, base := e.mood(e.atmosphere.active), e.mood(e.base.active)
	if !enabled && mood == nil && base == nil {
		return fmt.Errorf("select a mood before releasing simulation blackout")
	}
	if e.blackout && !enabled {
		e.atmosphere.fadeFor, e.base.fadeFor = 0, 0
		e.atmosphere.fadeFrom, e.base.fadeFrom = nil, nil
		e.restoreAt, e.restoreFor = now, 0
		if mood != nil {
			e.restoreFor = mood.CrossfadeSeconds
		}
		if base != nil {
			e.restoreFor = math.Max(e.restoreFor, base.CrossfadeSeconds)
		}
	}
	e.blackout = enabled
	return nil
}

func (e *Engine) SetMaster(value float64) error {
	if !finite(value, 0, 1) {
		return fmt.Errorf("master must be a finite number from 0 to 1")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.master = value
	return nil
}

func (e *Engine) State() State {
	e.mu.Lock()
	defer e.mu.Unlock()
	now := e.advance()
	values := e.lights(now)
	state := State{
		Blackout: e.blackout, EngineStatus: "simulation", ActiveMoodID: e.atmosphere.active, ActiveBaseMoodID: e.base.active,
		BPM: e.cfg.BPM, Master: e.master, Fixtures: []FixtureState{},
	}
	profiles := make(map[string]config.Profile, len(e.cfg.Profiles))
	for _, profile := range e.cfg.Profiles {
		profiles[profile.ID] = profile
	}
	for _, loadout := range e.cfg.Loadouts {
		if loadout.ID != e.cfg.ActiveLoadoutID {
			continue
		}
		for _, fixture := range loadout.Fixtures {
			profile := profiles[fixture.ProfileID]
			value := values[fixture.ID]
			channels, unsupported := render(profile, value, e.master, e.blackout)
			display := value
			for i := range display.RGB {
				display.RGB[i] *= e.master
			}
			display.Intensity *= e.master
			display.UV *= e.master
			if e.blackout {
				display = light{}
			}
			state.Fixtures = append(state.Fixtures, FixtureState{
				ID: fixture.ID, Name: fixture.Name, GroupID: fixture.GroupID,
				StartAddress: fixture.StartAddress, Color: colorHex(display.RGB),
				Intensity: display.Intensity, UV: display.UV,
				UnsupportedUV: unsupported, Channels: channels,
			})
			for offset, value := range channels {
				state.DMX[fixture.StartAddress-1+offset] = value
			}
		}
	}
	return state
}

func (e *Engine) advance() time.Time {
	now := e.clock()
	if now.Before(e.last) {
		return e.last
	}
	e.beats = e.anchor + now.Sub(e.epoch).Seconds()*e.cfg.BPM/60
	e.last = now
	return now
}

func (e *Engine) mood(id string) *config.Mood {
	for i := range e.cfg.Moods {
		if e.cfg.Moods[i].ID == id {
			return &e.cfg.Moods[i]
		}
	}
	return nil
}

func (e *Engine) lights(now time.Time) map[string]light {
	base := e.layerLights(e.base, now)
	atmosphere := e.layerLights(e.atmosphere, now)
	restore := fadeProgress(now, e.restoreAt, e.restoreFor)
	for id, value := range atmosphere {
		fallback := base[id]
		for i := range value.RGB {
			value.RGB[i] = (value.RGB[i] + fallback.RGB[i]*(1-value.coverage)) * restore
		}
		value.Intensity = (value.Intensity + fallback.Intensity*(1-value.coverage)) * restore
		value.UV = (value.UV + fallback.UV*(1-value.coverage)) * restore
		base[id] = value
	}
	return base
}

func fadeProgress(now time.Time, at time.Time, duration float64) float64 {
	if duration <= 0 {
		return 1
	}
	t := math.Min(1, math.Max(0, now.Sub(at).Seconds()/duration))
	return t * t * (3 - 2*t)
}

func (e *Engine) layerLights(layer moodLayer, now time.Time) map[string]light {
	result := make(map[string]light)
	mood := e.mood(layer.active)
	settings := make(map[string]config.MoodGroup)
	segment, blend := uint64(0), 0.0
	if mood != nil {
		for _, group := range mood.Groups {
			settings[group.GroupID] = group
		}
		progress := math.Max(0, e.beats-layer.start) / mood.BeatsPerChange
		segment = uint64(math.Floor(progress))
		blend = glide(progress-math.Floor(progress), mood.Smoothness)
	}
	fade := fadeProgress(now, layer.fadeAt, layer.fadeFor)
	for _, loadout := range e.cfg.Loadouts {
		if loadout.ID != e.cfg.ActiveLoadoutID {
			continue
		}
		for _, fixture := range loadout.Fixtures {
			var value light
			if group, ok := settings[fixture.GroupID]; ok {
				value.coverage = 1
				key := mood.ID + "\x00" + fixture.ID
				position := 0.0
				if group.ColorMode == "palette" {
					position = lerp(e.random(key, segment, 0), e.random(key, segment+1, 0), blend)
				}
				color := palette(group.Palette, position)
				intensity := lerp(group.IntensityMin, group.IntensityMax,
					lerp(e.random(key, segment, 1), e.random(key, segment+1, 1), blend))
				value.Intensity, value.UV = intensity, group.UV*intensity
				for i := range color {
					value.RGB[i] = color[i] * intensity
				}
			}
			previous := layer.fadeFrom[fixture.ID]
			for i := range value.RGB {
				value.RGB[i] = lerp(previous.RGB[i], value.RGB[i], fade)
			}
			value.Intensity = lerp(previous.Intensity, value.Intensity, fade)
			value.UV = lerp(previous.UV, value.UV, fade)
			value.coverage = lerp(previous.coverage, value.coverage, fade)
			result[fixture.ID] = value
		}
	}
	return result
}

// Hashing segment targets avoids catch-up loops and makes output independent of
// browser polling frequency, fixture order, and time spent without a client.
func (e *Engine) random(key string, segment, stream uint64) float64 {
	hash := fnv.New64a()
	_, _ = hash.Write([]byte(key))
	x := hash.Sum64() ^ e.seed ^ (segment * 0x9e3779b97f4a7c15) ^ (stream * 0xd1b54a32d192ed03)
	x = (x ^ (x >> 30)) * 0xbf58476d1ce4e5b9
	x = (x ^ (x >> 27)) * 0x94d049bb133111eb
	x ^= x >> 31
	return float64(x>>11) / (1 << 53)
}

func glide(phase, smoothness float64) float64 {
	t := math.Min(1, phase/smoothness)
	return t * t * (3 - 2*t)
}

func lerp(a, b, t float64) float64 { return a + (b-a)*t }

func finite(value, min, max float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= min && value <= max
}

func palette(colors []string, position float64) [3]float64 {
	location := position * float64(len(colors)-1)
	index := int(location)
	first := parseColor(colors[index])
	if index == len(colors)-1 {
		return first
	}
	second := parseColor(colors[index+1])
	for i := range first {
		first[i] = lerp(first[i], second[i], location-float64(index))
	}
	return first
}

func parseColor(color string) [3]float64 {
	value, _ := strconv.ParseUint(color[1:], 16, 24)
	return [3]float64{float64(value>>16) / 255, float64((value>>8)&255) / 255, float64(value&255) / 255}
}

func colorHex(color [3]float64) string {
	return fmt.Sprintf("#%02x%02x%02x", channel(color[0]), channel(color[1]), channel(color[2]))
}

func channel(value float64) int {
	return int(math.Round(math.Max(0, math.Min(1, value)) * 255))
}

func render(profile config.Profile, value light, master float64, blackout bool) ([]int, bool) {
	kinds := make(map[string]bool)
	for _, ch := range profile.Channels {
		kinds[ch.Kind] = true
	}
	rgb := value.RGB
	uv := value.UV
	// With a dimmer, put brightness there once, not on both the colour and dimmer.
	if kinds["dimmer"] && value.Intensity > 0 {
		for i := range rgb {
			rgb[i] /= value.Intensity
		}
		uv /= value.Intensity
	}
	white, amber := 0.0, 0.0
	if kinds["white"] {
		white = math.Min(rgb[0], math.Min(rgb[1], rgb[2])) * profile.WhiteMix
		for i := range rgb {
			rgb[i] -= white
		}
	}
	// Approximate an amber emitter as full red plus half green.
	if kinds["amber"] {
		amber = math.Min(rgb[0], rgb[1]*2) * profile.AmberMix
		rgb[0] -= amber
		rgb[1] -= amber * 0.5
	}
	scale := master
	if kinds["dimmer"] {
		scale = 1
	}
	mapped := map[string]float64{
		"red": rgb[0] * scale, "green": rgb[1] * scale, "blue": rgb[2] * scale,
		"white": white * scale, "amber": amber * scale, "uv": uv * scale,
		"dimmer": value.Intensity * master,
	}
	channels := make([]int, profile.Footprint)
	for _, ch := range profile.Channels {
		if ch.Kind == "fixed" {
			if blackout && ch.BlackoutValue != nil {
				channels[ch.Offset] = *ch.BlackoutValue
			} else {
				channels[ch.Offset] = *ch.Value
			}
		} else if !blackout {
			channels[ch.Offset] = channel(mapped[ch.Kind])
		}
	}
	return channels, value.UV > 0 && !kinds["uv"]
}
