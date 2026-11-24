## Downloads

| File | What it is |
| --- | --- |
| `splitspends-server-{{TAG}}-linux-amd64.tar.gz` (and `linux-arm64`, `darwin-amd64`, `darwin-arm64`, `windows-amd64.zip`) | The server (`splitspends`) and the stdio MCP host (`splitspends-mcp`), with `env.example`, the license and the README |
| `splitspends-web-{{TAG}}.tar.gz` | The web app, ready to serve. Point the server at it with `WEB_DIR=/path/to/web` |
| `splitspends-android-{{TAG}}.apk` | Android app. Open it on the phone to install (allow installs from your browser or files app) |
| `splitspends-ios-{{TAG}}-unsigned.ipa` | iPhone app, **unsigned**: install it with your own Apple ID through Xcode, AltStore or Sideloadly |
| `SHA256SUMS` | Checksums for every file above |

Container image with the server and the web app included: `{{IMAGE}}`

```bash
docker run -p 8080:8080 \
  -e DATABASE_URL=postgres://user:pass@db:5432/splitspends \
  -e SECRET_KEY="$(openssl rand -hex 32)" \
  {{IMAGE}}
```

Run the server from a download:

```bash
tar -xzf splitspends-server-{{TAG}}-linux-amd64.tar.gz
tar -xzf splitspends-web-{{TAG}}.tar.gz
DATABASE_URL=postgres://user:pass@localhost:5432/splitspends \
SECRET_KEY="$(openssl rand -hex 32)" WEB_DIR=./web \
  ./splitspends-server-{{TAG}}-linux-amd64/splitspends
```

Hosting options, HTTPS and every setting: [docs/HOSTING.md](https://github.com/Hostworthy/SplitSpends/blob/{{TAG}}/docs/HOSTING.md) and [docs/ENVIRONMENT.md](https://github.com/Hostworthy/SplitSpends/blob/{{TAG}}/docs/ENVIRONMENT.md).

Free for personal and other noncommercial use under the PolyForm Noncommercial License 1.0.0. Commercial use needs permission.
