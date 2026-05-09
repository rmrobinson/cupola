#!/usr/bin/env bash
set -euo pipefail

APP_NAME="cupolad"
SERVICE_NAME="cupolad.service"
USER_NAME="cupolad"
GROUP_NAME="cupolad"
INSTALL_DIR="/opt/cupolad"
CONFIG_DIR="/etc/cupolad"
DATA_DIR="/var/lib/cupolad"
SYSTEMD_DIR="/etc/systemd/system"

BUILD_BINARY=0
START_SERVICE=1
OVERWRITE_CONFIG=0
BINARY_SOURCE=""

usage() {
  printf 'Usage: %s [--binary PATH | --build] [--no-start] [--overwrite-config]\n' "$0"
  printf '\n'
  printf 'Installs Cupola as a systemd service running as cupolad:cupolad.\n'
  printf '\n'
  printf 'Options:\n'
  printf '  --binary PATH       Install this prebuilt binary as /opt/cupolad/cupolad.\n'
  printf '  --build             Build ./cmd/cupola with go and install the result.\n'
  printf '  --no-start          Install files but do not enable or restart the service.\n'
  printf '  --overwrite-config  Replace /etc/cupolad/config.yaml with deploy/config.yaml.\n'
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --binary)
      if [ "$#" -lt 2 ]; then
        echo "missing path after --binary" >&2
        exit 2
      fi
      BINARY_SOURCE="$2"
      shift 2
      ;;
    --build)
      BUILD_BINARY=1
      shift
      ;;
    --no-start)
      START_SERVICE=0
      shift
      ;;
    --overwrite-config)
      OVERWRITE_CONFIG=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

if [ "$(id -u)" -ne 0 ]; then
  echo "install.sh must be run as root, for example with sudo" >&2
  exit 1
fi

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "$SCRIPT_DIR/.." && pwd)"

if [ "$BUILD_BINARY" -eq 1 ] && [ -n "$BINARY_SOURCE" ]; then
  echo "use either --binary or --build, not both" >&2
  exit 2
fi

TMP_BINARY=""
cleanup() {
  if [ -n "$TMP_BINARY" ] && [ -f "$TMP_BINARY" ]; then
    rm -f "$TMP_BINARY"
  fi
}
trap cleanup EXIT

if [ "$BUILD_BINARY" -eq 1 ]; then
  TMP_BINARY="$(mktemp "/tmp/cupolad.XXXXXX")"
  echo "building $APP_NAME from $REPO_ROOT"
  (cd "$REPO_ROOT" && go build -o "$TMP_BINARY" ./cmd/cupola)
  BINARY_SOURCE="$TMP_BINARY"
elif [ -z "$BINARY_SOURCE" ]; then
  if [ -x "$REPO_ROOT/cupola" ]; then
    BINARY_SOURCE="$REPO_ROOT/cupola"
  elif [ -x "$REPO_ROOT/cupolad" ]; then
    BINARY_SOURCE="$REPO_ROOT/cupolad"
  else
    echo "no binary found. Run make build first, or use --build, or pass --binary PATH" >&2
    exit 1
  fi
fi

if [ ! -f "$BINARY_SOURCE" ]; then
  echo "binary does not exist: $BINARY_SOURCE" >&2
  exit 1
fi

if ! getent group "$GROUP_NAME" >/dev/null; then
  groupadd --system "$GROUP_NAME"
fi

if ! id -u "$USER_NAME" >/dev/null 2>&1; then
  useradd --system \
    --gid "$GROUP_NAME" \
    --home-dir "$DATA_DIR" \
    --shell /usr/sbin/nologin \
    "$USER_NAME"
fi

install -d -o root -g root -m 0755 "$INSTALL_DIR"
install -d -o root -g root -m 0755 "$CONFIG_DIR"
install -d -o "$USER_NAME" -g "$GROUP_NAME" -m 0750 "$DATA_DIR"
install -d -o "$USER_NAME" -g "$GROUP_NAME" -m 0750 "$DATA_DIR/tiles"

install -o root -g root -m 0755 "$BINARY_SOURCE" "$INSTALL_DIR/$APP_NAME"
install -o root -g root -m 0644 "$SCRIPT_DIR/$SERVICE_NAME" "$SYSTEMD_DIR/$SERVICE_NAME"

if [ "$OVERWRITE_CONFIG" -eq 1 ] || [ ! -f "$CONFIG_DIR/config.yaml" ]; then
  install -o root -g "$GROUP_NAME" -m 0640 "$SCRIPT_DIR/config.yaml" "$CONFIG_DIR/config.yaml"
else
  echo "preserving existing $CONFIG_DIR/config.yaml"
fi

if [ ! -f "$CONFIG_DIR/cupolad.env" ]; then
  install -o root -g "$GROUP_NAME" -m 0640 "$SCRIPT_DIR/cupolad.env.example" "$CONFIG_DIR/cupolad.env"
fi

systemctl daemon-reload

if [ "$START_SERVICE" -eq 1 ]; then
  systemctl enable "$SERVICE_NAME"
  systemctl restart "$SERVICE_NAME"
  systemctl --no-pager --full status "$SERVICE_NAME"
else
  echo "installed $SERVICE_NAME; start with: systemctl enable --now $SERVICE_NAME"
fi
