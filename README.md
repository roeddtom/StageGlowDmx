# StageGlowDmx

Local, mood-based stage lighting over Art-Net, operated from a touchscreen
browser and hosted on a Raspberry Pi.

**Current stage: mood generation and ENTTEC / Eurolite MK2 USB home output.**
USB hardware commissioning is still required. A full-screen Show grid is
available; Art-Net is not implemented and the system still needs rehearsal
and physical acceptance before live use.

Fixture profiles, groups, loadouts, channel patching, Art-Net destination settings,
and configuration backups are implemented. The mood editor and server-side
generator support static lighting, independent palette/intensity drift,
RGB/white/amber mixing, separate UV, tempo divisions, and smooth mood transitions.
Explicitly enabled USB output uses the same generated frames as the simulation.
Art-Net venue output remains deferred until after home testing and adjustments.

## Run locally

Install Go and Node.js 24 LTS or newer. On Windows, from this directory:

```powershell
.\scripts\build.ps1
.\build\stageglow.exe -listen 127.0.0.1:8080 -data .\data
```

Open `http://localhost:8080`. Configuration is saved locally in the selected data
directory. Use a single configuration browser at a time; multi-operator ownership
is a later milestone. Do not expose this trusted-LAN application to the internet.

For frontend development, run the Go service, then in a second terminal:

```powershell
Set-Location .\web
npm ci
npm run dev
```

Vite proxies `/api` requests to the Go service on port 8080.
After installing dependencies, `-SkipInstall` skips `npm ci` on subsequent builds.

Frontend tests use Node's built-in test runner.

## Create and simulate moods

1. Define profiles and groups, patch fixtures into a loadout, and select that
   loadout as active on the Fixtures page.
2. Open Moods and create a starter or a new mood. The starter keeps groups named
   Front steady and gives other groups a blue/purple palette. Edit colours,
   intensity bounds, UV, beats per change, smoothness, and crossfade duration.
   Equal intensity bounds hold brightness steady; excluded groups use the
   selected Base, or stay dark without one.
   Intensity, smoothness, and crossfade have sliders with precise numeric inputs.
   Intensity is shown as a percentage; the **?** buttons explain each control.
3. Apply the mood and wait for Saved. Select it in the mood list, then explicitly
   start the simulation. Inspect fixture colours and generated DMX values.

Use global BPM or tap tempo to control the flow. For example, 8 beats at 120 BPM
means a new target every 4 seconds. Smoothness 1 glides for the whole interval;
lower values glide sooner and then hold. Palette targets and intensity targets
are chosen independently for each fixture. White/amber extraction is approximate
and controlled by the fixture profile; UV follows intensity and master brightness.

On the Moods editor page, simulation blackout is immediate and stays latched
across preview selections. In Show, pressing a Mood, Base or Pause button
selects it and releases blackout immediately.
Releasing it fades into the selected mood; it does not enable physical output.
The server uses elapsed time rather than browser timers, so reconnecting does
not restart the mood. Process restarts clear the selected mood and start blacked
out. Patch changes and configuration imports also latch simulation blackout.
Schema-1 backups from the configuration-only release load with an empty mood list.

### Base and Atmosphere lighting

Each mood has a **Mood type**: Atmosphere (the default, including all older
backups) or Base. One of each can be selected independently.

- **Base** provides static fallback lighting: one colour and fixed intensity per
  included group. Create looks such as "Band front" and "Announcer", including
  only their relevant groups.
- **Atmosphere** controls its included groups, overriding the Base rather than
  adding brightness. Exclude Front from atmospheric moods to keep front lighting
  unchanged. Explicitly including Front at 0% makes it dark, even with a Base.
- Switching either layer crossfades without restarting the other layer's timing.
  **No base** fades out the base using its current crossfade duration and leaves
  the Atmosphere running. Uncovered groups become dark.

Both selections have independent selected indicators. In Show, Base buttons use
a **BASE** label and dashed border; they occupy any ordinary grid position you
assign. The compact top strip includes **No base**.

Pause must be an Atmosphere. It leaves the Base selected, but its included groups
still override the Base: exclude Front from Pause to preserve Base front lighting.
The supplied Pause starter includes every group, so adjust its group assignments
when using it with a Base.

Blackout overrides both layers. Moods-editor selections made while blacked out
remain pending; Show mood buttons instead select and resume immediately.
Releasing blackout fades in their combination over the longer selected
crossfade duration. Master applies to the combined output once. Base-only
lighting works without selecting an Atmosphere. Neither selection persists after
restart. Deleting a selected mood or changing its type clears that layer and
latches blackout; changing a Pause mood to Base clears its Pause assignment.

