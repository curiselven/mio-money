# 静态 UI 预览

用虚构数据在浏览器里直接打开桌面端页面，不启动 Mio，也不读取数据库。

## 截图

```sh
./scripts/ui-preview/shoot.sh
./scripts/ui-preview/shoot.sh /tmp/mio-ui-shots
CHROME="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" ./scripts/ui-preview/shoot.sh
```

默认输出到当前目录的 `./shots`。会截 `home`、`import`、`import-done`、`accounts`、`pending` 五页，尺寸 1280×900。Chrome 按 `CHROME`、`google-chrome`、`chromium`、macOS Chrome 路径依次查找。

每次调用 Chrome 都由 `timeout`/`gtimeout` 限制在 30 秒内；没有这两个命令时脚本会自己计时并在超时后结束该次调用。超时或失败只记为当前页失败，其余页继续截，最后列出失败页并以非零退出。可用 `SHOT_TIMEOUT` 改超时秒数，用 `EXTRA_CHROME_FLAGS` 追加参数（以空格分隔，如 `--no-sandbox`）：

```sh
SHOT_TIMEOUT=60 EXTRA_CHROME_FLAGS="--no-sandbox" ./scripts/ui-preview/shoot.sh
```

## 手动预览

```sh
site="$(./scripts/ui-preview/build.sh)"
python3 -m http.server 8765 --directory "$site"
```

然后访问 `http://127.0.0.1:8765/?page=import`。可用页面：

- `?page=home`
- `?page=import`
- `?page=import-done`
- `?page=accounts`
- `?page=pending`

`build.sh` 把 `cmd/mio-desktop/assets` 复制到临时目录，并在 `app.js` 前注入 `mock.js`。`mock.js` 覆盖 `main.*Service` 的返回值；`import` 页面会模拟选择文件与解析，`import-done` 还会模拟一次全部导入。生产代码不需要预览钩子。
