package output

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"stageglowdmx/internal/config"
	"stageglowdmx/internal/engine"
)

type recordingDevice struct {
	mu        sync.Mutex
	frames    []Frame
	fail      error
	closed    bool
	closeErr  error
	closeGate <-chan struct{}
}

func (d *recordingDevice) WriteFrame(_ context.Context, frame Frame) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.fail != nil {
		return d.fail
	}
	if d.closed {
		return errors.New("already closed")
	}
	d.frames = append(d.frames, frame)
	return nil
}

func (d *recordingDevice) Close(context.Context) error {
	if d.closeGate != nil {
		<-d.closeGate
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.closed = true
	return d.closeErr
}

func usbRig() config.Config {
	c := config.Defaults()
	c.Output = config.Output{Mode: "enttec", SerialPort: "COM1"}
	v, b := 200, 16
	c.Profiles = []config.Profile{{
		ID: "rgb", Name: "RGB", Footprint: 4, WhiteMix: 1, AmberMix: 1,
		Channels: []config.Channel{{Offset: 0, Kind: "red"}, {Offset: 1, Kind: "green"}, {Offset: 2, Kind: "blue"},
			{Offset: 3, Kind: "fixed", Value: &v, BlackoutValue: &b}},
	}}
	c.Groups = []config.Group{{ID: "front", Name: "Front"}}
	c.Loadouts = []config.Loadout{{ID: "rig", Name: "Rig", Fixtures: []config.Fixture{{
		ID: "wash", Name: "Wash", ProfileID: "rgb", GroupID: "front", StartAddress: 1,
	}}}}
	c.ActiveLoadoutID = "rig"
	c.Moods = []config.Mood{{
		ID: "warm", Name: "Warm", BeatsPerChange: 8, Smoothness: 1,
		Groups: []config.MoodGroup{{GroupID: "front", ColorMode: "static", Palette: []string{"#ff0000"}, IntensityMin: 0.5, IntensityMax: 0.5}},
	}}
	return c
}

func readyController(t *testing.T, device Device) *Controller {
	t.Helper()
	c := usbRig()
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	lighting := engine.New(c, nil, 1)
	return New(c, lighting, func(string) (Device, error) { return device, nil })
}

func waitCleanup(t *testing.T, c *Controller) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for {
		_, status := c.State()
		if !status.CleanupPending {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("cleanup did not finish")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestEnableStartsWithSafeFrameAndRequiresHardwareRelease(t *testing.T) {
	d := &recordingDevice{}
	c := readyController(t, d)
	defer c.Shutdown()
	c.Select("warm")
	c.Blackout(false, false)
	if err := c.Enable(); err != nil {
		t.Fatal(err)
	}
	state, status := c.State()
	if !state.Blackout || !state.OutputEnabled || !status.Enabled || d.frames[0][0] != 0 || d.frames[0][3] != 16 {
		t.Fatal("enable must first send a profile-safe blackout", state, status, d.frames)
	}
	if c.Blackout(false, false) == nil {
		t.Fatal("simulation control released hardware blackout")
	}
	if err := c.Blackout(false, true); err != nil {
		t.Fatal(err)
	}
	c.Tick()
	if d.frames[len(d.frames)-1][0] != 128 || d.frames[len(d.frames)-1][3] != 200 {
		t.Fatal("mood output differs from engine")
	}
	before := len(d.frames)
	if err := c.Blackout(true, true); err != nil || len(d.frames) != before+1 || d.frames[before][0] != 0 {
		t.Fatal("blackout must send immediately")
	}
	if err := c.Disable(); err != nil {
		t.Fatal(err)
	}
	waitCleanup(t, c)
	state, status = c.State()
	if !state.Blackout || state.OutputEnabled || status.Enabled || !d.closed || d.frames[len(d.frames)-1][3] != 16 {
		t.Fatal("disable must send safe frame, disarm and close")
	}
}

func TestUSBFailureLatchesBlackoutAndNeverReconnects(t *testing.T) {
	d := &recordingDevice{}
	c := readyController(t, d)
	var opens atomic.Int32
	c.open = func(string) (Device, error) { opens.Add(1); return d, nil }
	if err := c.Enable(); err != nil {
		t.Fatal(err)
	}
	c.Select("warm")
	c.Blackout(false, true)
	d.fail = errors.New("USB unplugged")
	c.Tick()
	waitCleanup(t, c)
	for i := 0; i < 10; i++ {
		c.Tick()
	}
	state, status := c.State()
	if status.Enabled || status.State != "fault" || status.Error == "" || !state.Blackout || opens.Load() != 1 {
		t.Fatal("fault did not fail closed", state, status, opens.Load())
	}
	if c.Play("warm", false) == nil || c.Play("warm", true) == nil {
		t.Fatal("performance action must not bypass fault")
	}
}

func TestPerformancePlayReleasesBlackoutAndWritesImmediately(t *testing.T) {
	for _, hardware := range []bool{false, true} {
		d := &recordingDevice{}
		c := readyController(t, d)
		if hardware {
			if err := c.Enable(); err != nil {
				t.Fatal(err)
			}
		}
		for i := 0; i < 2; i++ {
			c.Blackout(true, hardware)
			if c.Play("missing", hardware) == nil {
				t.Fatal("invalid mood accepted")
			}
			if state, _ := c.State(); !state.Blackout {
				t.Fatal("invalid mood released blackout")
			}
			if c.Play("warm", !hardware) == nil {
				t.Fatal("stale output mode released blackout")
			}
			before := len(d.frames)
			if err := c.Play("warm", hardware); err != nil {
				t.Fatal(err)
			}
			if state, _ := c.State(); state.Blackout || state.ActiveMoodID != "warm" {
				t.Fatal("performance selection did not resume lighting")
			}
			if hardware && (len(d.frames) != before+1 || d.frames[before][0] != 128) {
				t.Fatal("performance selection did not immediately write mood")
			}
		}
		c.Shutdown()
		if c.Play("warm", hardware) == nil {
			t.Fatal("shutdown controller allowed performance play")
		}
	}
}

func TestPendingCleanupBlocksReenable(t *testing.T) {
	gate := make(chan struct{})
	d := &recordingDevice{closeGate: gate}
	c := readyController(t, d)
	if err := c.Enable(); err != nil {
		t.Fatal(err)
	}
	c.Disable()
	if c.Enable() == nil {
		t.Fatal("re-enabled while old serial handle still active")
	}
	close(gate)
	waitCleanup(t, c)
	next := &recordingDevice{}
	c.open = func(string) (Device, error) { return next, nil }
	if err := c.Enable(); err != nil {
		t.Fatal("explicit re-enable after cleanup failed", err)
	}
	state, _ := c.State()
	if !state.Blackout || next.frames[0][0] != 0 {
		t.Fatal("reconnect replayed an old frame")
	}
	c.Shutdown()
}

func TestOutputModesPatchGuardsAndStartup(t *testing.T) {
	d := &recordingDevice{}
	c := readyController(t, d)
	before, status := c.State()
	if status.Enabled || !before.Blackout {
		t.Fatal("unsafe startup")
	}
	for _, mode := range []string{"simulation", "artnet"} {
		next := usbRig()
		next.Output.Mode = mode
		c.Configure(next)
		if c.Enable() == nil {
			t.Fatal("unsupported transport armed", mode)
		}
	}
	c.Configure(usbRig())
	if err := c.Enable(); err != nil {
		t.Fatal(err)
	}
	next := usbRig()
	next.Output.SerialPort = "COM2"
	if c.CanConfigure(next, false) == nil || c.CanConfigure(usbRig(), true) == nil {
		t.Fatal("live patch/import was not blocked")
	}
	next = usbRig()
	next.BPM = 60
	if err := c.CanConfigure(next, false); err != nil {
		t.Fatal("tempo changes must remain available", err)
	}
	c.Disable()
	c.Configure(next)
	state, _ := c.State()
	if !state.Blackout || state.OutputEnabled || state.BPM != 60 {
		t.Fatal("configuration did not remain safe")
	}
	c.Shutdown()
	if c.Enable() == nil {
		t.Fatal("shutdown output reopened")
	}
}

func TestOpenAndFirstWriteFailuresDoNotArm(t *testing.T) {
	for _, open := range []Opener{
		func(string) (Device, error) { return nil, errors.New("port missing or busy") },
		func(string) (Device, error) { return &recordingDevice{fail: errors.New("write failed")}, nil },
	} {
		c := readyController(t, nil)
		c.open = open
		if c.Enable() == nil {
			t.Fatal("failure armed output")
		}
		state, status := c.State()
		if state.OutputEnabled || !state.Blackout || status.State != "fault" {
			t.Fatal("failed enable was not safe")
		}
		c.Shutdown()
	}
}

func TestBackgroundCadenceRunsWithoutBrowser(t *testing.T) {
	d := &recordingDevice{}
	c := readyController(t, d)
	c.Enable()
	c.Select("warm")
	c.Blackout(false, true)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { c.Run(ctx); close(done) }()
	time.Sleep(180 * time.Millisecond)
	cancel()
	<-done
	d.mu.Lock()
	count := len(d.frames)
	d.mu.Unlock()
	if count < 4 {
		t.Fatal("frames did not continue without a browser", count)
	}
	c.Shutdown()
}

func TestLateOpenIsRetiredBeforeRetry(t *testing.T) {
	gate := make(chan struct{})
	d := &recordingDevice{}
	c := readyController(t, d)
	var opens atomic.Int32
	c.open = func(string) (Device, error) {
		opens.Add(1)
		<-gate
		return d, nil
	}
	if err := c.Enable(); err == nil {
		t.Fatal("blocked open must time out")
	}
	state, status := c.State()
	if state.OutputEnabled || !state.Blackout || status.State != "fault" || !status.CleanupPending {
		t.Fatal("timed out open must disarm and gate cleanup", state, status)
	}
	if c.Enable() == nil || opens.Load() != 1 {
		t.Fatal("retry opened a second connection during cleanup")
	}
	close(gate)
	waitCleanup(t, c)
	if !d.closed || len(d.frames) != 0 {
		t.Fatal("late connection must be closed without sending stale data")
	}
	c.Shutdown()
}

func TestCloseFailureRequiresRestart(t *testing.T) {
	c := readyController(t, &recordingDevice{closeErr: errors.New("driver close failed")})
	if err := c.Enable(); err != nil {
		t.Fatal(err)
	}
	c.Disable()
	waitCleanup(t, c)
	_, status := c.State()
	if status.State != "fault" || status.Error == "" || c.Enable() == nil {
		t.Fatal("failed close must prevent another connection", status)
	}
	c.Shutdown()
}