### Algam Lighting SLIMPAR1210 QUAD

On Fixtures, select **Algam Lighting SLIMPAR1210 QUAD (8ch)** and apply the
profile, then assign it to fixtures at their actual DMX addresses. Set the light
to DMX mode; this profile uses its eight-channel map:

| Channel | Function | Control |
|---|---|---|
| 1 | Master dimmer | Mood intensity and global master |
| 2-5 | Red, Green, Blue, White | Mood colour with white extraction |
| 6 | Strobe | Fixed 0 (off) |
| 7 | Effect | Fixed 0 (normal dimming, no built-in program) |
| 8 | Speed | Fixed 0 (unused without a built-in effect) |

Strobe, Effect and Speed remain separate fixed profile settings (offsets 5-7),
not evolving mood controls. Their blackout values are also 0. Leave them at
these defaults for gentle lighting. Editing a shared profile affects every
fixture using it; disable USB before changing the channel map or fixed values.
See the [manual's DMX chart](https://www.manualslib.com/manual/3524258/Algam-Lighting-Slimpar1210-Quad.html?page=16).

### Linked-emitter bar profiles

The fixture templates also include:

| Profile | Mode | Mood control | Fixed at 0 |
|---|---|---|---|
| Algam Lighting BARWASH 244 | 8ch **H001**, not E001 | Master + RGBW | Strobe, colour macros/programs, function speed |
| Eurolite LED CBB-2 COB RGB Bar | 11ch | Master + identical RGB on both emitters | Strobe, both colour presets, internal programs |
| Eurolite LED CBB-2 COB WW Bar | 5ch | Master + linked warm-white emitters | Strobe, internal programs |

Both emitters are treated as one fixture, not independent zones. Repeated
colour/white channels receive the same generated value. Only one channel is
the master dimmer; on the WW bar its Left and Right dimmers are mapped as
`white`, held fully open while the master controls brightness. White-only
fixtures ignore mood colours (even black) and use mood intensity instead;
set intensity to zero or use blackout to darken them. Their preview uses
neutral white to indicate brightness, not calibrated colour temperature.

Strobe and built-in programs remain separate fixed profile values, outside mood
control, with zero blackout values. Verify these no-effect defaults against
your fixture's manual/revision before enabling USB. The Barwash H001 map is
documented in its [manual](https://www.manualslib.com/manual/3457847/Algam-Lighting-Barwash-244.html?page=22);
the Eurolite maps follow the supplied channel order.

## Test moods through USB

Connect an original **ENTTEC DMX USB PRO** or **Eurolite USB-DMX512 PRO MK2** to the computer running the Go service
and its DMX output to your fixtures. This is server-side serial output, not
browser WebUSB; ENTTEC Open DMX USB is unsupported.

1. Configure and simulate the same fixture profiles, loadout, and moods first.
   Verify each fixture manual's channel mode and safe fixed values.
2. In Fixtures > Lighting output, select the exact interface model, refresh serial ports,
   choose the correct device, and apply settings. Windows uses a port such as
   `COM3`; on the Pi prefer a stable `/dev/serial/by-id/` path.
3. Wait for Saved, then explicitly enable USB output. The first frame is blacked
   out, even if a simulation was running.
4. In Moods, select a mood, lower master brightness, and confirm **Release USB
   blackout (real lights)**. Controls and saved mood edits now affect real lights.
5. Use Blackout at any time, or **Black out & disable USB** before repatching or
   disconnecting. Faults disarm USB and require an explicit retry after cleanup.

The server sends full frames at 30 Hz independently of browser activity. The
interface repeats DMX at its own configured refresh rate; the application does
not modify its persistent timing settings. Check the device's rate when
commissioning smooth fades. A protocol handshake and successful writes do not
prove that fixtures received the intended output.

**Eurolite MK2 is a separate output mode**, not an ENTTEC device. It uses
250,000 baud and the same full-universe label-6 framing, but does not support the
ENTTEC widget-parameter handshake used by our ENTTEC mode. Selecting ENTTEC for
this interface causes an initialization timeout. The MK2's generic FTDI port
name cannot identify its model: select it explicitly from its physical label.
Its enabled status confirms serial writes only, not device or fixture feedback.
The original Eurolite PRO (non-MK2) and Open DMX USB are not targeted.

Saved output settings never auto-enable USB after restart. Exports omit the
machine-local port and reset output mode to simulation; imports do the same.
Old schema-1 configurations without output settings default to simulation.

**A fault may leave lights on.** USB PRO buffers its last frame. Browser loss
does not stop lighting; USB disconnection, application crash, or power loss
cannot guarantee blackout. Test physical loss behavior and keep a hardware means
of stopping output available. See the [commissioning checklist](deploy/README.md).

## Run a show

1. On **Show**, choose 8-12 mood buttons and assign their moods in order.
   Choose a dedicated low-light **Pause** mood, or create the supplied starting
   point (static warm lighting at 10% intensity with UV off). Adjust its settings
   in Moods for your fixtures and stage. Unassigned buttons are disabled.
2. Wait for Saved and choose **Enter show mode**. The grid fills the browser
   viewport; setup headers, navigation, and backups are hidden. **Fullscreen**
   optionally hides browser chrome where supported.
3. Use the fixed bottom row: **Blackout**, **Pause**, **Tap tempo**. Blackout is
   immediate, but is **not latched against Show selections**: tapping any Mood,
   Base or Pause button selects it and resumes lighting through its crossfade.
   Tap **Blackout** again to restore the existing combination instead. Neither
   action requires a separate release or confirmation, including for physical USB.
4. Master brightness stays in the compact top strip. Tap repeatedly to set
   global BPM; tempo autosaves without locking the performance buttons.
   While blacked out, the previous selections show "Tap to resume".
5. **Exit show** requires confirmation to unlock editing and does not change
   lighting. Assignments and Pause selection persist in backups, but performance
   mode, master brightness, selected Base/Atmosphere, and USB enablement do not persist
   across process restart.

Entering Show never enables USB or releases blackout. Enable the interface on
Fixtures first for real output; otherwise Show controls the simulation. A USB
fault disables normal performance actions until addressed outside Show.
Connection loss shows unknown output state and disables mood/tempo controls;
commands are not queued for replay. Server lighting continues independently of
the browser. Blackout remains available as a best-effort command.

Use one operator. The Show editing lock prevents accidental local navigation,
not changes from another browser; multi-operator ownership is not implemented.

### Browser microphone tempo

In Show, toggle **Mic OFF / ON** and grant microphone access. Choose **Kick**
(40-180 Hz, default), **Snare** (700-5000 Hz), or **Full band**. These frequency
bands emphasize likely drum transients; they do not isolate instruments from
a live mix. Audio is analysed only in the browser, never recorded, uploaded,
or played through the speakers. Only the resulting BPM is saved to StageGlow.

Detection stays within **50-180 BPM**. Slower estimates are doubled until in
range; faster estimates are halved until in range (40 becomes 80, 200 becomes
100). Manual/tap tempo keeps its existing range. Detection needs several consistent beats
(typically 5-10 seconds), suppresses rapid retriggers, smooths small adjustments,
and updates global tempo at most once every five seconds. The mood's existing
beats-per-change setting still determines how slowly the lighting moves.
Silence or uncertain rhythm holds the last tempo; it never creates flashes.
Half/double-time ambiguity is resolved toward the current BPM, so tap an
approximate tempo first if the detector settles on the wrong octave.

**Tap tempo switches the microphone off** for immediate manual override.
Turning Mic off retains the last BPM. Exiting Show, hiding the page, losing
the server connection, an output fault or a microphone interruption also
stops capture. Re-enable deliberately; mic state is never persisted. Keep the
browser foregrounded during use, and rehearse detection with the actual band
and microphone placement: a clean kick/snare signal works best.

Microphone capture requires **localhost or HTTPS with a certificate trusted by
the browser/device**. `http://localhost:28092` works on the computer running the
app; `http://<pi-address>:8080` generally cannot use a phone's microphone.
Optional `-tls-cert` and `-tls-key` flags serve HTTPS directly; see
[local HTTPS setup](deploy/README.md#local-https-for-browser-microphone).
There are no cloud services or internet requirements at show time.

## Raspberry Pi

```powershell
.\scripts\build.ps1 -Target pi2
```

See [deployment instructions](deploy/README.md) for installing the self-contained
ARMv7 binary as a boot service on Raspberry Pi OS Lite. The Pi needs no Node.js
runtime and no internet connection to serve the application.

## Plan

For a code-oriented introduction, see [program flow and Go fundamentals](PROGRAM_FLOW.md).

See [the implementation plan](IMPLEMENTATION_PLAN.md) for the agreed first-release
scope, architecture, safety behavior, and delivery milestones.