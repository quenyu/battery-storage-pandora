// Run against an already running local Go service:
//   npm install --no-save playwright@1.62.1
//   npx playwright install chromium
//   node scripts/browser-smoke.cjs
// Use BROWSER_CHANNEL=chrome for installed Chrome, or BROWSER_EXECUTABLE for another Chromium.
// NODE_PATH may point at an existing installation of Playwright; no frontend build is needed.
const { chromium } = require('playwright');
const assert = require('node:assert/strict');
const fs = require('node:fs/promises');
const path = require('node:path');
const { randomUUID } = require('node:crypto');

async function main() {
  const base = (process.env.BASE_URL || 'http://127.0.0.1:8080').replace(/\/$/, '');
  assert.ok(['localhost', '127.0.0.1', '[::1]'].includes(new URL(base).hostname), 'Run only against a local server');
  const output = path.resolve(process.env.BROWSER_EVIDENCE_DIR || '.local/browser-evidence');
  await fs.mkdir(output, { recursive: true });
  const browser = await chromium.launch({
    headless: true,
    ...(process.env.BROWSER_CHANNEL ? { channel: process.env.BROWSER_CHANNEL } : {}),
    ...(process.env.BROWSER_EXECUTABLE ? { executablePath: process.env.BROWSER_EXECUTABLE } : {}),
  });
  const evidence = { startedAt: new Date().toISOString(), browserVersion: browser.version(), base, checks: [], requests: [], externalRequestsBlocked: [] };
  try {
    // A fresh context and disabled service workers exclude cached/CDN resources.
    const context = await browser.newContext({ serviceWorkers: 'block', viewport: { width: 1400, height: 1000 } });
    context.setDefaultTimeout(10000);
    await context.route('**/*', async route => {
      const url = new URL(route.request().url());
      if (['localhost', '127.0.0.1', '[::1]'].includes(url.hostname)) {
        return route.continue();
      }
      evidence.externalRequestsBlocked.push(url.href);
      return route.abort('blockedbyclient');
    });
    const probe = await context.newPage();
    await assert.rejects(() => probe.goto('https://example.com/offline-probe'), /ERR_BLOCKED_BY_CLIENT/);
    assert.equal(evidence.externalRequestsBlocked.length, 1);
    await probe.close();
    evidence.checks.push('External network probe blocked before Swagger loading');
    const page = await context.newPage();
    const errors = [];
    page.on('pageerror', error => errors.push(error.message));
    page.on('request', request => evidence.requests.push({ method: request.method(), url: request.url() }));
    const contractResponse = page.waitForResponse(response => response.url() === base + '/openapi.yaml');
    const loaded = await page.goto(base + '/swagger');
    assert.equal(page.url(), base + '/swagger/');
    const redirected = loaded.request().redirectedFrom();
    assert.ok(redirected, '/swagger must redirect');
    assert.equal((await redirected.response()).status(), 308);
    const contract = await contractResponse;
    assert.equal(contract.status(), 200);
    assert.deepEqual(await contract.body(), await fs.readFile(path.join(__dirname, '../docs/design/battery-storage-pandora/openapi.yaml')));
    await page.locator('.opblock').nth(18).waitFor();
    assert.equal(await page.locator('.opblock').count(), 19);
    assert.equal(await page.locator('.model-container').count(), 23);
    assert.equal(await page.locator('.errors-wrapper').count(), 0);
    evidence.checks.push('308 canonical redirect, 19 operations, 23 schemas, exact authoritative OpenAPI');
    await page.screenshot({ path: path.join(output, 'swagger-offline.png'), fullPage: true });

    const documented = await page.evaluate(() => {
      const spec = window.ui.specSelectors.specJson().toJS();
      return Object.values(spec.paths).flatMap(item => Object.values(item)
        .filter(operation => operation.operationId)
        .map(operation => ({ id: operation.operationId, responses: Object.keys(operation.responses) })));
    });
    evidence.operations = [];
    for (const operation of documented) {
      const block = page.locator('.opblock[id$="-' + operation.id + '"]');
      await block.locator('.opblock-summary-control').click();
      await block.getByRole('button', { name: 'Try it out', exact: true }).waitFor();
      for (const status of operation.responses) {
        assert.ok(await block.getByRole('cell', { name: status, exact: true }).count() > 0, operation.id + ' missing response ' + status);
      }
      assert.ok(await block.getByRole('tab', { name: 'Example Value', exact: true }).count() >= operation.responses.length, operation.id + ' response examples missing');
      evidence.operations.push(operation);
      await block.locator('.opblock-summary-control').click();
    }
    evidence.checks.push('All 19 operations expanded; every documented success/error response and response example rendered');

    const create = page.locator('.opblock').filter({ has: page.getByRole('button', { name: 'POST /employees Создать сотрудника со штрихкодом', exact: true }) });
    await create.getByRole('button', { name: 'POST /employees Создать сотрудника со штрихкодом', exact: true }).click();
    await create.getByRole('button', { name: 'Try it out', exact: true }).waitFor();
    assert.ok(await create.getByRole('tab', { name: 'Example Value', exact: true }).count() > 1);
    for (const status of ['201', '400', '404', '409', '413', '415', '500', '503']) {
      assert.ok(await create.getByRole('cell', { name: status, exact: true }).count() > 0, 'Missing documented response ' + status);
    }
    await create.getByRole('button', { name: 'Try it out', exact: true }).click();
    const key = randomUUID();
    const body = { name: 'Проверка Swagger', barcode: 'browser-' + randomUUID() };
    await create.getByRole('textbox', { name: 'Idempotency-Key', exact: true }).fill(key);
    await create.locator('textarea').fill(JSON.stringify(body, null, 2));
    const execute = async () => {
      const pending = page.waitForResponse(response => response.url() === base + '/api/v1/employees' && response.request().method() === 'POST');
      await create.getByRole('button', { name: 'Execute', exact: true }).click();
      const response = await pending;
      assert.equal(response.status(), 201);
      assert.equal(response.request().headers()['idempotency-key'], key);
      assert.deepEqual(response.request().postDataJSON(), body);
      return response.json();
    };
    const first = await execute();
    const replay = await execute();
    assert.deepEqual(replay, first);
    evidence.employeeId = first.id;
    evidence.idempotencyKey = key;
    evidence.barcode = body.barcode;
    evidence.checks.push('Try it out POST manually supplied header/JSON; repeat 201 with identical response and employee ID');
    await create.screenshot({ path: path.join(output, 'swagger-create-replay.png') });

    const read = page.locator('.opblock[id$="-getEmployee"]');
    await read.locator('.opblock-summary-control').click();
    await read.getByRole('button', { name: 'Try it out', exact: true }).click();
    await read.getByRole('textbox', { name: 'employee_id', exact: true }).fill(first.id);
    const pendingRead = page.waitForResponse(response => response.url() === base + '/api/v1/employees/' + first.id);
    await read.getByRole('button', { name: 'Execute', exact: true }).click();
    const readResponse = await pendingRead;
    assert.equal(readResponse.status(), 200);
    assert.deepEqual(await readResponse.json(), first);
    await read.screenshot({ path: path.join(output, 'swagger-read.png') });
    evidence.checks.push('Try it out GET employee returns persisted record');
    assert.deepEqual(errors, []);
    assert.equal(evidence.externalRequestsBlocked.length, 1, 'Swagger attempted to load an external resource');
    assert.ok(evidence.requests.every(request => new URL(request.url).origin === new URL(base).origin));
    evidence.checks.push('All Swagger/API requests same origin, zero page errors, no external runtime requests');
    evidence.result = 'PASS';
  } catch (error) {
    evidence.result = 'FAIL';
    evidence.error = error.stack;
    throw error;
  } finally {
    evidence.finishedAt = new Date().toISOString();
    await fs.writeFile(path.join(output, 'results.json'), JSON.stringify(evidence, null, 2));
    console.log(JSON.stringify(evidence, null, 2));
    await browser.close();
  }
}

main().catch(error => { console.error(error); process.exitCode = 1; });
