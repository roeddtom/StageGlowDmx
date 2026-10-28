# StageGlow: program flow for a developer new to Go

This guide explains the current implementation, not just the intended design.
It assumes you already know programming concepts such as functions, objects,
HTTP, and asynchronous execution, but have not used much Go.

The central idea is:

> The browser edits settings and sends commands. The Go process owns lighting
> state, calculates what the fixtures should do now, and optionally writes the
> result to the ENTTEC USB PRO.

Art-Net output is not implemented yet.

## 1. The map

```text
Browser: React + TypeScript
  Fixtures / Moods / Show
           |
           | HTTP JSON requests and state polling
           v
internal/server/server.go
  HTTP validation, endpoint routing, command serialization
           |
           +---- configuration ----> internal/config/
           |                         validated settings + config.json + .bak
           |
           v
internal/output/controller.go <------ independent 30 Hz ticker
  output lifecycle, locking, safety, USB faults
           |
           +----> internal/engine/engine.go
           |       time + mood + loadout -> fixture values -> 512 DMX channels
           |
           +----> internal/output/enttec.go
                   frame -> USB PRO packet -> operating-system serial port
```

The controller uses the same engine for simulation and physical USB output.
There is not a second, separate hardware mood generator.

| Location | Responsibility |
|---|---|
| `cmd\stageglow\main.go` | Construct the application and manage its lifetime |
| `cmd\stageglow\webui.go` | Serve frontend assets embedded in the executable |
| `internal\config\config.go` | Configuration types, defaults, decoding, validation, copying |
| `internal\config\store.go` | Read/write configuration safely |
| `internal\server\server.go` | Expose HTTP endpoints |
| `internal\engine\engine.go` | Generate lighting from elapsed time |
| `internal\output\controller.go` | Own output enablement, cadence, and fault handling |
| `internal\output\enttec.go` | Encode and send the USB PRO serial protocol |
| `web\src\ui\App.tsx` | Browser loading, autosave, polling, command routing |
| `web\src\ui\Moods.tsx` | Mood editing and preview controls |
| `web\src\ui\Show.tsx` | Performance grid and blackout toggle |

`internal` has a special meaning in Go: packages under it can only be imported
from the permitted parent directory tree. These packages are application
internals, not a public library API.

## 2. Starting the process

Go runs the `main` function in `package main`:

```go
func main() {
    if err := run(); err != nil {
        log.Print(err)
        os.Exit(1)
    }
}
```

The real setup is in `run() error`. Separating it from `main()` means cleanup
registered inside `run` can execute before `main` exits the process.
`os.Exit` itself does not run deferred functions.

The important startup sequence, excerpted from `run`, is:

```go
store, err := config.Open(*data)
if err != nil {
    return err
}
lighting := engine.New(store.Get(), nil, time.Now().UnixNano())
controller := output.New(store.Get(), lighting, nil)
```

Read this as:

1. Load and validate saved configuration, or create safe defaults for a new install.
2. Create the lighting engine with a configuration snapshot.
3. Give the engine to an output controller.

Some Go syntax in those few lines:

- `:=` declares and initializes local variables, with inferred types.
- Functions can return multiple values. `config.Open` returns `(*Store, error)`.
- `nil` means no value for a pointer, interface, function, or certain other types.
- Errors are normally returned and checked, rather than thrown as exceptions.
- `flag.String` returns a `*string`; `*data` reads the string it points to.
- `New` is a naming convention, not a language-level constructor keyword.

The two `nil` arguments above select defaults. The engine uses `time.Now` as its
clock; the controller uses `OpenENTTEC` as its device opener. Tests can supply
different implementations.

Startup does **not** enable USB output. The engine begins in blackout, with no
selected mood and master brightness at 1.

### Background work

Startup launches the output loop:

```go
outputCtx, stopOutput := context.WithCancel(context.Background())
defer stopOutput()
go controller.Run(outputCtx)
```

`go` starts a **goroutine**: a lightweight concurrent execution managed by the
Go runtime. This call does not wait for `Run` to finish.

A `context.Context` carries cancellation and deadlines. Calling `stopOutput`
signals cancellation; it does not forcibly kill a goroutine or interrupt every
possible operating-system operation.

`defer` schedules a function call when the enclosing function returns.
Deferred calls execute in reverse registration order.

The HTTP server is also started in a goroutine. `run` then waits for either an
HTTP serving error or an operating-system shutdown signal.

## 3. Serving the browser without a separate web server

The build script first runs the Vite frontend build. Go then embeds those built
assets into the executable:

```go
//go:embed all:web
var frontendFiles embed.FS
```

This is a compiler directive, not an ordinary explanatory comment.
`frontendHandler()` exposes the `web/dist` subtree through an `http.Handler`.
The deployed Pi therefore needs neither Node.js nor a separate web server.

