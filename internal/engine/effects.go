package engine

import (
	"fmt"
	"math"
	"time"
)

const holdEffectLimit = 30 * time.Second

type effectLayer struct {
	active          string
	start           float64
	at              time.Time
	releasing       bool
	releaseAt       time.Time
	releaseStrength float64
}

// TriggerEffect never releases blackout or changes either underlying mood clock.
func (e *Engine) TriggerEffect(id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	mood := e.mood(id)
	if mood == nil || !mood.IsEffect() {
		return fmt.Errorf("select an existing effect")
	}
	if e.blackout {
		return fmt.Errorf("release blackout with a Base or Atmosphere before triggering effects")
	}
	now := e.advance()
	e.effectStrength(now)
	if mood.Effect.Activation == "toggle" && e.effect.active == id && !e.effect.releasing {
		e.stopEffect(now)
		return nil
	}
	e.effect = effectLayer{active: id, start: e.beats, at: now}
	return nil
}

// Matching IDs prevent a delayed hold release from stopping a different effect.
func (e *Engine) StopEffect(id string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	now := e.advance()
	if id == "" || e.effect.active == id {
		e.stopEffect(now)
	}
}

func (e *Engine) stopEffect(now time.Time) {
	strength := e.effectStrength(now)
	if e.effect.active == "" || e.effect.releasing {
		return
	}
	e.effect.releasing, e.effect.releaseAt, e.effect.releaseStrength = true, now, strength
}

func (e *Engine) effectStrength(now time.Time) float64 {
	mood := e.mood(e.effect.active)
	if mood == nil || !mood.IsEffect() || e.blackout {
		e.effect = effectLayer{}
		return 0
	}
	fx := mood.Effect
	if fx.Activation == "hold" && now.Sub(e.effect.at) >= holdEffectLimit {
		e.effect = effectLayer{}
		return 0
	}
	if e.effect.releasing {
		fade := fadeProgress(now, e.effect.releaseAt, mood.CrossfadeSeconds)
		if fade >= 1 {
			e.effect = effectLayer{}
			return 0
		}
		return e.effect.releaseStrength * (1 - fade)
	}
	elapsed := math.Max(0, e.beats-e.effect.start)
	progress := elapsed / fx.DurationBeats
	timed := fx.Activation == "timed"
	if timed && progress >= 1 {
		e.effect = effectLayer{}
		return 0
	}
	switch fx.Pattern {
	case "swell":
		if !timed {
			return glide(math.Min(1, progress*2), 1)
		}
		return math.Pow(math.Sin(math.Pi*progress), 2)
	case "accent":
		attack := math.Min(1, elapsed/0.1)
		if timed {
			return attack * math.Pow(1-progress, 2)
		}
		return attack
	case "pulse":
		strength := math.Pow(math.Sin(math.Pi*elapsed/mood.BeatsPerChange), 2)
		if timed {
			strength *= math.Min(1, (1-progress)/0.2)
		}
		return strength
	default: // Solo highlight.
		strength := math.Min(1, progress/0.1)
		if timed {
			strength = math.Min(strength, (1-progress)/0.2)
		}
		return strength
	}
}

func (e *Engine) applyEffect(values map[string]light, now time.Time) {
	strength := e.effectStrength(now)
	if strength <= 0 {
		return
	}
	mood := e.mood(e.effect.active)
	for _, loadout := range e.cfg.Loadouts {
		if loadout.ID != e.cfg.ActiveLoadoutID {
			continue
		}
		for _, fixture := range loadout.Fixtures {
			for _, group := range mood.Groups {
				if group.GroupID != fixture.GroupID {
					continue
				}
				value := values[fixture.ID]
				intensity := lerp(value.Intensity, group.IntensityMax, strength)
				if mood.Effect.OverrideColour {
					colour := parseColor(group.Palette[0])
					for i := range value.RGB {
						value.RGB[i] = lerp(value.RGB[i], colour[i]*group.IntensityMax, strength)
					}
					value.UV = lerp(value.UV, group.UV*group.IntensityMax, strength)
				} else if value.Intensity > 0 {
					for i := range value.RGB {
						value.RGB[i] *= intensity / value.Intensity
					}
					value.UV *= intensity / value.Intensity
				}
				value.Intensity = intensity
				values[fixture.ID] = value
			}
		}
	}
}
