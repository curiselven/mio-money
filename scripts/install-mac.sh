#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
APP_NAME="Mio.app"
EXECUTABLE_NAME="Mio"
DEFAULT_PREFIX="/Applications"
DEFAULT_VERSION="0.1.0"

PREFIX="$DEFAULT_PREFIX"
ARCHIVE_ROOT="$HOME/.mio-old-apps"
VERSION="$DEFAULT_VERSION"
ICON=""
DRY_RUN=0

usage() {
  cat <<'EOF'
用法: scripts/install-mac.sh [选项]

构建并安装 Mio.app 到 /Applications。不会结束正在运行的 Mio。

选项:
  --prefix <dir>       安装目录，默认 /Applications
  --archive-dir <dir>  旧 Mio.app 的归档根目录，默认 $HOME/.mio-old-apps
  --version <ver>      写入 Info.plist 的版本号，默认 0.1.0
  --icon <path>        图标源文件（.png/.icns/.svg）；默认 packaging/macos/Mio.icns，缺失时回退 docs/images/logo.svg
  --dry-run            只打印将执行的步骤，不构建、不改 /Applications 或数据库
  --help               显示帮助
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --prefix) PREFIX="${2:?}"; shift 2 ;;
    --archive-dir) ARCHIVE_ROOT="${2:?}"; shift 2 ;;
    --version) VERSION="${2:?}"; shift 2 ;;
    --icon) ICON="${2:?}"; shift 2 ;;
    --dry-run) DRY_RUN=1; shift ;;
    --help|-h) usage; exit 0 ;;
    *) echo "未知参数: $1" >&2; usage >&2; exit 2 ;;
  esac
done

STAMP="$(date +%Y%m%d-%H%M%S)"
BUILD_DIR="$ROOT/build"
APP_PATH="$BUILD_DIR/$APP_NAME"
DEST_APP="$PREFIX/$APP_NAME"
DB_DIR="$HOME/Library/Application Support/MioMoney"
DB_PATH="$DB_DIR/mio.db"
BACKUP_DIR="$DB_DIR/backups"
BACKUP_PATH="$BACKUP_DIR/mio-$STAMP.db"
ARCHIVE_DIR="$ARCHIVE_ROOT/$STAMP"

run() {
  printf '  $'; printf ' %q' "$@"; printf '\n'
  if [[ "$DRY_RUN" -eq 0 ]]; then
    "$@"
  fi
}

step() {
  printf '\n==> %s\n' "$1"
}

if [[ -z "$ICON" ]]; then
  if [[ -f "$ROOT/packaging/macos/Mio.icns" ]]; then
    ICON="$ROOT/packaging/macos/Mio.icns"
  else
    ICON="$ROOT/docs/images/logo.svg"
  fi
fi

step "检查运行中的 Mio"
if [[ "$DRY_RUN" -eq 0 ]] && pgrep -x "$EXECUTABLE_NAME" >/dev/null 2>&1; then
  echo "Mio 正在运行，请先退出后再安装（不会自动结束进程）。" >&2
  exit 1
fi
echo "  pgrep -x ${EXECUTABLE_NAME}（运行中会退出）"

step "备份数据库"
if [[ -f "$DB_PATH" ]]; then
  run mkdir -p -m 0700 "$BACKUP_DIR"
  run cp -p "$DB_PATH" "$BACKUP_PATH"
  run chmod 0600 "$BACKUP_PATH"
  for suffix in -wal -shm; do
    if [[ -f "$DB_PATH$suffix" ]]; then
      run cp -p "$DB_PATH$suffix" "$BACKUP_PATH$suffix"
      run chmod 0600 "$BACKUP_PATH$suffix"
    fi
  done
  echo "  数据库备份目标: $BACKUP_PATH"
else
  echo "  未找到 ${DB_PATH}，跳过数据库备份"
fi

