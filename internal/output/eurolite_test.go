package output

import (
	"bytes"
	"context"
	"testing"
)

func TestEuroliteMK2WritesFullFrameWithoutParameterQuery(t *testing.T) {
	port := &fakeSerial{short: 7}
	d := &euroliteMK2{&enttec{port: port}}
	defer d.Close(context.Background())
	if err := d.initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	if port.writes.Len() != 0 {
		t.Fatal("MK2 must not send ENTTEC parameter queries")
	}
	var frame Frame
	frame[0], frame[511] = 123, 231
	if err := d.WriteFrame(context.Background(), frame); err != nil {
		t.Fatal(err)
	}
	wire := port.writes.Bytes()
	if len(wire) != 518 || !bytes.Equal(wire[:5], []byte{0x7e, 6, 1, 2, 0}) ||
		!bytes.Equal(wire[5:517], frame[:]) || wire[517] != 0xe7 {
		t.Fatal("incorrect MK2 framing or universe data")
	}
}

func TestEuroliteMK2CancelledInitialization(t *testing.T) {
	d := &euroliteMK2{&enttec{port: &fakeSerial{}}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := d.initialize(ctx); err != context.Canceled {
		t.Fatal("cancelled initialization succeeded", err)
	}
}
