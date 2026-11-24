const { chromium } = require('playwright');
const fs = require('fs');
const OUT = process.env.OUT_DIR || 'out';
fs.mkdirSync(OUT, { recursive: true });
const [LISBON, DINNER] = [process.env.LISBON_ID, process.env.DINNER_ID];
const BASE = process.env.BASE_URL || 'http://localhost:8080';
const LOG = process.env.SERVER_LOG;
const sizes = { phone: { width: 390, height: 844 }, desktop: { width: 1440, height: 900 } };
(async () => {
  const browser = await chromium.launch();
  for (const [tag, viewport] of Object.entries(sizes)) {
    const ctx = await browser.newContext({ viewport, deviceScaleFactor: tag === 'phone' ? 2 : 1, colorScheme: 'light' });
    const page = await ctx.newPage();
    const external = new Set();
    page.on('request', r => { const u = new URL(r.url()); if (u.host !== 'localhost:8080') external.add(u.host); });
    const semantics = async () => { const ph = await page.$('flt-semantics-placeholder'); if (ph) { await ph.evaluate(e => e.click()); await page.waitForTimeout(400); } };
    const settle = async (ms = 1500) => { await page.waitForTimeout(ms); };
    const shot = async (n) => { await settle(); await page.screenshot({ path: `${OUT}/${tag}-${n}.png` }); };
    const click = async (name, role = 'button') => { await semantics(); await page.getByRole(role, { name, exact: false }).first().click(); await settle(800); };
    const type = async (label, text) => { await semantics(); await page.getByRole('textbox', { name: label }).first().click(); await page.waitForTimeout(400); await page.keyboard.type(text, { delay: 25 }); };
    const go = async (path) => { await page.goto(BASE + path, { waitUntil: 'networkidle' }); await settle(2200); };

    await go('/');
    await shot('welcome');
    await click('Connect');
    await type('Email', 'ada@example.com'); await click('Continue'); await settle(1200);
    const log = fs.readFileSync(LOG, 'utf8');
    const code = [...log.matchAll(/to=ada@example\.com subject="(\d{6})/g)].pop()[1];
    await type('Verification code', code); await settle(3000);
    await go('/'); await shot('home');
    await go('/groups/' + LISBON); await shot('group-expenses');
    await click('Balances', 'tab'); await shot('group-balances');
    await click('Stats', 'tab'); await shot('group-stats');
    await go('/expenses/' + DINNER); await shot('expense-detail');
    await go('/friends'); await shot('friends');
    if (tag === 'phone') {
      await go('/groups/' + LISBON); await click('Add expense');
      await page.keyboard.type('Sunset boat tour', { delay: 25 });
      await page.mouse.click(240, 217); await page.waitForTimeout(400); await page.keyboard.type('180', { delay: 40 });
      await click('Shares'); await shot('expense-editor');
    }
    console.log(tag, 'external:', [...external].join(',') || 'none');
    await ctx.close();
  }
  await browser.close();
})();
