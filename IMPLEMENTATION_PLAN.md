# StageGlowDmx implementation plan

## Goal and agreed scope

Create gentle, evolving stage lighting that a musician can operate from a
touchscreen while playing. A mood describes lighting behavior, not just a
snapshot of DMX values. Everything runs on the closed local network, without
cloud services, accounts, CDNs, or an internet connection during a show.

Release one supports:

- One Art-Net universe and approximately 8-16 mixed RGB, RGBA, and RGBWAUV fixtures.
- Manually defined, reusable fixture profiles and saved rig loadouts.
- Named fixture groups such as Front, Back, Side Left, and Side Right.
- Static colours or independently drifting colours from a chosen palette.
- Fixed intensity or smoothly varying intensity within a range.
- Approximate automatic RGB/white/amber mixing and separate UV control.
- Global manual BPM and tap tempo, with beats-per-change and smoothness per mood.
- A show page with 8-12 ordinary mood slots plus dedicated Break and Blackout.
- Local autosave and downloadable/importable configuration backups.

No fixed gig deadline. Moving-head automation, microphone tempo detection,
Bluetooth MIDI, haze control, strobing, bounded colour-space ranges, and Luminair
import are outside release one.

## Deployment and architecture

Use the existing Raspberry Pi 2, connected by Ethernet to the closed-network
router. Phones and tablets connect to that router over Wi-Fi. Buying a newer Pi
is not necessary for the initial scope; assess the existing hardware before
considering an upgrade.

Recommended software:

- A currently supported Raspberry Pi OS Lite image compatible with the exact
  Pi 2 revision, preferably 32-bit for a common ARMv7 deployment target.
- A Go backend owning configuration, mood timing, DMX rendering, and Art-Net UDP.
- A React/TypeScript frontend built with Vite and served locally by the backend.
- HTTP APIs for configuration and commands, with a WebSocket for live state.
- Versioned JSON configuration, atomically saved on disk, with a last-known-good
  backup. A database is unnecessary for this small, single-writer configuration.
- One systemd service, starting on boot and restarting after a crash.

Build frontend assets and the Pi-compatible backend on a development machine;
deploy a self-contained application with no Node.js runtime needed on the Pi.
All browser assets, including icons and fonts if used, must be bundled locally.

The backend is authoritative. Browser timers must never drive lighting output.
The current mood continues if a browser disconnects or a phone sleeps. Reconnecting
clients retrieve the actual engine state rather than replaying old commands.

Use one active operator; additional clients may view state but cannot issue
competing changes. An explicit takeover releases the previous operator. Blackout
remains available to every connected client. This is a trusted-LAN application,
not an internet-facing service; reject cross-origin control requests and do not
enable permissive CORS.

## Configuration model

### Fixture profiles

A profile defines a name, DMX footprint, and zero-based channel offsets:

- Red, green, blue, optional white and amber, and optional UV.
- Optional master dimmer.
- Fixed safe values for all other channels, including a shutter-open value when
  needed to produce light without activating strobe or fixture macros.
- Explicit blackout values where a channel needs a different safe value.
- Per-profile white and amber contribution adjustments for approximate mixing.

Require every channel in the footprint to have an explicit supported function or
fixed value. Reject duplicate offsets, invalid offsets, and values outside 0-255.
Release one requires RGB-capable profiles; other fixture types wait for later.
Profiles with colour wheels or 16-bit movement channels are not automatically
supported merely because fixed channel values can be entered.

Provide a guarded live preview for commissioning. Preview requires deliberately
enabling output, never overrides blackout, and stops when the preview session
ends or disconnects. The default preview is dim and steady, without UV.

### Loadouts and groups

A loadout stores fixture instances, profile references, DMX start addresses, and
group membership. Addresses are shown as 1-512 in the UI. Reject overlapping
fixture footprints and any footprint extending beyond channel 512.

Groups have stable identities independent of a particular loadout. Moods target
these group identities, so the same mood works on a small or large rig. Each
fixture belongs to exactly one group within a loadout; individual mood overrides
are deferred.

Only one loadout is active. Switching loadouts or changing live patch/profile
definitions requires blackout and explicit confirmation. Changes invalidate
pending previews and rebuild the output map before output can be re-enabled.
Missing groups and unsupported channels are reported without blocking use of
otherwise compatible fixtures; unsupported UV is ignored with a visible warning.
Deleting referenced profiles or groups requires resolving references first.

### Moods and show

Each mood stores:

- Name and group assignments.
- Per group: static colour or palette drift, and fixed intensity or an intensity
  range. Front lighting can remain entirely static while other groups evolve.
- Per group: a separate fixed UV level, defaulting to zero.
- Beats per target change and smoothness, plus a mood-switch crossfade duration.

