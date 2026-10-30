# Raspberry Pi deployment

This development release supports mood simulation and explicitly enabled
ENTTEC DMX USB PRO and Eurolite USB-DMX512 PRO MK2 output. Art-Net is not implemented. Completing this installation
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

## Local HTTPS for browser microphone

HTTP on `localhost` supports microphone capture for a browser on the same
computer as StageGlow. Phones/tablets visiting the Pi's HTTP address generally
cannot grant microphone access: browsers require a secure context.

StageGlow can serve HTTPS directly:

```powershell
.\build\stageglow.exe -listen :8443 -data .\data -tls-cert C:\StageGlowTLS\stageglow.pem -tls-key C:\StageGlowTLS\stageglow-key.pem
```

Both TLS flags are required together. Without them the existing HTTP behavior
is unchanged. Use a certificate covering the actual hostname/IP you visit,
trusted on every controlling device. For example, with the external `mkcert`
tool installed on your development computer, generate certificates **outside
the repository** (replace the sample IP with your Pi's reserved address):

```powershell
New-Item -ItemType Directory -Force C:\StageGlowTLS
Set-Location C:\StageGlowTLS
mkcert -install
mkcert -cert-file stageglow.pem -key-file stageglow-key.pem stageglow.local 192.168.1.50 localhost 127.0.0.1
mkcert -CAROOT
```

Install the resulting CA's **root certificate** (`rootCA.pem` in the printed
CA directory) as a trusted CA on each phone/tablet according to its OS instructions;
iOS also requires enabling full trust. Never copy or share `rootCA-key.pem`.
Copy only the server certificate and server key to the Pi. Keep all private
keys out of Git/backups and restrict the server key to the service account.
For example, store the pair in `/etc/stageglow/tls`, with the key owned by
root, group `stageglow`, mode `640` and the directory mode `750`.

Override the systemd `ExecStart` to use the paths:

```ini
[Service]
ExecStart=
ExecStart=/opt/stageglow/stageglow -listen :8443 -data /var/lib/stageglow -tls-cert /etc/stageglow/tls/stageglow.pem -tls-key /etc/stageglow/tls/stageglow-key.pem
```

Restart the service and visit `https://<pi-address>:8443`, allowing TCP 8443 on
the trusted show subnet if needed. Verify the browser shows a trusted connection
and lets Mic request permission before rehearsal. A certificate-warning bypass
is not a reliable microphone setup. No internet is required once certificates
and trust are installed. HTTPS here enables microphone access; it does not add
operator authentication or make the application suitable for public exposure.

The browser analyses its own microphone, not a microphone plugged into the Pi.
Mic is opt-in in Show and turns off on exit, page hiding or connection loss.
Last BPM remains active when detection stops.

## USB setup and commissioning

Connect the original **ENTTEC DMX USB PRO** or **Eurolite USB-DMX512 PRO MK2** to the Pi, not the phone. The normal
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

In Fixtures > Lighting output, choose the exact interface model, refresh ports, select the stable
`/dev/serial/by-id/` entry, apply, and wait for Saved. Device listing does not
identify the model; check the physical label. The read-only USB PRO handshake
checks protocol compatibility, not brand identity. USB settings are stored only
on this appliance and are stripped from downloaded/imported backups.

Eurolite MK2 uses 250,000 baud and output framing without the ENTTEC read-only
handshake. Its generic FTDI USB identity is not enough to auto-detect it; select
Eurolite MK2 explicitly from the physical model label. ENTTEC mode times out on
this device. Do not replace the virtual COM driver with a libusb driver: this
application uses the normal serial device for both models.

Commission with a representative fixture at low brightness and UV off:

1. Verify channel modes and all shutter/strobe/macro safe values from manuals.
   Simulate static colours and an evolving mood before enabling hardware.
2. Enable USB and verify darkness before releasing blackout. Select a mood,
   reduce master, and confirm the explicit physical blackout release.
3. Compare RGB/white/amber, intensity, UV (only with suitable precautions),
   palette drift, tempo changes, and crossfades against the software values.
   USB PRO repeats at its configured refresh rate; host updates are 30 Hz.
   The app does not change persistent break/MAB/refresh parameters.
4. Verify USB enablement starts blacked out, and Moods-editor selections retain
   blackout. In Show, verify Mood/Base/Pause buttons immediately resume lighting
   from blackout without a second tap.
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
host writes, not fixture delivery. Full fixture and loss-behavior commissioning
remains required; successful host writes alone do not complete these checks.

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
