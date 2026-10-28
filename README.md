# StageGlowDmx

Local, mood-based stage lighting over Art-Net, operated from a touchscreen
browser and hosted on a Raspberry Pi.

**Current stage: mood generation and ENTTEC DMX USB PRO home output.**
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
   Equal intensity bounds hold brightness steady; excluded groups stay dark.
   Intensity, smoothness, and crossfade have sliders with precise numeric inputs.
   Intensity is shown as a percentage; the **?** buttons explain each control.
3. Apply the mood and wait for Saved. Select it in the mood list, then explicitly
   start the simulation. Inspect fixture colours and generated DMX values.

Use global BPM or tap tempo to control the flow. For example, 8 beats at 120 BPM
means a new target every 4 seconds. Smoothness 1 glides for the whole interval;
lower values glide sooner and then hold. Palette targets and intensity targets
are chosen independently for each fixture. White/amber extraction is approximate
and controlled by the fixture profile; UV follows intensity and master brightness.

Simulation blackout is immediate and stays latched across mood selections.
Releasing it fades into the selected mood; it does not enable physical output.
The server uses elapsed time rather than browser timers, so reconnecting does
not restart the mood. Process restarts clear the selected mood and start blacked
out. Patch changes and configuration imports also latch simulation blackout.
Schema-1 backups from the configuration-only release load with an empty mood list.

## Test moods through ENTTEC USB

Connect the original **ENTTEC DMX USB PRO** to the computer running the Go service
and its DMX output to your fixtures. This is server-side serial output, not
browser WebUSB; ENTTEC Open DMX USB is unsupported.

1. Configure and simulate the same fixture profiles, loadout, and moods first.
   Verify each fixture manual's channel mode and safe fixed values.
2. In Fixtures > Lighting output, select ENTTEC DMX USB PRO, refresh serial ports,
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
3. Use the fixed bottom row: **Blackout**, **Pause**, **Tap tempo**. Mood and
   Pause buttons crossfade while running. Blackout is immediate and latched;
   during blackout, mood and Pause buttons select the pending mood only.
   Tap **Blackout** again to restore the pending mood. It is a toggle, with no
   separate release button or confirmation dialog, including for physical USB.
4. Master brightness stays in the compact top strip. Tap repeatedly to set
   global BPM; tempo autosaves without locking the performance buttons.
   Active/pending labels distinguish what is selected from what is playing.
5. **Exit show** requires confirmation to unlock editing and does not change
   lighting. Assignments and Pause selection persist in backups, but performance
   mode, master brightness, selected mood, and USB enablement do not persist
   across process restart.

Entering Show never enables USB or releases blackout. Enable the interface on
Fixtures first for real output; otherwise Show controls the simulation. A USB
fault disables normal performance actions until addressed outside Show.
Connection loss shows unknown output state and disables mood/tempo controls;
commands are not queued for replay. Server lighting continues independently of
the browser. Blackout remains available as a best-effort command.

Use one operator. The Show editing lock prevents accidental local navigation,
not changes from another browser; multi-operator ownership is not implemented.

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