Palettes are ordered colour-stop lists. Each drifting fixture independently
chooses successive random target positions along the interpolated palette;
its current output glides to the next target without abrupt jumps. Static groups
do not change with tempo. Colour and intensity can vary independently.

A show stores 8-12 ordered ordinary mood slots and a designated Break mood.
Blackout is an engine override, not an editable mood. Break is an ordinary,
explicitly configured low-light mood accessible from its dedicated button.
Provide master brightness and prevent unassigned show buttons from activating.

### Runtime state and persistence

Persist profiles, loadouts, groups, moods, show layout, network configuration,
and the selected global BPM. Do not persist an armed/output-enabled state.
After process or Pi restart, always initialize in blackout; never automatically
resume light output from a saved mood.

Imports are schema-versioned and validated in full before replacing configuration.
Export configuration rather than transient DMX frames. Require blackout for an
import, retain the previous valid configuration, and reject unsupported future
schema versions with an actionable message. Corrupt configuration must not arm
output or silently overwrite the last-known-good backup.

## Lighting engine and Art-Net

Keep the renderer separate from networking and browser code. Use monotonic time
and injectable clocks/randomness so output behavior is repeatably testable.

Begin with a fixed 30 Hz output cadence and measure sustained operation on the
Pi 2. Maintain a complete 512-channel frame; browser activity must not interrupt
the cadence. Avoid disk writes and unbounded queues in the render loop.

Rendering steps:

1. Evaluate each fixture's current colour, intensity, and UV from its group mood.
2. Blend mood transitions from the currently rendered state, including when a
   second mood is selected before the previous transition finishes.
3. Convert logical colour to RGB/white/amber using bounded, nonnegative
   white/amber extraction and profile contribution settings.
4. Apply intensity and master brightness once: use the master dimmer where
   available, otherwise scale emitting channels. UV follows these controls too.
5. Apply profile channel mapping and safe fixed values.
6. Apply blackout last, setting all emitting channels/dimmers to zero and
   retaining appropriate safe values for non-light channels.

RGB-only fixtures receive RGB fallback. Approximate matching is acceptable;
physical colour matching is not promised without fixture calibration.

Ordinary mood changes and Break use configurable crossfades, defaulting to
3 seconds. Blackout takes effect on the next output frame and stays latched
until explicitly released. Selecting a mood during blackout changes the pending
mood without releasing blackout. The underlying mood clock continues; release
fades into its current state using the configured transition duration.

Global tempo changes preserve the current rendered value and timing phase,
retiming future targets without snapping. Label timing as "every 4 beats" or
"every 8 beats", not ambiguous fractions such as "1/4". For example, at 120 BPM,
8 beats is 4 seconds. Smoothness controls the proportion of each interval spent
gliding rather than holding.

Implement ArtDmx over UDP port 6454, initially using a manually configured
unicast node IP and Art-Net Port-Address. Clearly distinguish the node's universe
label from protocol addressing; confirm the mapping against the actual controller.
Include valid packet fields, length, byte order, and sequence handling. Discovery
and multiple universes are deferred.

Transmit full frames continuously while blacked out as well as while active.
Show output-enabled status, configured destination, and send errors. Successful
UDP sends do not prove the node is receiving; do not label them as confirmed
controller connectivity.

## User interface

### 1. Fixtures and loadouts

Profile editor, fixture patch table, group assignment, loadout save/duplicate,
address-conflict feedback, guarded preview, and Art-Net destination settings.
Keep advanced/raw DMX details away from the show page.

### 2. Mood editor

Group-based controls for colour mode, palettes, intensity, UV, tempo division,
smoothness, and crossfade. Include a simulation preview without hardware output
and a separately enabled live preview. Provide a starter mood with steady warm
front lighting and slowly drifting blue/purple back lighting.

### 3. Show page

High-contrast, large touch targets for 8-12 mood slots, persistent Blackout and
Break controls, master brightness, tap tempo/BPM, active/pending mood indication,
and controller/browser connection status. Avoid confirmations for normal
performance actions. Make blackout visibly latched and its release deliberate.

An edit lock prevents accidental navigation into configuration. It does not
claim to be authentication. Avoid reliance on hover, tiny sliders, or colour
alone to indicate state. Verify landscape and portrait phone/tablet layouts.
On browser connection loss, show a prominent disconnected state; do not queue
performance commands for later replay.

## Safety and operational boundaries

No strobe behavior is generated in release one. Profile authors must use fixture
manuals to identify safe shutter/macro values: arbitrary DMX profiles cannot be
made safe by assuming every unused channel should be zero.

UV is opt-in and defaults to zero. A software intensity limit is not a guarantee
of safe UV exposure; fixture guidance and venue practices still apply.

