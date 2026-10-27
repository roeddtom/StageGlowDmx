package config

import (
	"math"
	"strings"
	"testing"
)

func rig() Config {
	c := Defaults()
	c.Profiles = []Profile{{
		ID: "rgb", Name: "RGB", Footprint: 3,
		Channels: []Channel{{Offset: 0, Kind: "red"}, {Offset: 1, Kind: "green"}, {Offset: 2, Kind: "blue"}},
		WhiteMix: 1, AmberMix: 0.5,
	}}
	c.Groups = []Group{{ID: "front", Name: "Front"}}
	c.Loadouts = []Loadout{{
		ID: "small", Name: "Small",
		Fixtures: []Fixture{{ID: "one", Name: "One", ProfileID: "rgb", StartAddress: 510, GroupID: "front"}},
	}}
	c.ActiveLoadoutID = "small"
	c.ArtNet = ArtNet{DestinationIP: "192.168.1.50", PortAddress: 32767}
	return c
}

func TestValidation(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Config)
	}{
		{"schema", func(c *Config) { c.SchemaVersion = 2 }},
		{"null arrays", func(c *Config) { c.Groups = nil }},
		{"blank identity", func(c *Config) { c.Profiles[0].Name = "  " }},
		{"duplicate profile", func(c *Config) { c.Profiles = append(c.Profiles, c.Profiles[0]) }},
		{"duplicate group", func(c *Config) { c.Groups = append(c.Groups, c.Groups[0]) }},
		{"duplicate loadout", func(c *Config) { c.Loadouts = append(c.Loadouts, c.Loadouts[0]) }},
		{"small footprint", func(c *Config) { c.Profiles[0].Footprint = 2 }},
		{"large footprint", func(c *Config) { c.Profiles[0].Footprint = 513 }},
		{"missing offset", func(c *Config) { c.Profiles[0].Channels = c.Profiles[0].Channels[:2] }},
		{"duplicate offset", func(c *Config) { c.Profiles[0].Channels[2].Offset = 1 }},
		{"negative offset", func(c *Config) { c.Profiles[0].Channels[0].Offset = -1 }},
		{"offset past end", func(c *Config) { c.Profiles[0].Channels[2].Offset = 3 }},
		{"unknown kind", func(c *Config) { c.Profiles[0].Channels[2].Kind = "strobe" }},
		{"duplicate colour", func(c *Config) { c.Profiles[0].Channels[2].Kind = "red" }},
		{"missing rgb", func(c *Config) { c.Profiles[0].Channels[2].Kind = "white" }},
		{"missing fixed value", func(c *Config) {
			c.Profiles[0].Footprint++
			c.Profiles[0].Channels = append(c.Profiles[0].Channels, Channel{Offset: 3, Kind: "fixed"})
		}},
		{"bad optional value", func(c *Config) { n := 256; c.Profiles[0].Channels[0].Value = &n }},
		{"bad blackout value", func(c *Config) { n := -1; c.Profiles[0].Channels[0].BlackoutValue = &n }},
		{"mix nan", func(c *Config) { c.Profiles[0].WhiteMix = math.NaN() }},
		{"mix infinity", func(c *Config) { c.Profiles[0].AmberMix = math.Inf(1) }},
		{"mix range", func(c *Config) { c.Profiles[0].WhiteMix = 1.01 }},
		{"unknown profile", func(c *Config) { c.Loadouts[0].Fixtures[0].ProfileID = "missing" }},
		{"unknown group", func(c *Config) { c.Loadouts[0].Fixtures[0].GroupID = "missing" }},
		{"duplicate fixture", func(c *Config) {
			c.Loadouts[0].Fixtures = append(c.Loadouts[0].Fixtures, c.Loadouts[0].Fixtures[0])
		}},
		{"overlap", func(c *Config) {
			f := c.Loadouts[0].Fixtures[0]
			f.ID, f.StartAddress = "two", 509
			c.Loadouts[0].Fixtures = append(c.Loadouts[0].Fixtures, f)
		}},
		{"out of bounds", func(c *Config) { c.Loadouts[0].Fixtures[0].StartAddress = 511 }},
		{"zero address", func(c *Config) { c.Loadouts[0].Fixtures[0].StartAddress = 0 }},
		{"null fixtures", func(c *Config) { c.Loadouts[0].Fixtures = nil }},
		{"active reference", func(c *Config) { c.ActiveLoadoutID = "missing" }},
		{"multicast", func(c *Config) { c.ArtNet.DestinationIP = "239.1.2.3" }},
		{"broadcast", func(c *Config) { c.ArtNet.DestinationIP = "255.255.255.255" }},
		{"unspecified", func(c *Config) { c.ArtNet.DestinationIP = "0.0.0.0" }},
		{"ipv6", func(c *Config) { c.ArtNet.DestinationIP = "::1" }},
		{"hostname", func(c *Config) { c.ArtNet.DestinationIP = "node.local" }},
		{"port address", func(c *Config) { c.ArtNet.PortAddress = 32768 }},
		{"bpm low", func(c *Config) { c.BPM = 19 }},
		{"bpm high", func(c *Config) { c.BPM = 301 }},
		{"bpm nan", func(c *Config) { c.BPM = math.NaN() }},
	}
	if err := rig().Validate(); err != nil {
		t.Fatal(err)
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			c := rig()
			test.edit(&c)
			if err := c.Validate(); err == nil {
				t.Fatal("invalid configuration accepted")
			}
		})
	}
}

func TestOptionalChannelsAndPatchBoundaries(t *testing.T) {
	c := rig()
	value := 255
	for _, kind := range []string{"white", "amber", "uv", "dimmer", "fixed", "fixed"} {
		p := &c.Profiles[0]
		p.Channels = append(p.Channels, Channel{Offset: p.Footprint, Kind: kind, Value: &value, BlackoutValue: &value})
		p.Footprint++
	}
	c.Loadouts[0].Fixtures[0].StartAddress = 1
	f := c.Loadouts[0].Fixtures[0]
	f.ID, f.StartAddress = "two", 504
	c.Loadouts[0].Fixtures = append(c.Loadouts[0].Fixtures, f)
	c.ActiveLoadoutID = ""
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	c.Profiles[0].Channels[8].Kind = "white"
	if err := c.Validate(); err == nil {
		t.Fatal("duplicate optional channel accepted")
	}
}

func TestDecodeStrict(t *testing.T) {
	data, _ := encode(Defaults())
	for _, text := range []string{
		string(data) + "{}",
		string(data) + "garbage",
		strings.Replace(string(data), `"bpm": 120`, `"bpm": 120, "armed": true`, 1),
		strings.Replace(string(data), `"destinationIp": ""`, `"destinationIp": "", "unknown": 1`, 1),
		strings.Replace(string(data), `"destinationIp": ""`, `"destinationIp": null`, 1),
		strings.Replace(string(data), `"portAddress": 0`, `"portAddress": null`, 1),
		strings.Replace(string(data), `"activeLoadoutId": "",`, ``, 1),
		"null",
	} {
		if _, err := Decode(strings.NewReader(text)); err == nil {
			t.Errorf("accepted invalid JSON: %s", text)
		}
	}
}
