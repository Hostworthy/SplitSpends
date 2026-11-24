// Captures the README screenshots from a release web build in on-device
// mode, with demo data entered through the UI.
//
//   flutter build web --release --no-web-resources-cdn
//   # serve build/web with SPA fallback, for example the backend with WEB_DIR
//   node tool/screenshots.js http://localhost:8080/ ../docs/screenshots
//
// Needs Playwright (npm i -g playwright, then NODE_PATH=$(npm root -g)).
const { chromium } = require('playwright');
const path = require('path');

const base = process.argv[2] || 'http://localhost:8080/';
const out = process.argv[3] || path.join(__dirname, '..', '..', 'docs', 'screenshots');

(async () => {
  const browser = await chromium.launch();
  const context = await browser.newContext({ viewport: { width: 1280, height: 800 }, deviceScaleFactor: 2 });
  const page = await context.newPage();
  page.on('pageerror', e => console.error('pageerror', e.message));

  const wait = ms => page.waitForTimeout(ms);
  // Flutter draws to a canvas; the semantics tree gives Playwright roles.
  async function open(route) {
    await page.goto(base + route, { waitUntil: 'networkidle' });
    await page.waitForSelector('flt-semantics-placeholder', { state: 'attached', timeout: 30000 });
    await page.locator('flt-semantics-placeholder').dispatchEvent('click');
  }
  // Flutter keeps focus in the last field, so click before typing.
  async function type(name, text) {
    await page.getByRole('textbox', { name }).click();
    await wait(300);
    await page.keyboard.type(text);
    await wait(300);
  }
  async function shot(name) {
    await page.waitForLoadState('networkidle');
    await wait(2500);
    await page.screenshot({ path: path.join(out, `${name}.png`) });
    console.log('screenshots:', name);
  }
  async function group(name, kind, people) {
    await page.getByRole('button', { name: 'New group' }).first().click();
    await type('Group name', name);
    await page.getByRole('checkbox', { name: new RegExp(kind) }).click();
    for (const person of people) {
      await type('Add people by name or email', person);
      await page.keyboard.press('Enter');
      await wait(500);
    }
    await page.getByRole('button', { name: 'Create group' }).click();
    await page.waitForURL(/\/groups\//);
    return page.url().split('/groups/')[1];
  }
  async function expense(groupId, description, amount, payer) {
    await open(`expenses/new?groupId=${groupId}`);
    await type('Description', description);
    await type('0.00', amount);
    if (payer) {
      await page.getByRole('checkbox', { name: /You$/ }).first().click();
      await page.getByRole('region', { name: 'Dialog' }).getByRole('button', { name: new RegExp(` ${payer}$`) }).click();
      await wait(800);
      if (await page.getByRole('button', { name: 'Dismiss' }).count()) await page.keyboard.press('Escape');
      await wait(500);
    }
    await page.getByRole('button', { name: 'Save' }).click();
    await wait(1500);
  }

  await open('');
  await page.getByRole('button', { name: 'Start' }).click();
  await type('Your name', 'Alex');
  await page.getByRole('button', { name: 'Get started' }).click();

  const flat = await group('Flat 4B', 'Home', ['Riley', 'Casey']);
  await expense(flat, 'Rent', '2400');
  await expense(flat, 'Electricity', '120', 'Riley');
  await expense(flat, 'Groceries', '86.75', 'Casey');
  await expense(flat, 'Internet', '60');
  await open('');
  const trip = await group('Lisbon trip', 'Trip', ['Sam', 'Jordan', 'Taylor']);
  await expense(trip, 'Flights', '840');
  await expense(trip, 'Hotel in Alfama', '620', 'Sam');
  await expense(trip, 'Dinner at the market', '96.40', 'Jordan');
  await expense(trip, 'Coffee and pastries', '18.50', 'Taylor');
  await expense(trip, 'Tram tickets', '24');
  await expense(trip, 'Sintra day trip', '135', 'Sam');

  await open('');
  await shot('dashboard');
  await open(`groups/${trip}`);
  await shot('group');
  await page.getByRole('tab', { name: /Balances/ }).click();
  await shot('balances');
  await open('');
  await page.getByRole('button', { name: 'New group' }).first().click();
  await type('Group name', 'Ski weekend');
  await shot('new-group');
  await open(`expenses/new?groupId=${trip}`);
  await type('Description', 'Museum tickets');
  await type('0.00', '48');
  await page.getByRole('button', { name: 'Shares' }).click();
  await shot('editor');

  await page.setViewportSize({ width: 390, height: 844 });
  await open('');
  await shot('mobile-dashboard');
  await open(`groups/${trip}`);
  await shot('mobile-group');
  await page.emulateMedia({ colorScheme: 'dark' });
  await open('');
  await shot('mobile-dashboard-dark');
  await open(`groups/${trip}`);
  await shot('mobile-group-dark');

  await browser.close();
})().catch(e => {
  console.error(e);
  process.exit(1);
});
