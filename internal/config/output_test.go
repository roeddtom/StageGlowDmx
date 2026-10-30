package config

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestOutputSettingsAndLegacyBackups(t *testing.T) {
	c := Defaults()
	for _, mode := range []string{"enttec", "eurolite-mk2"} {
		for _, path := range []string{"", "COM1", "COM23", "/dev/ttyUSB0", "/dev/ttyACM12", "/dev/serial/by-id/usb-ENTTEC_DMX_USB_PRO_123-if00-port0"} {
			c.Output = Output{Mode: mode, SerialPort: path}
			if err := c.Validate(); err != nil {
				t.Fatal("valid output selection rejected", mode, path, err)
			}
		}
	}
	for _, path := range []string{"COM0", "com1", "/etc/passwd", "/dev/serial/by-id/../ttyUSB0", "/dev/tty", "COM1\n", "/dev/serial/by-id/.."} {
		c.Output.SerialPort = path
		if err := c.Validate(); err == nil {
			t.Fatal("unsafe or malformed serial path accepted", path)
		}
	}
	c = Defaults()
	c.Output.Mode = "unknown"
	if c.Validate() == nil {
		t.Fatal("unknown mode accepted")
	}
	c = Defaults()
	data, _ := json.Marshal(c)
	var document map[string]any
	json.Unmarshal(data, &document)
	delete(document, "output")
	legacy, _ := json.Marshal(document)
	decoded, err := Decode(strings.NewReader(string(legacy)))
	if err != nil || decoded.Output != (Output{Mode: "simulation"}) {
		t.Fatal("legacy backup did not default to simulation", decoded.Output, err)
	}
	for _, value := range []string{`null`, `{}`, `{"mode":"enttec"}`, `{"mode":"enttec","serialPort":null}`, `{"mode":"simulation","serialPort":"","enabled":true}`} {
		document["output"] = json.RawMessage(value)
		invalid, _ := json.Marshal(document)
		if _, err := Decode(strings.NewReader(string(invalid))); err == nil {
			t.Fatal("malformed output config accepted", value)
		}
	}
}