The main HTTP handler sends non-API requests to that frontend handler:

```go
if !strings.HasPrefix(r.URL.Path, "/api/") && r.URL.Path != "/api" {
    frontend.ServeHTTP(w, r)
    return
}
```

`http.Handler` is an interface with a `ServeHTTP` method. `http.HandlerFunc`
adapts an ordinary function with the appropriate signature into that interface.
The standard library handles connections and invokes handlers concurrently.

## 4. Configuration versus runtime state

These are deliberately different:

| Persistent configuration | In-memory runtime state |
|---|---|
| Profiles, groups, loadouts, DMX addresses | Selected/pending mood |
| Mood palettes, intensity, smoothness, crossfade | Blackout state |
| BPM and Show assignments | Master brightness |
| Output mode and local serial-port name | USB enabled state, errors, frames sent |

For example, saving an output mode of `enttec` does not mean that output is armed
after a restart.

### Structs, methods, and JSON

Go uses structs rather than classes. This type is from the configuration model:

```go
type MoodGroup struct {
    GroupID      string   `json:"groupId"`
    ColorMode    string   `json:"colorMode"`
    Palette      []string `json:"palette"`
    IntensityMin float64  `json:"intensityMin"`
    IntensityMax float64  `json:"intensityMax"`
    UV           float64  `json:"uv"`
}
```

`[]string` is a **slice**, a variable-length view over an underlying array.
`float64` is a 64-bit floating-point number.

The backtick text is a **struct tag**. `encoding/json` uses it to map the Go
field `IntensityMin` to JSON `intensityMin`.

An uppercase initial letter exports a name from its package. `MoodGroup` and
its fields are exported; a lowercase helper such as `clone` is package-private.

A method declares its receiver before its name:

```go
func (s *Store) Get() Config {
    s.mu.RLock()
    defer s.mu.RUnlock()
    return clone(s.current)
}
```

`s *Store` is a pointer receiver, roughly the instance this method acts on.
Go automatically dereferences it for expressions such as `s.current`.

The returned configuration is deep-copied. Simply copying a struct containing
slices would still share the slices' backing storage. A deep copy prevents
browser handlers, the engine, and callers from accidentally sharing mutable
configuration.

### Saving from the browser

```text
Edit a mood draft
  -> Apply mood
  -> App stages updated configuration
  -> debounced autosave checks the saved server baseline
  -> PUT /api/config
  -> strict JSON decoding + full configuration validation
  -> controller checks whether the change is allowed while USB is active
  -> store.Save(...)
  -> controller.Configure(...)
  -> canonical configuration returned to browser
```

Moving a mood slider changes the local draft, not the fixtures immediately.
Applying the mood stages a save. Once saved, edits to a currently running mood
can affect generated output.

The backend validates independently of the frontend. Invalid ranges, overlapping
fixture addresses, missing references, unknown JSON fields, and malformed
commands are not trusted just because they came from the UI.

`Store.Save` validates, copies, and encodes the configuration. It retains the
previous validated file in `.bak`, writes through a temporary file, and renames
the replacement into place. It updates in-memory configuration only after the
write succeeds. A corrupt existing file is not silently replaced with defaults.

## 5. Following one Show button press

Suppose USB is enabled and you tap a mood button.

```text
Show button
  -> onCommand({ action: 'select', moodId: '...' })
  -> App POSTs /api/lighting
  -> server validates mode and command shape
  -> controller.Select(id)
  -> engine.Select(id)
  -> server returns the latest runtime state
  -> React updates the selected/pending label
```

With USB disabled, App uses `/api/simulation` instead. The backend rejects
commands sent through the wrong mode, except that requesting blackout through
the simulation endpoint is also allowed as an emergency action.

The command is JSON:

```json
{"action":"select","moodId":"your-saved-mood-id"}
```

The server decodes into a struct containing optional pointers:

```go
var command struct {
    Action  string   `json:"action"`
    MoodID  *string  `json:"moodId,omitempty"`
    Enabled *bool    `json:"enabled,omitempty"`
    Value   *float64 `json:"value,omitempty"`
}
```

Why `*bool` instead of `bool`? A plain `bool` defaults to `false`, so it could not
distinguish "the client supplied false" from "the client omitted this field."
Here `nil` means absent or JSON null, and both are rejected where a boolean is
required. `*command.Enabled` reads the supplied value after a nil check.
`omitempty` affects JSON encoding; it does not enforce required request fields.

Selecting a mood captures the current generated light values as the start of
the next crossfade. It **does not release blackout**.

Pause is not a separate engine mode: its fixed button selects the configured
Pause mood through this same path.