The app cannot guarantee blackout if the Pi loses power, Ethernet is unplugged,
or the Art-Net node stops receiving packets. Identify and configure the node's
DMX-loss behavior during commissioning and physically verify it. Recommend a
hardware-accessible means of stopping light output.

On graceful shutdown, attempt to send safe blackout frames, but never treat
that as a substitute for node-side loss-of-signal behavior.

## Implementation milestones and commits

Complete and commit each coherent milestone after its relevant checks pass.
Use additional small commits within a milestone when useful; do not commit
unrelated changes, generated build outputs, or deployment-specific secrets.

| Milestone | Deliverable | Acceptance gate |
|-----------|-------------|-----------------|
| 0. Planning | Record interview decisions and implementation sequence | This document committed; hardware unknowns listed |
| 1. Skeleton | Go service, bundled React UI, shared API contract, local development scripts | Serve the built app offline; produce an ARMv7 binary |
| 2. Configuration | Profiles, groups, loadouts, validated patching, atomic persistence, import/export | Invalid/overlapping patches rejected; backup round-trip and restart recovery |
| 3. Engine | Static/palette moods, intensity drift, colour mixing, tap tempo, crossfades, blackout | Deterministic engine tests cover timing, bounds, continuity, and safety overrides |
| 4. Art-Net | Single-universe unicast sender and output diagnostics | Packet-level receiver tests; real node emits expected steady colours and blackout |
| 5. Editors | Profile/loadout and mood editors, simulation and guarded live preview | Complete a rig and reusable moods through the UI without editing files |
| 6. Performance UI | Show slots, Break, master level, tap tempo, operator ownership, edit lock | Touchscreen operation and reconnect behavior work without interrupting output |
| 7. Appliance deployment | OS setup guide, service installation, backup/restore and commissioning checklist | Pi boots blacked out and serves the app without internet; live-rig rehearsal |

Dependencies: configuration precedes engine integration; engine precedes live
Art-Net integration; both precede live preview and performance acceptance.
The hardware-free UI skeleton and simulation can be developed before obtaining
the controller model.

## Verification strategy

- Backend unit tests: profile/address validation, schema import, atomic-save
  recovery, RGBWA bounds and fallback, intensity applied once, UV scaling,
  independent deterministic drift, tap tempo, phase-preserving tempo changes,
  interrupted fades, blackout precedence, and startup blackout.
- Art-Net integration tests: capture local UDP packets and assert destination,
  addressing, packet format, frame contents, and blackout output.
- UI tests: configuration creation, error handling, mood selection, Break,
  blackout latch/release, disconnected controls, and backup import/export.
- End-to-end tests with a simulated node: browser disconnect/reconnect, operator
  takeover, process restart, preview cleanup, and patch changes under blackout.
- Pi acceptance: sustained output for at least a full rehearsal, responsive
  touch controls, stable memory, and measured frame timing under UI activity.
- Physical commissioning: representative RGB/RGBA/RGBWAUV fixtures, safe shutter
  values, actual colour mixing, dimming, node universe mapping, and unplug/reboot
  behavior. Software tests cannot replace these hardware checks.

Add test tooling with the corresponding implementation, not as planning work.
Keep tests focused on introduced behavior and use the smallest relevant checks
for each commit.

## Later releases

1. Moving heads: extend profiles with 8/16-bit pan/tilt, calibrated limits,
   patterns and bounded random movement. Add fixture-specific beam/shutter
   handling and commissioning before enabling motion on a live rig.
2. Microphone tempo: compare a Pi-connected USB audio input with browser audio.
   Remote browser microphone capture normally requires HTTPS and permission;
   the initial HTTP LAN deployment must not assume it will work. Evaluate
   local HTTPS or Pi-side capture before choosing. Use confidence gating,
   smoothing, manual override and free-run fallback; never map audio transients
   directly to flashes.
3. Bluetooth MIDI: validate Android/browser support with the actual pedal.
   Web MIDI plus Bluetooth support is not universal; consider a native Android
   bridge or Pi-side MIDI input only if needed.
4. Haze: add dedicated fixture semantics, explicit enable/disable, maximum
   output, and a timed cutoff. Do not treat haze as another colour channel or
   send it through random mood intensity.
5. Optional enhancements: bounded colour ranges, fixture overrides, calibration,
   Art-Net discovery, and additional universes only when required.

## Hardware details to resolve before live output

- Exact Pi 2 revision, compatible OS image, power supply, and storage condition.
- Art-Net controller model, IP/subnet, Port-Address mapping, and DMX-loss options.
- Fixture manuals and channel modes, especially shutter/strobe/macro channels.
- Router address reservation for the Pi and node; provide an IP fallback if
  local hostname resolution is unavailable.

Unknown hardware details do not block the configuration UI or simulated engine.
They do block declaring the system ready for a live show.
