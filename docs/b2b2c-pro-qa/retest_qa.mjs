import { chromium } from '/workspace/pw/node_modules/playwright/index.mjs';
import fs from 'fs';

const B = process.env.B2B_BASE || 'http://127.0.0.1:3310';
const OUT = '/workspace/b2b2c-pro-qa/retest';
fs.mkdirSync(OUT, { recursive: true });
const results = [];
const browser = await chromium.launch({ headless: true });

function record(id, title, expected, actual, result, shot = null, notes = '') {
  results.push({ id, title, expected, actual, result, shot, notes });
  console.log(`[${result}] ${id} ${title}: ${actual.slice(0, 200)}`);
}

async function shot(page, name) {
  const f = `${OUT}/${name}.png`;
  await page.screenshot({ path: f, fullPage: true });
  return f;
}

async function apiLogin(username, password, ip = '10.20.30.99') {
  const res = await fetch(B + '/api/user/login', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'X-Forwarded-For': ip },
    body: JSON.stringify({ username, password }),
  });
  const setCookie = typeof res.headers.getSetCookie === 'function' ? res.headers.getSetCookie() : [];
  const body = await res.json();
  if (!body.success) throw new Error(`login ${username}: ${JSON.stringify(body)}`);
  return { data: body.data, setCookie };
}

function cookiesFrom(setCookie) {
  return setCookie.map(raw => {
    const parts = raw.split(';').map(s => s.trim());
    const eq = parts[0].indexOf('=');
    const cookie = { name: parts[0].slice(0, eq), value: parts[0].slice(eq + 1), domain: '127.0.0.1', path: '/' };
    for (const a of parts.slice(1)) {
      const al = a.toLowerCase();
      if (al.startsWith('path=')) cookie.path = a.slice(5);
      if (al === 'httponly') cookie.httpOnly = true;
      if (al.startsWith('samesite=')) cookie.sameSite = /** @type {'Strict'} */ (a.split('=')[1]);
    }
    return cookie;
  });
}

async function authedContext(user, pass, ip) {
  const { data, setCookie } = await apiLogin(user, pass, ip);
  const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 }, locale: 'zh-HK' });
  await ctx.addCookies(cookiesFrom(setCookie));
  const page = await ctx.newPage();
  await page.route('**/api/user/auth/refresh', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        success: true,
        message: '',
        data: {
          access_token: data.access_token,
          token_type: data.token_type,
          access_expires_at: data.access_expires_at,
          session: data.session,
          user: data.user,
        },
      }),
    });
  });
  await page.route('**/api/**', async (route) => {
    if (route.request().url().includes('/auth/refresh')) return route.fallback();
    const headers = {
      ...route.request().headers(),
      authorization: `Bearer ${data.access_token}`,
      'new-api-user': String(data.user.id),
    };
    await route.continue({ headers });
  });
  await page.goto(B + '/', { waitUntil: 'domcontentloaded' });
  await page.waitForTimeout(1500);
  return { ctx, page, data };
}

// ---- Status flags ----
{
  const st = await (await fetch(B + '/api/status')).json();
  const d = st.data || st;
  const flags = {
    register_enabled: d.register_enabled,
    password_register_enabled: d.password_register_enabled,
    oauth_register_enabled: d.oauth_register_enabled,
  };
  const ok = flags.register_enabled === false && flags.password_register_enabled === false && flags.oauth_register_enabled === false;
  record('TC03r', 'Status 註冊旗標預設關閉', '三旗標均 false', JSON.stringify(flags), ok ? 'Pass' : 'Fail');
}

// ---- TC17: sign-in has no sign-up link ----
{
  const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 }, locale: 'zh-HK' });
  const page = await ctx.newPage();
  await page.goto(B + '/sign-in', { waitUntil: 'networkidle' });
  await page.waitForTimeout(1500);
  const links = await page.locator('a[href*="sign-up"]').count();
  const f = await shot(page, 'public-login');
  const body = (await page.innerText('body')).replace(/\s+/g, ' ').slice(0, 300);
  record('TC17', '登入頁無註冊入口', 'a[href*="sign-up"]=0', `count=${links}; url=${page.url()}; body=${body}`, links === 0 ? 'Pass' : 'Fail', f);
  await ctx.close();
}

