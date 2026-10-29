package output

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"time"

	"stageglowdmx/internal/config"
	"stageglowdmx/internal/engine"
)

const (
	frameInterval = time.Second / 30
	ioTimeout     = 500 * time.Millisecond
	openTimeout   = 2 * time.Second
)

type Status struct {
	Mode           string `json:"mode"`
	Port           string `json:"port"`
	Enabled        bool   `json:"enabled"`
	State          string `json:"state"`
	Error          string `json:"error"`
	FramesSent     uint64 `json:"framesSent"`
	CleanupPending bool   `json:"cleanupPending"`
}

type Controller struct {
	mu       sync.Mutex
	cfg      config.Config
	engine   *engine.Engine
	open     Opener
	device   Device
	status   Status
	retiring <-chan error
	closed   bool
}

func New(c config.Config, lighting *engine.Engine, open Opener) *Controller {
	if open == nil {
		open = OpenENTTEC
	}
	return &Controller{
		cfg: c.Copy(), engine: lighting, open: open,
		status: Status{Mode: c.Output.Mode, Port: c.Output.SerialPort, State: "disabled"},
	}
}

// Run has no frame queue: delayed ticks render the newest state, not old frames.
func (c *Controller) Run(ctx context.Context) {
	ticker := time.NewTicker(frameInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.Tick()
		}
	}
}

func (c *Controller) Tick() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.status.Enabled {
		if err := c.write(); err != nil {
			c.fault(err)
		}
	}
}

func (c *Controller) State() (engine.State, Status) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.checkCleanup()
	state := c.engine.State()
	state.OutputEnabled = c.status.Enabled
	if c.status.Enabled {
		state.EngineStatus = "enttec"
	}
	return state, c.status
}

func (c *Controller) Enable() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return errors.New("output service is shutting down")
	}
	if c.status.Enabled {
		return errors.New("output is already enabled")
	}
	c.checkCleanup()
	if c.retiring != nil {
		return errors.New("previous USB operation is still cleaning up; do not reconnect or re-enable until cleanup finishes")
	}
	if c.cfg.Output.Mode != "enttec" || c.cfg.Output.SerialPort == "" {
		return errors.New("select ENTTEC DMX USB PRO and its serial port; Art-Net output is not implemented")
	}
	patched := false
	for _, l := range c.cfg.Loadouts {
		if l.ID == c.cfg.ActiveLoadoutID && len(l.Fixtures) > 0 {
			patched = true
		}
	}
	if !patched {
		return errors.New("select an active loadout containing fixtures before enabling output")
	}
	_ = c.engine.Blackout(true)
	c.status.State, c.status.Error = "opening", ""
	ctx, cancel := context.WithTimeout(context.Background(), openTimeout)
	defer cancel()
	type opened struct {
		device Device
		err    error
	}
	result := make(chan opened)
	openDone := make(chan error, 1)
	portName := c.cfg.Output.SerialPort
	go func() {
		device, err := c.open(portName)
		select {
		case result <- opened{device, err}:
			openDone <- nil
		case <-ctx.Done():
			// A late open must never leak a live port or race a new connection.
			if device != nil {
				openDone <- device.Close(context.Background())
			} else {
				openDone <- nil
			}
		}
	}()
	select {
	case value := <-result:
		if value.err != nil || value.device == nil {
			c.device = value.device
			err := value.err
			if err == nil {
				err = errors.New("serial opener returned no device")
			}
			c.fault(err)
			return err
		}
		c.device = value.device
	case <-ctx.Done():
		c.retiring = openDone
		c.status.CleanupPending = true
		c.status.State, c.status.Error = "fault", "USB port open timed out; waiting for driver cleanup before retry"
		return errors.New(c.status.Error)
	}
	if d, ok := c.device.(interface{ initialize(context.Context) error }); ok {
		initCtx, done := context.WithTimeout(context.Background(), openTimeout)
		err := d.initialize(initCtx)
		done()
		if err != nil {
			c.fault(err)
			return err
		}
	}
	if err := c.write(); err != nil {
		c.fault(err)
		return err
	}
	c.status.Enabled, c.status.State = true, "enabled"
	return nil
}