step "构建 CLI 和桌面应用"
run mkdir -p "$BUILD_DIR" "$ROOT/bin" "$APP_PATH/Contents/MacOS" "$APP_PATH/Contents/Resources"
run go build -o "$ROOT/bin/mio" ./cmd/mio
run go build -o "$APP_PATH/Contents/MacOS/$EXECUTABLE_NAME" ./cmd/mio-desktop

step "写入 Info.plist"
run cp "$ROOT/packaging/macos/Info.plist" "$APP_PATH/Contents/Info.plist"
if [[ "$DRY_RUN" -eq 0 ]]; then
  /usr/libexec/PlistBuddy -c "Set :CFBundleShortVersionString $VERSION" "$APP_PATH/Contents/Info.plist"
else
  echo "  \$ /usr/libexec/PlistBuddy -c \"Set :CFBundleShortVersionString $VERSION\" $APP_PATH/Contents/Info.plist"
fi

step "生成图标"
if [[ ! -f "$ICON" ]]; then
  echo "图标不存在: ${ICON}；可用 --icon <path> 指定。" >&2
  exit 1
fi
ICON_TMP="$BUILD_DIR/icon-work"
ICONSET="$ICON_TMP/icon.iconset"
run mkdir -p "$ICONSET"
if [[ "$ICON" == *.icns ]]; then
  run cp "$ICON" "$APP_PATH/Contents/Resources/Mio.icns"
else
  PNG="$ICON_TMP/source.png"
  if [[ "$ICON" == *.svg ]]; then
    if [[ "$DRY_RUN" -eq 0 ]]; then
      qlmanage -t -s 1024 -o "$ICON_TMP" "$ICON" >/dev/null 2>&1 || true
      PNG="$ICON_TMP/$(basename "$ICON").png"
    else
      echo "  \$ qlmanage -t -s 1024 -o $ICON_TMP $ICON"
    fi
  else
    run sips -s format png "$ICON" --out "$PNG"
  fi
  if [[ "$DRY_RUN" -eq 0 && ! -f "$PNG" ]]; then
    echo "无法从 $ICON 生成 PNG；可先用 --icon 提供 PNG/ICNS。" >&2
    exit 1
  fi
  run sips -z 16 16 "$PNG" --out "$ICONSET/icon_16x16.png"
  run sips -z 32 32 "$PNG" --out "$ICONSET/icon_16x16@2x.png"
  run sips -z 32 32 "$PNG" --out "$ICONSET/icon_32x32.png"
  run sips -z 64 64 "$PNG" --out "$ICONSET/icon_32x32@2x.png"
  run sips -z 128 128 "$PNG" --out "$ICONSET/icon_128x128.png"
  run sips -z 256 256 "$PNG" --out "$ICONSET/icon_128x128@2x.png"
  run sips -z 256 256 "$PNG" --out "$ICONSET/icon_256x256.png"
  run sips -z 512 512 "$PNG" --out "$ICONSET/icon_256x256@2x.png"
  run sips -z 512 512 "$PNG" --out "$ICONSET/icon_512x512.png"
  run sips -z 1024 1024 "$PNG" --out "$ICONSET/icon_512x512@2x.png"
  run iconutil -c icns "$ICONSET" -o "$APP_PATH/Contents/Resources/Mio.icns"
fi

step "Ad-hoc 签名"
run codesign --force --deep -s - "$APP_PATH"

step "归档旧应用"
if [[ -e "$DEST_APP" ]]; then
  run mkdir -p "$ARCHIVE_DIR"
  run mv "$DEST_APP" "$ARCHIVE_DIR/$APP_NAME"
  echo "  旧应用归档到: $ARCHIVE_DIR/$APP_NAME"
else
  echo "  $DEST_APP 不存在，跳过归档"
fi

step "安装新应用"
run mkdir -p "$PREFIX"
run ditto "$APP_PATH" "$DEST_APP"
run xattr -dr com.apple.quarantine "$DEST_APP"

step "完成"
echo "  Mio.app: $DEST_APP"
echo "  CLI: $ROOT/bin/mio"
if [[ "$DRY_RUN" -eq 1 ]]; then
  echo "  dry-run：以上命令均未执行。"
fi