// ---- TC18: /sign-up redirects to /sign-in ----
{
  const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 }, locale: 'zh-HK' });
  const page = await ctx.newPage();
  await page.goto(B + '/sign-up', { waitUntil: 'networkidle' });
  await page.waitForTimeout(2000);
  const url = page.url();
  const redirected = /sign-in/i.test(url) && !/sign-up/i.test(url.replace(/.*\/sign-in.*/, '/sign-in'));
  const hasPasswordForm =
    (await page.locator('input[name="username"], input[name="password"], input[autocomplete="new-password"]').count()) > 0 ||
    /建立一個用戶|Create an account|Confirm password|確認密碼/i.test(await page.innerText('body').catch(() => ''));
  // More precise: password confirm field unique to signup
  const confirmCount = await page.getByLabel(/confirm password|確認密碼/i).count().catch(() => 0);
  const createBtn = await page.getByRole('button', { name: /create account|建立/i }).count().catch(() => 0);
  const f = await shot(page, 'public-signup-redirect');
  const body = (await page.innerText('body')).replace(/\s+/g, ' ').slice(0, 300);
  const ok = /\/sign-in/i.test(url) && confirmCount === 0 && createBtn === 0;
  record(
    'TC18',
    '/sign-up 深鏈在註冊關閉時導向登入',
    'redirect → /sign-in；不顯示密碼註冊表單',
    `url=${url}; confirmFields=${confirmCount}; createBtn=${createBtn}; body=${body}`,
    ok ? 'Pass' : 'Fail',
    f,
    redirected ? '' : 'url check soft'
  );
  await ctx.close();
}

// ---- Smoke: Admin ----
{
  const { ctx, page } = await authedContext('admin', 'Admin12345!', '10.20.30.41');
  await page.goto(B + '/resellers', { waitUntil: 'domcontentloaded' });
  await page.waitForTimeout(2500);
  const f1 = await shot(page, 'admin-resellers');
  const body1 = (await page.innerText('body')).replace(/\s+/g, ' ').slice(0, 400);
  const ok1 = !/sign-in/i.test(page.url()) && !/403|禁止/i.test(body1);
  record('SM-ADMIN-1', 'Admin 代理商列表', '可開啟 /resellers', `url=${page.url()}; body=${body1}`, ok1 ? 'Pass' : 'Fail', f1);

  await page.goto(B + '/users', { waitUntil: 'domcontentloaded' });
  await page.waitForTimeout(2000);
  const f2 = await shot(page, 'admin-users');
  const body2 = (await page.innerText('body')).replace(/\s+/g, ' ').slice(0, 400);
  const ok2 = !/sign-in/i.test(page.url());
  record('SM-ADMIN-2', 'Admin 用戶管理', '可開啟 /users', `url=${page.url()}; body=${body2}`, ok2 ? 'Pass' : 'Fail', f2);
  await ctx.close();
}

// ---- Smoke: Reseller ----
{
  const { ctx, page } = await authedContext('reseller1', 'Reseller123!', '10.20.30.42');
  for (const [path, name, title] of [
    ['/reseller', 'reseller-console', 'Reseller 總覽'],
    ['/reseller/customers', 'reseller-customers', 'Reseller 客戶列表'],
    ['/wallet', 'reseller-wallet', 'Reseller 錢包儲值頁'],
  ]) {
    await page.goto(B + path, { waitUntil: 'domcontentloaded' });
    await page.waitForTimeout(2200);
    const f = await shot(page, name);
    const body = (await page.innerText('body')).replace(/\s+/g, ' ').slice(0, 400);
    const ok = !/sign-in/i.test(page.url()) && !/\/403/i.test(page.url());
    record(`SM-RSL-${name}`, title, `可開啟 ${path}`, `url=${page.url()}; body=${body}`, ok ? 'Pass' : 'Fail', f);
  }
  await ctx.close();
}

// ---- Smoke: Customer ----
{
  const { ctx, page } = await authedContext('cust1', 'Customer123!', '10.20.30.43');
  for (const [path, name, title, expectForbidden] of [
    ['/profile', 'cust-profile', 'Customer 個人資料／代理商卡片', false],
    ['/wallet', 'cust-wallet', 'Customer 錢包儲值頁', false],
    ['/dashboard', 'cust-dashboard', 'Customer 概覽', false],
    ['/reseller', 'cust-forbidden-reseller', 'Customer 進 Reseller 區應 403', true],
  ]) {
    await page.goto(B + path, { waitUntil: 'domcontentloaded' });
    await page.waitForTimeout(2200);
    const f = await shot(page, name);
    const body = (await page.innerText('body')).replace(/\s+/g, ' ').slice(0, 400);
    const url = page.url();
    let ok;
    if (expectForbidden) {
      ok = /\/403/i.test(url) || /禁止|Forbidden|403/i.test(body);
    } else {
      ok = !/sign-in/i.test(url) && !/\/403/i.test(url);
    }
    record(`SM-CUST-${name}`, title, expectForbidden ? '403' : `可開啟 ${path}`, `url=${url}; body=${body}`, ok ? 'Pass' : 'Fail', f);
  }
  await ctx.close();
}

