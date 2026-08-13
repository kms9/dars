import { chromium } from '@playwright/test';
const browser = await chromium.launch();
const page = await browser.newPage();
const logs = [];
page.on('console', m => logs.push(`[console.${m.type()}] ${m.text()}`));
page.on('pageerror', e => logs.push(`[pageerror] ${e.message}`));
page.on('requestfailed', r => logs.push(`[requestfailed] ${r.url()} ${r.failure()?.errorText}`));
const api = [];
page.on('response', r => {
  const u = new URL(r.url());
  if (u.pathname.startsWith('/api') || u.pathname.startsWith('/auth')) api.push(`${r.status()} ${r.request().method()} ${u.pathname}`);
});
await page.goto('http://localhost:23000/login', { waitUntil: 'domcontentloaded', timeout: 60000 });
await page.waitForTimeout(25000);
console.log('URL:', page.url());
console.log('TEXT:', (await page.locator('body').innerText()).slice(0, 600));
console.log('API CALLS:', JSON.stringify(api));
console.log('LOGS:\n' + logs.slice(0, 40).join('\n'));
await browser.close();
