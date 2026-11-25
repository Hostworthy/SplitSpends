## Downloads

| File | What it is |
| --- | --- |
| `splitspends-server-{{TAG}}-linux-amd64.tar.gz` (and `linux-arm64`, `darwin-amd64`, `darwin-arm64`, `windows-amd64.zip`) | The server (`splitspends`) and the stdio MCP host (`splitspends-mcp`), with `env.example`, the license, the third-party notices and the README |
| `splitspends-web-{{TAG}}.tar.gz` | The web app, ready to serve. Point the server at it with `WEB_DIR=/path/to/web` |
| `{{APK}}` | Android app. Open it on the phone to install (allow installs from your browser or files app).{{APK_NOTE}} |
| `splitspends-ios-{{TAG}}-unsigned.ipa` | iPhone app, **unsigned**: install it with your own Apple ID through Xcode, AltStore or Sideloadly |
| `SHA256SUMS` | Checksums for every file above |

Container image with the server and the web app included: `{{IMAGE}}`. The command below pulls this release's exact build by its digest:

```bash
docker run -p 8080:8080 \
  -e DATABASE_URL=postgres://user:pass@db:5432/splitspends \
  -e SECRET_KEY="$(openssl rand -hex 32)" \
  {{IMAGE_DIGEST}}
```

Run the server from a download:

```bash
tar -xzf splitspends-server-{{TAG}}-linux-amd64.tar.gz
tar -xzf splitspends-web-{{TAG}}.tar.gz
DATABASE_URL=postgres://user:pass@localhost:5432/splitspends \
SECRET_KEY="$(openssl rand -hex 32)" WEB_DIR=./web \
  ./splitspends-server-{{TAG}}-linux-amd64/splitspends
```

Check that a file or the image was built by this repository's release workflow, with the [GitHub CLI](https://cli.github.com/):

```bash
gh attestation verify splitspends-server-{{TAG}}-linux-amd64.tar.gz -R {{REPO}}
gh attestation verify oci://{{IMAGE_DIGEST}} -R {{REPO}}
```

Hosting options, HTTPS and every setting: [docs/HOSTING.md](https://github.com/Hostworthy/SplitSpends/blob/{{TAG}}/docs/HOSTING.md) and [docs/ENVIRONMENT.md](https://github.com/Hostworthy/SplitSpends/blob/{{TAG}}/docs/ENVIRONMENT.md).

Free for personal and other noncommercial use under the PolyForm Noncommercial License 1.0.0. Commercial use needs permission.