// ---- API smoke dual-leg / topup / isolation ----
async function api(method, path, body, token, userId, ip = '10.20.30.50') {
  const headers = { Accept: 'application/json', 'X-Forwarded-For': ip };
  if (body) headers['Content-Type'] = 'application/json';
  if (token) headers.Authorization = 'Bearer ' + token;
  if (userId) headers['New-API-User'] = String(userId);
  const res = await fetch(B + path, { method, headers, body: body ? JSON.stringify(body) : undefined });
  const text = await res.text();
  let j;
  try { j = JSON.parse(text); } catch { j = { _text: text.slice(0, 200) }; }
  j._status = res.status;
  return j;
}

{
  const admin = await apiLogin('admin', 'Admin12345!', '10.20.30.51');
  const r1 = await apiLogin('reseller1', 'Reseller123!', '10.20.30.52');
  const r2 = await apiLogin('reseller2', 'Reseller123!', '10.20.30.53');
  const c1 = await apiLogin('cust1', 'Customer123!', '10.20.30.54');

  const self = await api('GET', '/api/user/self', null, c1.data.access_token, c1.data.user.id);
  const resellerCard = self?.data?.reseller;
  const hasCard = resellerCard && (resellerCard.display_name || resellerCard.name);
  const leaks = JSON.stringify(resellerCard || {}).match(/markup|cost_multiplier|cost_factor/i);
  record(
    'SM-API-CARD',
    'Customer self 含代理商卡片且不洩漏成本',
    '有 display_name；無 markup/cost',
    `reseller=${JSON.stringify(resellerCard)}; online_topup=${self?.data?.online_topup_enabled}`,
    hasCard && !leaks ? 'Pass' : 'Fail'
  );

  const topup = await api('GET', '/api/user/topup/info', null, c1.data.access_token, c1.data.user.id);
  record(
    'SM-API-TOPUP-C',
    'Customer topup info 未被封鎖',
    'HTTP 200',
    `HTTP${topup._status} success=${topup.success} msg=${topup.message}`,
    topup._status === 200 ? 'Pass' : 'Fail'
  );

  const rTop = await api('GET', '/api/user/topup/info', null, r1.data.access_token, r1.data.user.id);
  record(
    'SM-API-TOPUP-R',
    'Reseller topup info 未被封鎖',
    'HTTP 200',
    `HTTP${rTop._status} success=${rTop.success}`,
    rTop._status === 200 ? 'Pass' : 'Fail'
  );

  // Isolation: reseller1 cannot see reseller2 customers
  const customers = await api('GET', '/api/reseller/customers', null, r1.data.access_token, r1.data.user.id);
  const foreign = await api('GET', '/api/reseller/customers/6', null, r1.data.access_token, r1.data.user.id);
  record(
    'SM-API-ISO',
    '跨代理商隔離',
    '外國客戶 404',
    `list=${customers._status}; foreign6=${foreign._status} msg=${foreign.message}`,
    foreign._status === 404 || foreign.success === false ? 'Pass' : 'Fail'
  );

  // Admin credit endpoint exists
  const ledger = await api('GET', '/api/admin/resellers/2', null, admin.data.access_token, admin.data.user.id);
  record(
    'SM-API-ADMIN-R',
    'Admin 讀取代理商詳情',
    'HTTP 200',
    `HTTP${ledger._status} success=${ledger.success}`,
    ledger._status === 200 && ledger.success ? 'Pass' : 'Fail'
  );

  // Register API still rejected
  const reg = await api('POST', '/api/user/register', { username: 'hacker' + Date.now(), password: 'Hacker12345!' }, null, null);
  record(
    'SM-API-REG',
    '公開註冊 API 仍關閉',
    'success=false',
    `HTTP${reg._status} success=${reg.success} msg=${reg.message}`,
    reg.success === false ? 'Pass' : 'Fail'
  );
}

await browser.close();
fs.writeFileSync('/workspace/b2b2c-pro-qa/retest/results.json', JSON.stringify(results, null, 2));
const pass = results.filter(r => r.result === 'Pass').length;
const fail = results.filter(r => r.result === 'Fail').length;
console.log(`\nSUMMARY pass=${pass} fail=${fail} total=${results.length}`);
