package output

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"
)

type fakeSerial struct {
	reader *bytes.Reader
	writes bytes.Buffer
	short  int
	fail   error
}

func (s *fakeSerial) Read(b []byte) (int, error) { return s.reader.Read(b) }
func (s *fakeSerial) Write(b []byte) (int, error) {
	if s.fail != nil {
		return 0, s.fail
	}
	if s.short > 0 && len(b) > s.short {
		b = b[:s.short]
	}
	return s.writes.Write(b)
}
func (s *fakeSerial) Close() error                       { return nil }
func (s *fakeSerial) SetReadTimeout(time.Duration) error { return nil }
func (s *fakeSerial) ResetInputBuffer() error            { return nil }
func (s *fakeSerial) Drain() error                       { return nil }

func TestENTTECHandshakeAndFullUniverse(t *testing.T) {
	input := append([]byte{0x42, 0x43}, packet(5, []byte{0})...)
	input = append(input, packet(3, []byte{1, 2, 9, 1, 40})...)
	port := &fakeSerial{reader: bytes.NewReader(input), short: 7}
	d := &enttec{port: port}
	if err := d.initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(port.writes.Bytes(), []byte{0x7e, 3, 2, 0, 0, 0, 0xe7}) {
		t.Fatal("incorrect read-only parameter request", port.writes.Bytes())
	}
	port.writes.Reset()
	var frame Frame
	frame[0], frame[1], frame[511] = 0x7e, 0xe7, 255
	if err := d.WriteFrame(context.Background(), frame); err != nil {
		t.Fatal(err)
	}
	wire := port.writes.Bytes()
	if len(wire) != 518 || !bytes.Equal(wire[:5], []byte{0x7e, 6, 1, 2, 0}) ||
		!bytes.Equal(wire[5:517], frame[:]) || wire[517] != 0xe7 {
		t.Fatal("incorrect framing, little-endian length, start code, or full DMX universe")
	}
}

func TestENTTECRejectsBadRepliesAndWrites(t *testing.T) {
	for _, response := range [][]byte{
		{0x7e, 3, 0xff, 0xff},
		{0x7e, 3, 1, 0, 0, 0},
		packet(3, []byte{1, 2}),
		packet(3, []byte{1, 2, 8, 1, 40}),
		packet(3, []byte{1, 2, 9, 0, 40}),
		packet(3, []byte{1, 2, 9, 1, 41}),
	} {
		d := &enttec{port: &fakeSerial{reader: bytes.NewReader(response)}}
		if d.initialize(context.Background()) == nil {
			t.Fatal("invalid widget reply accepted", response)
		}
	}
	d := &enttec{port: &fakeSerial{fail: io.ErrClosedPipe}}
	if err := d.WriteFrame(context.Background(), Frame{}); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatal("write error lost", err)
	}
	if err := writeAll(zeroWriter{}, []byte{1}); !errors.Is(err, io.ErrShortWrite) {
		t.Fatal("zero-byte write must fail")
	}
}

type zeroWriter struct{}

func (zeroWriter) Write([]byte) (int, error) { return 0, nil }

type blockedSerial struct {
	stopped chan struct{}
	once    sync.Once
}

func (s *blockedSerial) Read([]byte) (int, error)           { <-s.stopped; return 0, io.ErrClosedPipe }
func (s *blockedSerial) Write([]byte) (int, error)          { <-s.stopped; return 0, io.ErrClosedPipe }
func (s *blockedSerial) Close() error                       { s.once.Do(func() { close(s.stopped) }); return nil }
func (s *blockedSerial) SetReadTimeout(time.Duration) error { return nil }
func (s *blockedSerial) ResetInputBuffer() error            { return nil }
func (s *blockedSerial) Drain() error                       { return nil }

func TestENTTECWriteTimeoutAndCleanup(t *testing.T) {
	d := &enttec{port: &blockedSerial{stopped: make(chan struct{})}}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := d.WriteFrame(ctx, Frame{}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("blocked write must time out", err)
	}
	cleanup, cancelCleanup := context.WithTimeout(context.Background(), time.Second)
	defer cancelCleanup()
	if err := d.Close(cleanup); err != nil {
		t.Fatal(err)
	}
	if d.WriteFrame(context.Background(), Frame{}) == nil {
		t.Fatal("closed device must not accept another frame")
	}
}

func TestENTTECReadTimeout(t *testing.T) {
	d := &enttec{port: &blockedSerial{stopped: make(chan struct{})}}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := d.initialize(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("initialization must time out", err)
	}
	if err := d.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}
