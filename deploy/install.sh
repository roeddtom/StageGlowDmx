#!/bin/sh
set -eu

cd "$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"

if [ "$(id -u)" -ne 0 ]; then
    echo "Run with sudo: sudo sh ./install.sh" >&2
    exit 1
fi

case "$(uname -m)" in
    armv7l|armv8l) ;;
    *)
        echo "This bundle requires 32-bit ARMv7 Raspberry Pi OS (Pi 2 or newer)." >&2
        echo "Install the 32-bit OS image; ARMv6 and 64-bit OS builds are not included." >&2
        exit 1
        ;;
esac

for command in systemctl sha256sum install getent useradd usermod; do
    command -v "$command" >/dev/null
done
getent group dialout >/dev/null
sha256sum -c SHA256SUMS

if ! id stageglow >/dev/null 2>&1; then
    useradd --system --user-group --home-dir /var/lib/stageglow --shell /usr/sbin/nologin stageglow
fi
usermod -a -G dialout stageglow
install -d -o root -g root -m 755 /opt/stageglow

if [ -f /etc/systemd/system/stageglow.service ]; then
    systemctl stop stageglow
fi
if [ -f /opt/stageglow/stageglow ]; then
    install -o root -g root -m 755 /opt/stageglow/stageglow /opt/stageglow/stageglow.previous
fi
install -o root -g root -m 755 stageglow-linux-armv7 /opt/stageglow/stageglow
install -o root -g root -m 644 stageglow.service /etc/systemd/system/stageglow.service
systemctl daemon-reload
systemctl enable --now stageglow
systemctl --no-pager --full status stageglow
echo "Installed. USB remains disabled until explicitly enabled in the browser."
echo "See README.md for rig import, HTTPS microphone setup and commissioning."
