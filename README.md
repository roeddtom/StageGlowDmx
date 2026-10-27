# StageGlowDmx

Local, mood-based stage lighting over Art-Net, operated from a touchscreen
browser and hosted on a Raspberry Pi.

**Current stage: configuration only. No DMX or Art-Net is emitted.**

Fixture profiles, groups, loadouts, channel patching, Art-Net destination settings,
and configuration backups are the first implementation slice. Mood generation,
live output, and performance controls follow in later milestones.

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