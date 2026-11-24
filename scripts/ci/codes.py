#!/usr/bin/env python3
"""Test helper for device tests: serves the latest sign-in code a development
server wrote to its log (SMTP_HOST unset), so an emulator can sign in like a
person reading their email. Never run this next to a real server.

    python3 scripts/ci/codes.py server.log 8099
    curl 'http://localhost:8099/code?email=ada@example.com'  ->  {"code": "123456"}

    python3 scripts/ci/codes.py server.log --check ada@example.com Cabin
"""
import json
import os
import re
import sys
import time
import urllib.parse
import urllib.request
from http.server import BaseHTTPRequestHandler, HTTPServer


def latest(log, email):
    pattern = re.compile(r'to=%s subject="(\d{6}) is' % re.escape(email))
    with open(log, encoding="utf-8") as f:
        found = pattern.findall(f.read())
    return found[-1] if found else None


def serve(log, port):
    class Handler(BaseHTTPRequestHandler):
        def do_GET(self):
            url = urllib.parse.urlparse(self.path)
            email = urllib.parse.parse_qs(url.query).get("email", [""])[0]
            code = latest(log, email) if url.path == "/code" else None
            body = json.dumps({"code": code} if code else {"error": "no code"}).encode()
            self.send_response(200 if code else 404)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(body)

        def log_message(self, *args):
            pass

    HTTPServer(("0.0.0.0", port), Handler).serve_forever()


def call(method, path, body=None, token=None):
    api = os.environ.get("SPLITSPENDS_API", "http://localhost:8080/api/v1")
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Authorization"] = "Bearer " + token
    data = None if body is None else json.dumps(body).encode()
    req = urllib.request.Request(api + path, data=data, method=method, headers=headers)
    with urllib.request.urlopen(req) as res:
        return json.loads(res.read() or b"null")


def check(log, email, group):
    """Signs in as email through the API and asserts the uploaded group and
    its expense reached the server."""
    call("POST", "/auth/otp/request", {"email": email})
    time.sleep(1)
    token = call("POST", "/auth/otp/verify", {"email": email, "code": latest(log, email)})["data"]["token"]
    groups = call("GET", "/groups", token=token)["items"]
    match = [g for g in groups if g["name"] == group]
    assert match, f"group {group!r} not on the server: {[g['name'] for g in groups]}"
    expenses = call("GET", "/expenses?groupId=" + match[0]["id"], token=token)["items"]
    assert any(e["description"] == "Groceries" and e["total"] == 9000 for e in expenses), [e["description"] for e in expenses]
    print(f"ok: {group} with {len(expenses)} expense(s) uploaded for {email}")


if __name__ == "__main__":
    if len(sys.argv) >= 5 and sys.argv[2] == "--check":
        check(sys.argv[1], sys.argv[3], sys.argv[4])
    else:
        serve(sys.argv[1], int(sys.argv[2]))
