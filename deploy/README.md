# Raspberry Pi deployment

This is a configuration and mood-simulation development release. It does not
transmit DMX or Art-Net. Completing this installation does not make the system
show-ready.

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
page must clearly report that simulation does not enable Art-Net output. Verify it
works after reboot and without the router's internet uplink.

For diagnosis:

```sh
journalctl -u stageglow -n 100 --no-pager
curl http://localhost:8080/api/health
```

If a host firewall is enabled, allow TCP 8080 only from the trusted show subnet.
Art-Net UDP firewall rules are not needed for this simulation-only release.

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

## Before any future live-output release

Identify the Art-Net controller, configure its destination address and universe
mapping, and check every fixture's manual for safe channel defaults. Physically
test node-side DMX-loss behavior. Neither service restart nor browser controls
can guarantee blackout after power or network loss.
