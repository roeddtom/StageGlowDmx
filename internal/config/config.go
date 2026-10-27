package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"strings"
)

const SchemaVersion = 1

type Channel struct {
	Offset        int    `json:"offset"`
	Kind          string `json:"kind"`
	Value         *int   `json:"value,omitempty"`
	BlackoutValue *int   `json:"blackoutValue,omitempty"`
}

type Profile struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Footprint int       `json:"footprint"`
	Channels  []Channel `json:"channels"`
	WhiteMix  float64   `json:"whiteMix"`
	AmberMix  float64   `json:"amberMix"`
}

type Group struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Fixture struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	ProfileID    string `json:"profileId"`
	StartAddress int    `json:"startAddress"`
	GroupID      string `json:"groupId"`
}

type Loadout struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Fixtures []Fixture `json:"fixtures"`
}

type ArtNet struct {
	DestinationIP string `json:"destinationIp"`
	PortAddress   int    `json:"portAddress"`
}

type Config struct {
	SchemaVersion   int       `json:"schemaVersion"`
	Profiles        []Profile `json:"profiles"`
	Groups          []Group   `json:"groups"`
	Loadouts        []Loadout `json:"loadouts"`
	ActiveLoadoutID string    `json:"activeLoadoutId"`
	ArtNet          ArtNet    `json:"artNet"`
	BPM             float64   `json:"bpm"`
}

func Defaults() Config {
	return Config{
		SchemaVersion: SchemaVersion,
		Profiles:      []Profile{},
		Groups:        []Group{},
		Loadouts:      []Loadout{},
		BPM:           120,
	}
}

type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }

func invalid(format string, args ...any) error {
	return &ValidationError{Message: fmt.Sprintf(format, args...)}
}

// Decode accepts one complete configuration, never a partial update.
func Decode(r io.Reader) (Config, error) {
	var c Config
	d := json.NewDecoder(r)
	var raw json.RawMessage
	if err := d.Decode(&raw); err != nil {
		return c, fmt.Errorf("decode configuration: %w", err)
	}
	var extra any
	if err := d.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return c, errors.New("configuration must contain exactly one JSON value")
		}
		return c, fmt.Errorf("trailing configuration data: %w", err)
	}
	strict := json.NewDecoder(bytes.NewReader(raw))
	strict.DisallowUnknownFields()
	if err := strict.Decode(&c); err != nil {
		return c, fmt.Errorf("decode configuration: %w", err)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return c, fmt.Errorf("configuration must be an object: %w", err)
	}
	if err := required(object, "schemaVersion", "profiles", "groups", "loadouts", "activeLoadoutId", "artNet", "bpm"); err != nil {
		return c, err
	}
	for collection, fields := range map[string][]string{
		"profiles": {"id", "name", "footprint", "channels", "whiteMix", "amberMix"},
		"groups":   {"id", "name"},
		"loadouts": {"id", "name", "fixtures"},
	} {
		var items []map[string]json.RawMessage
		_ = json.Unmarshal(object[collection], &items)
		for _, item := range items {
			if err := required(item, fields...); err != nil {
				return c, err
			}
			nested, nestedFields := "channels", []string{"offset", "kind"}
			if collection == "loadouts" {
				nested, nestedFields = "fixtures", []string{"id", "name", "profileId", "startAddress", "groupId"}
			} else if collection != "profiles" {
				continue
			}
			var children []map[string]json.RawMessage
			_ = json.Unmarshal(item[nested], &children)
			for _, child := range children {
				if err := required(child, nestedFields...); err != nil {
					return c, err
				}
			}
		}
	}
	var artNet map[string]json.RawMessage
	_ = json.Unmarshal(object["artNet"], &artNet)
	if err := required(artNet, "destinationIp", "portAddress"); err != nil {
		return c, err
	}
	return c, c.Validate()
}

func required(object map[string]json.RawMessage, fields ...string) error {
	for _, field := range fields {
		if _, ok := object[field]; !ok {
			return invalid("configuration field %q is required", field)
		}
	}
	for field, value := range object {
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return invalid("configuration field %q must not be null", field)
		}
	}
	return nil
}

func identity(id, name, collection string, seen map[string]bool) error {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(name) == "" {
		return invalid("%s requires nonblank id and name", collection)
	}
	if seen[id] {
		return invalid("%s has duplicate id %q", collection, id)
	}
	seen[id] = true
	return nil
}

func unit(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0 && v <= 1 }