### Blackout is a separate command

Show's toggle sends one of:

```json
{"action":"blackout","enabled":true}
```

```json
{"action":"blackout","enabled":false}
```

Here `enabled` refers to **blackout**, not USB output enablement.
USB enablement uses the separate `POST /api/output` endpoint.

The controller changes engine blackout and, if USB is armed, immediately attempts
to write a frame. It uses the same lock as scheduled writes, so an older normal
frame cannot race past the blackout write.

While blackout is on, mood buttons still choose the pending mood. Tapping the
Blackout toggle again restores that mood using its crossfade duration.

## 6. How a mood becomes DMX values

The key entry point is `Engine.State()`. Despite its getter-like name, it
advances the engine's time calculation and renders a fresh snapshot.

```text
Current time
  -> elapsed beats
  -> position within this mood's change interval
  -> deterministic colour/intensity targets for each fixture
  -> smooth interpolation within the interval
  -> crossfade from the previous lighting state
  -> profile-specific channel mapping + master + blackout
  -> 512-channel DMX universe and browser preview values
```

### Time, not request counts

The beat calculation is:

```go
e.beats = e.anchor + now.Sub(e.epoch).Seconds()*e.cfg.BPM/60
```

At 120 BPM, two beats pass per second. With `BeatsPerChange` set to 8, each
target interval lasts four seconds.

The generator is sampled at the current time; it does not advance by a fixed
amount each time the browser asks for state. Closing a browser therefore does
not pause the mood. If output is disabled and no one asks for a snapshot, the
engine can calculate the correct point when it is next sampled.

BPM changes rebase the time anchor to preserve the current beat position.
Normal `time.Now` values include monotonic timing information used by Go's
duration calculations; the engine also guards against its clock moving backward.

### Smoothness and crossfade are different

For evolving moods, `lights` calculates the interval number and its blend:

```go
progress := math.Max(0, e.beats-e.start) / mood.BeatsPerChange
segment := uint64(math.Floor(progress))
blend := glide(progress-math.Floor(progress), mood.Smoothness)
```

Smoothness controls how much of **each target interval** is spent gliding.
Crossfade controls the transition **into a selected or edited mood**, or back
from blackout. Both use eased interpolation rather than abrupt linear corners.

The small interpolation helper is:

```go
func lerp(a, b, t float64) float64 { return a + (b-a)*t }
```

At `t = 0`, this returns `a`; at `t = 1`, it returns `b`.

The `random` helper hashes the seed, mood/fixture identity, interval number, and
stream number. It produces repeatable targets without consuming a sequential
random-number stream on every sample. Colour and intensity use separate streams.
Extra browser polls do not change the targets.

### Mapping fixture values to channels

The engine looks up each fixture's profile and calls `render`.
That function handles RGB, approximate white/amber extraction, UV, optional
dimmer, fixed safe channel values, master brightness, and blackout.

Intensity is applied once: if a fixture has a dimmer channel, brightness is
placed there rather than multiplying both colour and dimmer.

The resulting channel values are copied into the universe:

```go
for offset, value := range channels {
    state.DMX[fixture.StartAddress-1+offset] = value
}
```

DMX addresses in configuration are 1-based; Go array indices are 0-based.
`[512]int` is a fixed-size array, unlike the slice `[]int`.

Blackout does not necessarily mean every channel is zero: emitters are dark,
but fixed-function channels can require profile-specific safe blackout values.

## 7. The independent USB output loop

The actual loop in `Controller.Run` is:

```go
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
```

`frameInterval` is `time.Second / 30`.

- A channel carries values between concurrent work.
- `<-ticker.C` receives a tick from the ticker's channel.
- `<-ctx.Done()` waits for cancellation.
- `select` waits for a channel operation that can proceed; this is not a
  value-based `switch`.
- The loop still exists while output is disabled, but `Tick` writes only when armed.

On an enabled tick, the controller asks the engine for the **latest** state,
converts channel integers to bytes, and writes a frame. There is no queue of
old lighting frames to replay after a delay.

### Interfaces make real and fake devices interchangeable

The transport contract is small:

```go
type Frame [512]byte

type Device interface {
    WriteFrame(context.Context, Frame) error
    Close(context.Context) error
}

type Opener func(string) (Device, error)
```

Any type implementing those methods satisfies `Device` automatically; Go has no
`implements` declaration. `Opener` is a named function type, so tests can inject
a function returning a recording device instead of opening a real serial port.

The controller's engine is a concrete `*engine.Engine`; the serial device is an
interface. Abstraction is introduced where replacing behavior is useful.

### USB PRO framing

The serial transport uses ENTTEC's framed protocol, not raw DMX bit timing:

```text
0x7e | label | payload length, low byte | high byte | payload | 0xe7
```