func (c *Controller) Disable() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.disable()
}

func (c *Controller) disable() error {
	_ = c.engine.Blackout(true)
	var err error
	if c.device != nil {
		err = c.write()
		if err == nil {
			c.status.Error = ""
		}
		c.retire()
	}
	c.status.Enabled = false
	if err != nil {
		c.status.State, c.status.Error = "fault", err.Error()
	} else if c.status.State != "fault" {
		c.status.State = "disabled"
	}
	return err
}

func (c *Controller) Shutdown() error {
	c.mu.Lock()
	c.closed = true
	err := c.disable()
	pending := c.retiring
	c.mu.Unlock()
	if pending != nil {
		select {
		case closeErr := <-pending:
			if err == nil {
				err = closeErr
			}
		case <-time.After(ioTimeout):
			if err == nil {
				err = errors.New("USB shutdown cleanup timed out; hardware may retain its last frame")
			}
		}
	}
	return err
}

func (c *Controller) Select(id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.engine.Select(id)
}

func (c *Controller) ClearBase() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.engine.ClearBase()
}

func (c *Controller) Master(value float64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.engine.SetMaster(value)
}

func (c *Controller) Blackout(enabled bool, hardware bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !enabled && c.status.Enabled && !hardware {
		return errors.New("use the explicit hardware blackout release control while USB output is enabled")
	}
	if !enabled && hardware && !c.status.Enabled {
		return errors.New("enable USB output before releasing hardware blackout")
	}
	if err := c.engine.Blackout(enabled); err != nil {
		return err
	}
	if c.status.Enabled {
		if err := c.write(); err != nil {
			c.fault(err)
			return err
		}
	}
	return nil
}

func (c *Controller) CanConfigure(next config.Config, importing bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.status.Enabled && (importing || patchChanged(c.cfg, next)) {
		return errors.New("black out and disable USB output before changing patch/output settings or importing configuration")
	}
	return nil
}

func (c *Controller) Configure(next config.Config) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if patchChanged(c.cfg, next) {
		_ = c.engine.Blackout(true)
	}
	c.cfg = next.Copy()
	c.engine.Configure(next)
	c.status.Mode, c.status.Port = next.Output.Mode, next.Output.SerialPort
}

func patchChanged(before, after config.Config) bool {
	return before.Output != after.Output || before.ArtNet != after.ArtNet ||
		before.ActiveLoadoutID != after.ActiveLoadoutID ||
		!reflect.DeepEqual(before.Profiles, after.Profiles) || !reflect.DeepEqual(before.Loadouts, after.Loadouts)
}

func (c *Controller) write() error {
	var frame Frame
	for i, value := range c.engine.State().DMX {
		frame[i] = byte(value)
	}
	ctx, cancel := context.WithTimeout(context.Background(), ioTimeout)
	defer cancel()
	if err := c.device.WriteFrame(ctx, frame); err != nil {
		return fmt.Errorf("USB DMX write failed; interface may retain its last frame: %w", err)
	}
	c.status.FramesSent++
	return nil
}

func (c *Controller) fault(err error) {
	c.status.Enabled, c.status.State, c.status.Error = false, "fault", err.Error()
	_ = c.engine.Blackout(true)
	if c.device != nil {
		c.retire()
	}
}

func (c *Controller) retire() {
	device := c.device
	c.device = nil
	done := make(chan error, 1)
	c.retiring = done
	c.status.CleanupPending = true
	go func() { done <- device.Close(context.Background()) }()
}

func (c *Controller) checkCleanup() {
	if c.retiring == nil {
		return
	}
	select {
	case err := <-c.retiring:
		c.retiring = nil
		c.status.CleanupPending = false
		if err != nil {
			c.status.State, c.status.Error = "fault", fmt.Sprintf("USB close failed; restart service and check device: %v", err)
			c.closed = true
		}
	default:
	}
}
