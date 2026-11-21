#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
site="$(mktemp -d "${TMPDIR:-/tmp}/mio-ui-preview.XXXXXX")"

cp -R "$root/cmd/mio-desktop/assets/." "$site/"
cp "$root/scripts/ui-preview/mock.js" "$site/mock.js"

python3 - "$site/index.html" <<'PY'
from pathlib import Path
import sys

path = Path(sys.argv[1])
html = path.read_text()
needle = '  <script type="module" src="/app.js"></script>'
if needle not in html:
    raise SystemExit("index.html 中找不到 app.js 入口")
html = html.replace(needle, '  <script src="/mock.js"></script>\n' + needle, 1)
path.write_text(html)
PY

printf '%s\n' "$site"
