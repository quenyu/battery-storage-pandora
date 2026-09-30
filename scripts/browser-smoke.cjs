// Run against an already running local Go service:
//   node scripts/browser-smoke.cjs
// Playwright and a Chromium browser must already be available (optional QA tools).
// Use BROWSER_CHANNEL=chrome for installed Chrome, or BROWSER_EXECUTABLE for another Chromium.
// NODE_PATH may point at an existing installation of Playwright; no frontend build is needed.
const { chromium } = require('playwright');
const assert = require('node:assert/strict');
const fs = require('node:fs/promises');
const path = require('node:path');
const { randomUUID } = require('node:crypto');

async function main() {
  const base = (process.env.BASE_URL || 'http://127.0.0.1:18080').replace(/\/$/, '');
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
    assert.deepEqual(await contract.body(), await fs.readFile(path.join(__dirname, '../swagger/openapi.yaml')));
    await page.locator('.opblock').nth(18).waitFor();
    assert.equal(await page.locator('.opblock').count(), 19);
    assert.equal(await page.locator('.model-container').count(), 20);
    assert.equal(await page.locator('.errors-wrapper').count(), 0);
    evidence.checks.push('308 canonical redirect, 19 operations, 20 schemas, exact authoritative OpenAPI');
    assert.equal(await page.getByRole('button', { name: 'Authorize', exact: true }).count(), 0, 'Public API must not show an Authorize button');
    assert.equal((await page.locator('.info .description').allTextContents()).join(''), '', 'Swagger must omit its technical info description');
    assert.equal(await page.locator('.info .title small:visible').count(), 0, 'Swagger must omit version badges');
    await page.locator('.info').screenshot({ path: path.join(output, 'swagger-header.png') });

    const documented = await page.evaluate(() => {
      const spec = window.ui.specSelectors.specJson().toJS();
      for (const item of Object.values(spec.paths)) {
        for (const operation of Object.values(item).filter(value => value.operationId)) {
          if ((operation.parameters || []).some(parameter => ['limit', 'cursor'].includes(parameter.name))) {
            throw new Error('Swagger contains a removed pagination parameter');
          }
        }
      }
      for (const name of ['EmployeeList', 'CredentialList', 'BatteryList', 'OperationList']) {
        if (Object.keys(spec.components.schemas[name].properties).join(',') !== 'items') {
          throw new Error(name + ' must expose only items');
        }
      }
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

    const create = page.locator('.opblock[id$="-createEmployee"]');
    await create.locator('.opblock-summary-control').click();
    await create.getByRole('button', { name: 'Try it out', exact: true }).waitFor();
    assert.ok(await create.getByRole('tab', { name: 'Example Value', exact: true }).count() > 1);
    for (const status of ['201', '400', '404', '409', '422', '413', '415', '500', '503']) {
      assert.ok(await create.getByRole('cell', { name: status, exact: true }).count() > 0, 'Missing documented response ' + status);
    }
    await create.getByRole('button', { name: 'Try it out', exact: true }).click();
    const key = randomUUID();
    const body = { display_name: 'Проверка Swagger', personnel_number: 'browser-' + randomUUID() };
    await create.getByRole('textbox', { name: 'Idempotency-Key', exact: true }).fill(key);
    await create.locator('textarea').fill(JSON.stringify(body, null, 2));
    const execute = async (expectReplay = false) => {
      const pending = page.waitForResponse(response => response.url() === base + '/api/employees' && response.request().method() === 'POST');
      await create.getByRole('button', { name: 'Execute', exact: true }).click();
      const response = await pending;
      assert.equal(response.status(), 201);
      assert.equal(response.request().headers()['idempotency-key'], key);
      assert.equal(response.request().headers().authorization, undefined);
      assert.deepEqual(response.request().postDataJSON(), body);
      if (expectReplay) assert.equal(response.headers()['idempotency-replayed'], 'true');
      return response.json();
    };
    const first = await execute();
    const replay = await execute(true);
    assert.deepEqual(replay, first);
    evidence.employeeId = first.id;
    evidence.idempotencyKey = key;
    evidence.personnelNumber = body.personnel_number;
    evidence.checks.push('Try it out POST manually supplied header/JSON; repeat 201 with identical response and employee ID');
    const requestURL = create.locator('.request-url');
    const live = create.locator('.live-responses-table');
    await live.waitFor();
    await requestURL.evaluate(element => window.scrollTo(0, window.scrollY + element.getBoundingClientRect().top - 35));
    const bounds = await create.boundingBox();
    const top = await requestURL.boundingBox();
    const bottom = await live.boundingBox();
    await page.screenshot({ path: path.join(output, 'swagger-result.png'), clip: { x: bounds.x, y: top.y, width: bounds.width, height: bottom.y + bottom.height - top.y } });

    const read = page.locator('.opblock[id$="-getEmployee"]');
    await read.locator('.opblock-summary-control').click();
    await read.getByRole('button', { name: 'Try it out', exact: true }).click();
    await read.getByRole('textbox', { name: 'employee_id', exact: true }).fill(first.id);
    const pendingRead = page.waitForResponse(response => response.url() === base + '/api/employees/' + first.id);
    await read.getByRole('button', { name: 'Execute', exact: true }).click();
    const readResponse = await pendingRead;
    assert.equal(readResponse.status(), 200);
    assert.deepEqual(await readResponse.json(), first);
    evidence.checks.push('Try it out GET employee returns persisted record');

    const list = page.locator('.opblock[id$="-listEmployees"]');
    await list.locator('.opblock-summary-control').click();
    await list.getByRole('button', { name: 'Try it out', exact: true }).click();
    const pendingList = page.waitForResponse(response => response.url() === base + '/api/employees' && response.request().method() === 'GET');
    await list.getByRole('button', { name: 'Execute', exact: true }).click();
    const listResponse = await pendingList;
    assert.equal(listResponse.status(), 200);
    const employees = await listResponse.json();
    assert.deepEqual(Object.keys(employees), ['items']);
    assert.ok(employees.items.some(employee => employee.id === first.id));
    evidence.checks.push('Try it out lists employees as items without pagination parameters or continuation fields');

    const credentialValue = 'browser-card-' + randomUUID();
    const card = page.locator('.opblock[id$="-createCredential"]');
    await card.locator('.opblock-summary-control').click();
    await card.getByRole('button', { name: 'Try it out', exact: true }).click();
    await card.getByRole('textbox', { name: 'employee_id', exact: true }).fill(first.id);
    await card.getByRole('textbox', { name: 'Idempotency-Key', exact: true }).fill(randomUUID());
    await card.locator('textarea').fill(JSON.stringify({ value: credentialValue }));
    const pendingCard = page.waitForResponse(response => response.url() === base + '/api/employees/' + first.id + '/credentials' && response.request().method() === 'POST');
    await card.getByRole('button', { name: 'Execute', exact: true }).click();
    assert.equal((await pendingCard).status(), 201);
    const address = (Number.parseInt(randomUUID().slice(0, 8), 16) + 1) + '.1.1';
    const store = page.locator('.opblock[id$="-storeNewBattery"]');
    await store.locator('.opblock-summary-control').click();
    await store.getByRole('button', { name: 'Try it out', exact: true }).click();
    await store.getByRole('textbox', { name: 'Idempotency-Key', exact: true }).fill(randomUUID());
    await store.locator('textarea').fill(JSON.stringify({ inventory_code: 'browser-battery-' + randomUUID(), actor_credential_value: credentialValue, destination_location: address }));
    const pendingStore = page.waitForResponse(response => response.url() === base + '/api/batteries' && response.request().method() === 'POST');
    await store.getByRole('button', { name: 'Execute', exact: true }).click();
    const storeResponse = await pendingStore;
    assert.equal(storeResponse.status(), 201);
    const stored = await storeResponse.json();
    assert.equal(stored.battery.current_location, address);
    assert.equal(stored.operation.type, 'STORE');
    assert.equal(stored.operation.destination_location, address);
    evidence.batteryId = stored.battery.id;
    evidence.location = address;
    evidence.checks.push('Try it out creates a separate credential and STORE at a canonical numeric address');
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
