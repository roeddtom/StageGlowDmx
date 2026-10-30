package output

import (
	"context"
	"fmt"

	"go.bug.st/serial"
)

// The MK2 uses the same label-6 framing as USB PRO, but at 250000 baud
// and without its parameter-query handshake. It must be selected explicitly.
type euroliteMK2 struct{ *enttec }

func OpenEuroliteMK2(name string) (Device, error) {
	port, err := serial.Open(name, &serial.Mode{
		BaudRate: 250000, DataBits: 8, Parity: serial.NoParity, StopBits: serial.OneStopBit,
	})
	if err != nil {
		return nil, fmt.Errorf("open Eurolite MK2 port (check cable, driver, permissions and other lighting apps): %w", err)
	}
	return &euroliteMK2{&enttec{port: port}}, nil
}

func (d *euroliteMK2) initialize(ctx context.Context) error {
	return ctx.Err()
}