func (c Config) Validate() error {
	if c.SchemaVersion != SchemaVersion {
		return invalid("unsupported schemaVersion %d; this application supports version %d", c.SchemaVersion, SchemaVersion)
	}
	if c.Profiles == nil || c.Groups == nil || c.Loadouts == nil {
		return invalid("profiles, groups and loadouts must be arrays, not null or missing")
	}
	profiles := make(map[string]Profile)
	ids := make(map[string]bool)
	for _, p := range c.Profiles {
		if err := identity(p.ID, p.Name, "profiles", ids); err != nil {
			return err
		}
		if p.Footprint < 3 || p.Footprint > 512 || len(p.Channels) != p.Footprint {
			return invalid("profile %q must cover every channel of a footprint from 3 to 512", p.ID)
		}
		if !unit(p.WhiteMix) || !unit(p.AmberMix) {
			return invalid("profile %q mix values must be finite numbers from 0 to 1", p.ID)
		}
		offsets := make(map[int]bool)
		kinds := make(map[string]bool)
		for _, ch := range p.Channels {
			if ch.Offset < 0 || ch.Offset >= p.Footprint || offsets[ch.Offset] {
				return invalid("profile %q has an invalid or duplicate channel offset %d", p.ID, ch.Offset)
			}
			offsets[ch.Offset] = true
			switch ch.Kind {
			case "red", "green", "blue", "white", "amber", "uv", "dimmer":
				if kinds[ch.Kind] {
					return invalid("profile %q has duplicate %s channels", p.ID, ch.Kind)
				}
				kinds[ch.Kind] = true
			case "fixed":
				if ch.Value == nil {
					return invalid("profile %q fixed channel requires value", p.ID)
				}
			default:
				return invalid("profile %q has unsupported channel kind %q", p.ID, ch.Kind)
			}
			for _, value := range []*int{ch.Value, ch.BlackoutValue} {
				if value != nil && (*value < 0 || *value > 255) {
					return invalid("profile %q channel values must be from 0 to 255", p.ID)
				}
			}
		}
		if !kinds["red"] || !kinds["green"] || !kinds["blue"] {
			return invalid("profile %q requires one red, green and blue channel", p.ID)
		}
		profiles[p.ID] = p
	}
	groups := make(map[string]bool)
	for _, g := range c.Groups {
		if err := identity(g.ID, g.Name, "groups", groups); err != nil {
			return err
		}
	}
	loadouts := make(map[string]bool)
	for _, l := range c.Loadouts {
		if err := identity(l.ID, l.Name, "loadouts", loadouts); err != nil {
			return err
		}
		if l.Fixtures == nil {
			return invalid("loadout %q fixtures must be an array", l.ID)
		}
		fixtures := make(map[string]bool)
		var occupied [512]bool
		for _, f := range l.Fixtures {
			if err := identity(f.ID, f.Name, "fixtures in "+l.ID, fixtures); err != nil {
				return err
			}
			p, ok := profiles[f.ProfileID]
			if !ok || !groups[f.GroupID] {
				return invalid("fixture %q must reference an existing profile and group", f.ID)
			}
			if f.StartAddress < 1 || f.StartAddress > 512 || p.Footprint > 513-f.StartAddress {
				return invalid("fixture %q footprint must fit in addresses 1 to 512", f.ID)
			}
			for offset := 0; offset < p.Footprint; offset++ {
				address := f.StartAddress - 1 + offset
				if occupied[address] {
					return invalid("fixture %q overlaps another fixture at address %d", f.ID, address+1)
				}
				occupied[address] = true
			}
		}
	}
	if c.ActiveLoadoutID != "" && !loadouts[c.ActiveLoadoutID] {
		return invalid("activeLoadoutId must reference an existing loadout or be empty")
	}
	if c.ArtNet.DestinationIP != "" {
		ip := net.ParseIP(c.ArtNet.DestinationIP)
		if ip == nil || ip.To4() == nil || strings.Contains(c.ArtNet.DestinationIP, ":") ||
			(!ip.IsGlobalUnicast() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast()) || ip.To4()[0] == 0 {
			return invalid("artNet.destinationIp must be an IPv4 unicast address or empty")
		}
	}
	if c.ArtNet.PortAddress < 0 || c.ArtNet.PortAddress > 32767 {
		return invalid("artNet.portAddress must be from 0 to 32767")
	}
	if math.IsNaN(c.BPM) || math.IsInf(c.BPM, 0) || c.BPM < 20 || c.BPM > 300 {
		return invalid("bpm must be a finite number from 20 to 300")
	}
	return nil
}

func clone(c Config) Config {
	c.Profiles = append([]Profile{}, c.Profiles...)
	for i := range c.Profiles {
		c.Profiles[i].Channels = append([]Channel{}, c.Profiles[i].Channels...)
		for j := range c.Profiles[i].Channels {
			ch := &c.Profiles[i].Channels[j]
			if ch.Value != nil {
				value := *ch.Value
				ch.Value = &value
			}
			if ch.BlackoutValue != nil {
				value := *ch.BlackoutValue
				ch.BlackoutValue = &value
			}
		}
	}
	c.Groups = append([]Group{}, c.Groups...)
	c.Loadouts = append([]Loadout{}, c.Loadouts...)
	for i := range c.Loadouts {
		c.Loadouts[i].Fixtures = append([]Fixture{}, c.Loadouts[i].Fixtures...)
	}
	return c
}
