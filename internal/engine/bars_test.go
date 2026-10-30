package engine

import (
	"reflect"
	"testing"
	"time"

	"stageglowdmx/internal/config"
)

func TestLinkedBarRendering(t *testing.T) {
	for _, test := range []struct {
		name  string
		kinds []string
		want  []int
		color string
	}{
		{"Barwash", []string{"dimmer", "red", "green", "blue", "white", "fixed", "fixed", "fixed"}, []int{64, 255, 128, 0, 0, 0, 0, 0}, "#402000"},
		{"CBB RGB", []string{"dimmer", "fixed", "red", "green", "blue", "red", "green", "blue", "fixed", "fixed", "fixed"}, []int{64, 0, 255, 128, 0, 255, 128, 0, 0, 0, 0}, "#402000"},
		{"CBB WW", []string{"dimmer", "fixed", "white", "white", "fixed"}, []int{64, 0, 255, 255, 0}, "#404040"},
		{"White without master", []string{"white", "white", "fixed"}, []int{64, 64, 0}, "#404040"},
	} {
		t.Run(test.name, func(t *testing.T) {
			c := testRig()
			p := &c.Profiles[0]
			p.Name, p.Footprint, p.Channels = test.name, len(test.kinds), nil
			zero := 0
			for offset, kind := range test.kinds {
				ch := config.Channel{Offset: offset, Kind: kind}
				if kind == "fixed" {
					ch.Value, ch.BlackoutValue = &zero, &zero
				}
				p.Channels = append(p.Channels, ch)
			}
			c.Loadouts[0].Fixtures = c.Loadouts[0].Fixtures[:1]
			e, _ := testEngine(t, c)
			e.SetMaster(0.5)
			s := e.State()
			if !reflect.DeepEqual(s.Fixtures[0].Channels, test.want) || s.Fixtures[0].Color != test.color {
				t.Fatal("linked emitters, master scaling or preview incorrect", s.Fixtures[0])
			}
			e.Blackout(true)
			for _, value := range e.State().DMX {
				if value != 0 {
					t.Fatal("blackout must darken every channel")
				}
			}
		})
	}
}

func TestWhiteOnlyIgnoresPaletteAndFadesIntensityOnce(t *testing.T) {
	c := testRig()
	zero := 0
	c.Profiles[0].Footprint = 5
	c.Profiles[0].Channels = []config.Channel{
		{Offset: 0, Kind: "dimmer"}, {Offset: 1, Kind: "fixed", Value: &zero, BlackoutValue: &zero},
		{Offset: 2, Kind: "white"}, {Offset: 3, Kind: "white"}, {Offset: 4, Kind: "fixed", Value: &zero, BlackoutValue: &zero},
	}
	c.Loadouts[0].Fixtures = c.Loadouts[0].Fixtures[:1]
	c.Moods[0].CrossfadeSeconds = 2
	c.Moods[0].Groups[0].Palette = []string{"#000000"}
	e, now := testEngine(t, c)
	*now = now.Add(time.Second)
	s := e.State()
	if s.DMX[0] != 64 || s.DMX[2] != 255 || s.DMX[3] != 255 || s.Fixtures[0].Color != "#404040" {
		t.Fatal("white-only must ignore palette and apply fade only through master dimmer", s.Fixtures[0])
	}
}
