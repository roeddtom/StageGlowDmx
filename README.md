# StageGlowDmx

Local, mood-based stage lighting over Art-Net, operated from a touchscreen
browser and hosted on a Raspberry Pi.

**Current stage: mood simulation. No DMX or Art-Net is transmitted.**

Fixture profiles, groups, loadouts, channel patching, Art-Net destination settings,
and configuration backups are implemented. The mood editor and server-side
generator support static lighting, independent palette/intensity drift,
RGB/white/amber mixing, separate UV, tempo divisions, and smooth mood transitions.
Hardware output and the performance show page follow in later milestones.

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

## Raspberry Pi

```powershell
.\scripts\build.ps1 -Target pi2
```

See [deployment instructions](deploy/README.md) for installing the self-contained
ARMv7 binary as a boot service on Raspberry Pi OS Lite. The Pi needs no Node.js
runtime and no internet connection to serve the application.

## Plan

See [the implementation plan](IMPLEMENTATION_PLAN.md) for the agreed first-release
scope, architecture, safety behavior, and delivery milestones.