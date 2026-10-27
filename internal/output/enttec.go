// Package output sends already-rendered frames; it does not generate moods.
package output

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"go.bug.st/serial"
)

type Frame [512]byte

type Device interface {
	WriteFrame(context.Context, Frame) error
	Close(context.Context) error
}

type Opener func(string) (Device, error)

type serialPort interface {
	io.ReadWriteCloser
	SetReadTimeout(time.Duration) error
	ResetInputBuffer() error
	Drain() error
}

type enttec struct {
	port      serialPort
	mu        sync.Mutex
	operation chan struct{}
	closing   chan struct{}
	closeErr  error
}

// packet implements ENTTEC's DMX USB PRO API v1.44 (70304): a little-endian
// payload length delimited by 0x7e and 0xe7. Label 6 carries a start code + DMX.
func packet(label byte, data []byte) []byte {
	message := make([]byte, len(data)+5)
	message[0], message[1] = 0x7e, label
	binary.LittleEndian.PutUint16(message[2:4], uint16(len(data)))
	copy(message[4:], data)
	message[len(message)-1] = 0xe7
	return message
}

func writeAll(w io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := w.Write(data)
		if err != nil {
			return err
		}
		if n <= 0 || n > len(data) {
			return io.ErrShortWrite
		}
		data = data[n:]
	}
	return nil
}

func readPacket(ctx context.Context, r io.Reader, label byte) ([]byte, error) {
	read := func(b []byte) error {
		for len(b) > 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
			n, err := r.Read(b)
			if err != nil {
				return err
			}
			if n < 0 || n > len(b) {
				return io.ErrUnexpectedEOF
			}
			b = b[n:]
		}
		return nil
	}
	for {
		var start [1]byte
		if err := read(start[:]); err != nil {
			return nil, err
		}
		if start[0] != 0x7e {
			continue
		}
		var header [3]byte
		if err := read(header[:]); err != nil {
			return nil, err
		}
		size := int(binary.LittleEndian.Uint16(header[1:]))
		if size > 600 {
			return nil, errors.New("invalid ENTTEC packet length")
		}
		data := make([]byte, size+1)
		if err := read(data); err != nil {
			return nil, err
		}
		if data[size] != 0xe7 {
			return nil, errors.New("invalid ENTTEC packet terminator")
		}
		if header[0] == label {
			return data[:size], nil
		}
	}
}

func OpenENTTEC(name string) (Device, error) {
	// This is an FTDI virtual COM port, not raw DMX. ENTTEC documents baud as a
	// dummy value; 57600 8N1 is used by common USB PRO integrations.
	port, err := serial.Open(name, &serial.Mode{
		BaudRate: 57600, DataBits: 8, Parity: serial.NoParity, StopBits: serial.OneStopBit,
	})
	if err != nil {
		return nil, fmt.Errorf("open USB PRO port (check cable, driver, permissions and other lighting apps): %w", err)
	}
	return &enttec{port: port}, nil
}

func (d *enttec) initialize(ctx context.Context) error {
	return d.run(ctx, func() error {
		if err := d.port.SetReadTimeout(50 * time.Millisecond); err != nil {
			return err
		}
		if err := d.port.ResetInputBuffer(); err != nil {
			return err
		}
		// Read-only Get Widget Parameters (label 3, zero user-config bytes).
		// Do not change the device's persistent break/MAB/refresh settings.
		if err := writeAll(d.port, packet(3, []byte{0, 0})); err != nil {
			return err
		}
		params, err := readPacket(ctx, d.port, 3)
		if err != nil {
			return fmt.Errorf("no valid USB PRO response; Open DMX USB is unsupported: %w", err)
		}
		if len(params) != 5 || params[2] < 9 || params[2] > 127 ||
			params[3] < 1 || params[3] > 127 || params[4] > 40 {
			return errors.New("unsupported USB PRO widget parameters")
		}
		return nil
	})
}

func (d *enttec) WriteFrame(ctx context.Context, frame Frame) error {
	payload := make([]byte, 513)
	copy(payload[1:], frame[:])
	return d.run(ctx, func() error {
		if err := writeAll(d.port, packet(6, payload)); err != nil {
			return err
		}
		return d.port.Drain()
	})
}

// A port driver may block a write or close. Bound each call without ever
// starting another operation on that handle; Close also waits for its last I/O.
func (d *enttec) run(ctx context.Context, operation func() error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	d.mu.Lock()
	if d.closing != nil || d.operation != nil {
		d.mu.Unlock()
		return errors.New("USB port is closing or still busy")
	}
	done := make(chan struct{})
	d.operation = done
	d.mu.Unlock()
	result := make(chan error, 1)
	go func() {
		err := operation()
		result <- err
		d.mu.Lock()
		d.operation = nil
		close(done)
		d.mu.Unlock()
	}()
	select {
	case err := <-result:
		// Synchronize with clearing operation before the caller can send again.
		<-done
		return err
	case <-ctx.Done():
		return fmt.Errorf("USB operation timed out: %w", ctx.Err())
	}
}

func (d *enttec) Close(ctx context.Context) error {
	d.mu.Lock()
	if d.closing == nil {
		d.closing = make(chan struct{})
		operation := d.operation
		go func() {
			err := d.port.Close()
			if operation != nil {
				<-operation
			}
			d.mu.Lock()
			d.closeErr = err
			close(d.closing)
			d.mu.Unlock()
		}()
	}
	done := d.closing
	d.mu.Unlock()
	select {
	case <-done:
		return d.closeErr
	case <-ctx.Done():
		return fmt.Errorf("USB port cleanup still pending: %w", ctx.Err())
	}
}

func Ports() ([]string, error) {
	ports, err := serial.GetPortsList()
	if err != nil {
		return nil, fmt.Errorf("list USB serial ports: %w", err)
	}
	// Use stable Linux names in preference to guessing a ttyUSB number.
	stable, err := filepath.Glob("/dev/serial/by-id/*")
	if err != nil {
		return nil, err
	}
	ports = append(stable, ports...)
	if ports == nil {
		ports = []string{}
	}
	sort.Strings(ports)
	return ports, nil
}