The packet helper contains:

```go
message := make([]byte, len(data)+5)
message[0], message[1] = 0x7e, label
binary.LittleEndian.PutUint16(message[2:4], uint16(len(data)))
copy(message[4:], data)
message[len(message)-1] = 0xe7
```

`make` allocates a slice, map, or channel. `message[2:4]` is a slice covering
indices 2 and 3. `copy` copies bytes into existing storage.

Initialization sends a read-only widget-parameters request, label 3.
DMX output uses label 6 with 513 payload bytes: a zero start code followed by
512 channel bytes. The interface handles DMX wire timing and repeats its buffered
frame at its configured refresh rate.

## 8. Concurrency and fault boundaries

Several goroutines can want state at once: HTTP handlers and the output ticker.
A mutex protects shared mutable state:

```go
c.mu.Lock()
defer c.mu.Unlock()
```

The server serializes API handling with its command mutex. The controller mutex
coordinates commands with scheduled USB writes. The engine has its own mutex,
and the store uses an `RWMutex`, allowing multiple readers or one writer.
These locks protect different layers; a goroutine is not automatically
thread-safe just because Go makes it easy to launch.

The usual call direction is:

```text
HTTP handler -> controller lock -> engine lock
Ticker       -> controller lock -> engine lock
```

Avoid adding code that holds the engine lock and then calls back into the
controller: reversing lock order can cause a deadlock.

USB operations have deadlines, but a blocked driver call may outlive its context.
The serial wrapper isolates potentially blocking work, prevents overlapping
operations on one handle, and coordinates closing. Cleanup gates further opens
so retries cannot accumulate competing connections.

Enabling output performs the following:

```text
Check mode, port, active patch, and cleanup
  -> latch engine blackout
  -> open serial device with a bounded wait
  -> perform USB PRO handshake
  -> write the first profile-safe blackout frame
  -> only then mark output enabled
```

A write failure disarms output, latches engine blackout, records an error, and
retires the device. It does not automatically reconnect. This is a software
safety response, not proof the physical lights went dark: USB PRO may keep
repeating the last frame it received.

## 9. Browser state and shutdown

App initially fetches configuration, runtime state, and health. It then polls
`GET /api/state`: normally once per second, or about every 150 ms for Moods and
active Show mode. This is HTTP polling, not WebSockets.

Command responses also contain runtime state. A command-version counter prevents
an older in-flight poll from overwriting a newer command result.
The browser displays server state instead of assuming that a click succeeded.
On connection loss it marks output unknown and does not queue performance
commands for later replay.

The Go process waits for shutdown with a channel-based `select`. It asks the HTTP
server to finish outstanding requests, then unwinds `run`'s deferred calls:
signal cleanup, output-loop cancellation, and controller shutdown.
Controller shutdown attempts a safe blackout write and serial cleanup with
bounded waits.

This applies to a normal shutdown, not a crash, forced termination, or power
loss. Neither process shutdown nor a disconnected browser guarantees physical
blackout.

## 10. Learning by running a focused test

Tests live beside the code in files ending `_test.go`. A test is a function such
as `func TestSomething(t *testing.T)`.

The engine tests inject a clock and seed:

```go
now := time.Unix(1000, 0)
e := New(c, func() time.Time { return now }, 42)
```

Here `c` is the fixture/mood configuration built by the test. The function
literal closes over `now`; advancing that variable advances simulated time:

```go
now = now.Add(100 * time.Millisecond)
state := e.State()
```

No real-time sleep, browser, or USB device is needed to test the generator.
Output tests instead inject fake `Device` implementations to record frames or
simulate failures.

From the repository root in PowerShell, with Go and Node.js available:

```powershell
# First application build: install locked web dependencies, build web, test Go,
# then build the host executable.
.\scripts\build.ps1

# Later builds when dependencies are already installed:
.\scripts\build.ps1 -SkipInstall

# Run a focused engine behavior test:
go test ./internal/engine -run TestStaticFrontAndIndependentBoundedDrift -v

# Run backend tests and static analysis:
go test ./...
go vet ./...

# Run the locally built Windows application using a separate learning data dir:
.\build\stageglow.exe -listen 127.0.0.1:8080 -data .\learning-data
```

The slash-separated strings after `go test` are Go package patterns, not Windows
filesystem paths. Build the frontend before compiling the executable so embedded
assets exist. Do not rebuild Vite's output while Go is compiling that same asset
directory.

For a first reading session, follow `main.go` into `server.go`, then trace a mood
selection through `controller.Select` and `engine.Select`. Next read
`Engine.State`, `lights`, and `render`. Leave serial timeout/cleanup internals
until that ordinary command-to-frame flow feels familiar.
