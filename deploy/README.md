# Raspberry Pi deployment

This development release supports mood simulation and explicitly enabled
ENTTEC DMX USB PRO output. Art-Net is not implemented. Completing this installation
does not make the system show-ready; physical commissioning remains required.

## Prepare the appliance

Use Raspberry Pi Imager to install a currently supported Raspberry Pi OS Lite
image compatible with the exact Pi 2 revision. Prefer a 32-bit image. Configure
a unique login and SSH access in Imager, and connect the Pi by Ethernet to the
existing router. No desktop, Wi-Fi adapter, or internet connection at a show is
required.

Reserve the Pi's address in the router. Keep a note of that address; local
hostname resolution is not guaranteed on every phone. The app listens on port
8080. Do not expose it through router port forwarding or to an untrusted network.

## Build on Windows

Install Go and Node.js on the development machine, then from the repository root:

```powershell
.\scripts\build.ps1 -Target pi2
```

The script installs locked frontend dependencies, checks both applications,
bundles the UI, and cross-compiles a Linux ARMv7 binary with no C runtime
dependency. Internet is needed on the development machine for initial dependency
installation, not on the Pi at show time.

Copy `build\stageglow-linux-armv7` and `deploy\stageglow.service` to the Pi using
your SSH client's file-transfer feature. The Pi does not need Go, Node.js, npm,
or a separate web server.

## Install on the Pi

The following commands run **on the Linux Pi**, assuming the transferred files
are in your login user's home directory:

```sh
sudo useradd --system --user-group --home-dir /var/lib/stageglow --shell /usr/sbin/nologin stageglow
sudo usermod -a -G dialout stageglow
sudo install -d -o root -g root -m 755 /opt/stageglow
sudo install -o root -g root -m 755 ~/stageglow-linux-armv7 /opt/stageglow/stageglow
sudo install -o root -g root -m 644 ~/stageglow.service /etc/systemd/system/stageglow.service
sudo systemctl daemon-reload
sudo systemctl enable --now stageglow
sudo systemctl status stageglow --no-pager
```

Create the service user only on the first installation. systemd creates and owns
the writable state directory `/var/lib/stageglow`; the application directory
remains read-only to the service.

Open `http://<pi-address>:8080` from a phone or tablet on the same router. The
page must report USB output disabled at startup. Verify it
works after reboot and without the router's internet uplink.

For diagnosis:

```sh
journalctl -u stageglow -n 100 --no-pager
curl http://localhost:8080/api/health
```

If a host firewall is enabled, allow TCP 8080 only from the trusted show subnet.
Art-Net UDP firewall rules are not needed for this release.

## ENTTEC USB setup and commissioning

Connect the original **ENTTEC DMX USB PRO** to the Pi, not the phone. The normal
Raspberry Pi OS FTDI virtual COM driver should provide `/dev/ttyUSB0` and a stable
`/dev/serial/by-id/` symlink. Install the appropriate FTDI virtual COM driver if
testing on Windows and no COM port appears. Open DMX USB is a different product
and is unsupported; Mk2 and RDM operation are not targeted.

Inspect the Pi devices:

```sh
ls -l /dev/serial/by-id/
ls -l /dev/ttyUSB0
id stageglow
```

The service user needs read/write access to the serial device. On Raspberry Pi
OS that normally means membership of `dialout`, added above. Restart the service
after changing group membership. If the device has another owning group, grant
only that group to the service; do not make devices world-writable or run the
service as root. The unit intentionally does not use `PrivateDevices=true`,
which would hide the USB device. Close other lighting apps using the same port.

In Fixtures > Lighting output, choose ENTTEC, refresh ports, select the stable
`/dev/serial/by-id/` entry, apply, and wait for Saved. Device listing does not
identify the model; check the physical label. The read-only USB PRO handshake
checks protocol compatibility, not brand identity. USB settings are stored only
on this appliance and are stripped from downloaded/imported backups.

Commission with a representative fixture at low brightness and UV off:

1. Verify channel modes and all shutter/strobe/macro safe values from manuals.
   Simulate static colours and an evolving mood before enabling hardware.
2. Enable USB and verify darkness before releasing blackout. Select a mood,
   reduce master, and confirm the explicit physical blackout release.
3. Compare RGB/white/amber, intensity, UV (only with suitable precautions),
   palette drift, tempo changes, and crossfades against the software values.
   USB PRO repeats at its configured refresh rate; host updates are 30 Hz.
   The app does not change persistent break/MAB/refresh parameters.
4. Verify blackout stays latched across mood selections and while enabling.
   Verify disable and normal service stop send profile-safe blackout, including
   fixtures that need nonzero fixed-channel blackout values.
5. While a safe dim mood runs, close the browser and reconnect: output should
   continue. Unplug USB: the app should show a fault, never auto-reconnect, and
   require explicit enablement after cleanup and reconnection.
6. Test application crash, Pi power loss, USB removal, and fixture DMX-loss
   behavior with a physical means to stop light output ready. A buffered
   interface can keep repeating its last frame; do not assume port closure
   extinguishes lights.
7. Restart the service and Pi: the app must return disabled and blacked out,
   with no mood selected. Restore a backup and confirm output returns to
   simulation with a blank port.
8. Run several moods for a sustained session on the Pi 2 and check smoothness,
   USB errors, temperature, and responsiveness with and without a browser open.
9. Configure the Show grid and low-light Pause mood. Rehearse the fixed
   Blackout/Pause/Tap Tempo controls, deliberate blackout release, master level,
   screen rotation, and reconnects from your actual phone/tablet. Verify the
   Pause mood is suitably dim on physical fixtures; its default 10% is only
   a starting point, not calibrated stage brightness.

If a write or open times out, output disarms and cleanup blocks new connections.
Wait for cleanup before retrying. A stuck driver or failed close can require
service restart and cable/driver inspection. Frames-written counters confirm only
host writes, not fixture delivery. These physical checks have not been performed
as part of the software implementation.

## Backup, restore, and update

Use the app's configuration export and store a copy away from the Pi. Imports
are validated before replacing configuration. Keep a backup before updating the
application; a future release may add configuration schema migrations.

For a complete filesystem backup, stop the service before copying its state
directory, then start it again:

```sh
sudo systemctl stop stageglow
# Copy /var/lib/stageglow to your chosen backup destination.
sudo systemctl start stageglow
```

If configuration is corrupt, the service refuses to start rather than silently
replacing your rig. Preserve the affected files, inspect the logs, and restore
a known-good backup while the service is stopped.

To update the binary, stop the service, install the replacement binary as above,
and start it again. Keep the previous binary for rollback.

## Before venue Art-Net output

Identify the Art-Net controller, configure its destination address and universe
mapping, and check every fixture's manual for safe channel defaults. Physically
test node-side DMX-loss behavior. Neither service restart nor browser controls
can guarantee blackout after power or network loss.
