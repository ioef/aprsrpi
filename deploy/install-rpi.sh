#!/usr/bin/env bash
set -euo pipefail

APP_ROOT="${APRSRPI_ROOT:-/opt/aprsrpi}"
APP_USER="${APRSRPI_USER:-pi}"
CONFIG_DIR="/etc/aprsrpi"
SERVICE_NAME="aprsrpi.service"
SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
SOURCE_ROOT="$(dirname -- "$SCRIPT_DIR")"

if [[ "$(uname -m)" != "aarch64" ]]; then
  echo "This installer targets 64-bit Raspberry Pi OS (aarch64)." >&2
  echo "Detected: $(uname -m)" >&2
  exit 1
fi

if [[ "$(id -u)" -ne 0 ]]; then
  exec sudo -E env APRSRPI_ROOT="$APP_ROOT" APRSRPI_USER="$APP_USER" "$0" "$@"
fi

if ! id "$APP_USER" >/dev/null 2>&1; then
  echo "User '$APP_USER' does not exist. Set APRSRPI_USER to an existing Pi user." >&2
  exit 1
fi

USER_HOME="$(getent passwd "$APP_USER" | cut -d: -f6)"

echo "Installing Raspberry Pi packages..."
apt-get update
DEBIAN_FRONTEND=noninteractive apt-get install -y chromium nodejs npm golang-go python3 bluez bluez-tools

echo "Building Vue frontend..."
cd "$SOURCE_ROOT/web"
npm install
npm run build

echo "Building Go backend..."
cd "$SOURCE_ROOT"
go build -o "$SOURCE_ROOT/aprsrpi" .

echo "Installing application into $APP_ROOT..."
install -d -o "$APP_USER" -g "$APP_USER" "$APP_ROOT"
install -d -o "$APP_USER" -g "$APP_USER" /var/log/aprsrpi
cp -a "$SOURCE_ROOT/web" "$APP_ROOT/"
install -m 755 -o "$APP_USER" -g "$APP_USER" "$SOURCE_ROOT/aprsrpi" "$APP_ROOT/aprsrpi"

install -d -m 755 "$CONFIG_DIR"
if [[ ! -e "$CONFIG_DIR/config.json" ]]; then
  CONFIG_SOURCE="$SOURCE_ROOT/config.json"
  if [[ ! -e "$CONFIG_SOURCE" ]]; then
    CONFIG_SOURCE="$SOURCE_ROOT/config.native.example.json"
    echo "No prepared config.json found; installing the native example."
  fi
  python3 -m json.tool "$CONFIG_SOURCE" >/dev/null
  install -m 640 -o "$APP_USER" -g "$APP_USER" "$CONFIG_SOURCE" "$CONFIG_DIR/config.json"
  echo "Installed $CONFIG_SOURCE as $CONFIG_DIR/config.json"
else
  echo "Keeping existing $CONFIG_DIR/config.json"
fi

sed "s/^User=.*/User=$APP_USER/" "$SOURCE_ROOT/deploy/aprsrpi.service" > "/etc/systemd/system/$SERVICE_NAME"
chmod 644 "/etc/systemd/system/$SERVICE_NAME"
systemctl daemon-reload
systemctl enable --now "$SERVICE_NAME"

install -d -o "$APP_USER" -g "$APP_USER" "$USER_HOME/.config/autostart"
cat > "$USER_HOME/.config/autostart/aprsrpi-kiosk.desktop" <<EOF
[Desktop Entry]
Type=Application
Name=APRS RPi Kiosk
Exec=chromium --kiosk --noerrdialogs --disable-infobars http://localhost:8080
Terminal=false
X-GNOME-Autostart-enabled=true
EOF
chown "$APP_USER:$APP_USER" "$USER_HOME/.config/autostart/aprsrpi-kiosk.desktop"
chmod 644 "$USER_HOME/.config/autostart/aprsrpi-kiosk.desktop"

echo
echo "Installation complete."
echo "Edit $CONFIG_DIR/config.json if needed."
echo "Configure a bidirectional KISS TNC: Mobilinkd or APRS Voyager via USB/Bluetooth."
echo "Check status with: systemctl status $SERVICE_NAME"
