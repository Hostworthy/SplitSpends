# README Screenshots

Regenerates the README hero image (`docs/images/hero.webp`) with demo data.

1. Build the web app (`make web-build`) and start the server with an empty database, `SIGNUP_MODE=open`, `WEB_DIR=app/build/web` and no `SMTP_HOST`, writing its log to a file (login codes are read from it).
2. Seed demo data through the API:

   ```bash
   SERVER_LOG=/path/to/server.log python3 seed.py
   ```

   It prints the group ids. Find the dinner expense id with `GET /api/v1/expenses?q=Dinner`.
3. Capture phone (390x844 at 2x) and desktop (1440x900) screenshots with the preinstalled Playwright:

   ```bash
   SERVER_LOG=/path/to/server.log LISBON_ID=<id> DINNER_ID=<id> OUT_DIR=out node capture.js
   ```

4. Compose the README hero image from a desktop and a phone shot:

   ```bash
   python3 hero.py out/desktop-group-expenses.png out/phone-group-balances.png ../../docs/images/hero.webp
   ```

Keep demo data in plain English.
