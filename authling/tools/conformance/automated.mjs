// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: AGPL-3.0-or-later
import { chromium } from '@playwright/test';
import { readFile, writeFile } from 'node:fs/promises';
import { pathToFileURL } from 'node:url';
import { randomBytes } from 'node:crypto';
import { setTimeout as delay } from 'node:timers/promises';
import path from 'node:path';

// These warnings describe the documented Authling claim contract. A different
// warning is a regression, even if the suite still labels the module WARNING.
const expectedWarnings = {
  'oidcc-server': ['EnsureIdTokenDoesNotContainNonRequestedClaims'],
  'oidcc-scope-profile': ['VerifyScopesReturnedInUserInfoClaims'],
  'oidcc-scope-email': ['EnsureIdTokenDoesNotContainEmailForScopeEmail'],
};

function failure(message) {
  return Object.assign(new Error(message), { safeMessage: message });
}

// Run a bounded regression selection, not a certification submission. Each
// module must finish; suite skips and review requests are not successful checks.
export async function runAutomated({ api, suite, issuer, state, configPath, summaryPath, checkRunning, clientDriver, clientEnv, signal }) {
  const results = [];
  const browser = await chromium.launch({ args: ['--host-resolver-rules=MAP conformance.localhost 127.0.0.1'] });
  const context = await browser.newContext({ ignoreHTTPSErrors: true });
  const page = await context.newPage();
  page.setDefaultTimeout(15_000);
  const password = randomBytes(24).toString('hex');
  const email = `conformance-${randomBytes(8).toString('hex')}@example.invalid`;
  try {
    await page.goto(`${issuer}/signup`);
    await page.getByLabel('Email address').fill(email);
    await page.getByRole('button', { name: 'Email me a code' }).click();
    let code;
    for (let attempt = 0; attempt < 100 && !code; attempt++) {
      checkRunning();
      const mailbox = await (await fetch('http://127.0.0.1:19409/api/v1/messages')).json();
      if (mailbox.messages[0]) {
        const message = await (await fetch(`http://127.0.0.1:19409/api/v1/message/${mailbox.messages[0].ID}`)).json();
        code = /verification code is ([0-9]{6})\./.exec(message.Text)?.[1];
      }
      if (!code) await delay(100);
    }
    if (!code) throw new Error('Conformance signup email did not arrive');
    await page.getByLabel('Verification code').fill(code);
    await page.getByRole('button', { name: 'Verify email' }).click();
    await page.getByLabel('Password', { exact: true }).fill(password);
    await page.getByLabel('Confirm password').fill(password);
    await page.getByRole('button', { name: 'Create account' }).click();
    await page.getByRole('link', { name: 'Edit profile' }).click();
    await page.getByLabel('Preferred username').fill('conformance-handle');
    await page.getByLabel('Full name').fill('Conformance Test Person');
    await page.getByRole('button', { name: 'Save profile' }).click();

    async function run(plan, name, { drive, variant, configFile, expectedSkip = false, scenario } = {}) {
      const query = new URLSearchParams({ test: name });
      if (plan) query.set('plan', plan);
      if (variant) query.set('variant', JSON.stringify(variant));
      const test = await api(`/api/runner?${query}`, configFile ?? null);
      await api(`/api/runner/${test.id}`, null);
      const visited = new Set();
      let driverError;
      if (drive) {
        // start is asynchronous. Client modules must be waiting for requests
        // before the driver fetches metadata or begins authorization.
        let ready = false;
        for (let attempt = 0; attempt < 120; attempt++) {
          checkRunning();
          const info = await api(`/api/info/${test.id}`);
          if (info.status === 'WAITING') { ready = true; break; }
          if (['FINISHED', 'INTERRUPTED'].includes(info.status)) break;
          await delay(100);
        }
        if (!ready) driverError = failure(`Client module ${name} did not become ready`);
        else try { await drive(); } catch (error) { driverError = error; }
      }
      for (let attempt = 0; attempt < 120; attempt++) {
        checkRunning();
        const info = await api(`/api/info/${test.id}`);
        if (['FINISHED', 'INTERRUPTED'].includes(info.status)) {
          const logs = await api(`/api/log/${test.id}`);
          const findings = [...new Set(logs.filter((entry) => ['WARNING', 'FAILURE'].includes(entry.result))
            .map((entry) => entry.src).filter((src) => typeof src === 'string' && /^[A-Za-z0-9_.]+$/.test(src)))];
          const result = { module: name, scenario, variant, status: info.status, result: info.result, findings,
            clientChecked: Boolean(drive) && !driverError,
            clientAssertion: driverError ? (driverError.safeMessage ?? 'Client driver failed') : undefined };
          results.push(result);
          console.log(`${name}: ${info.result}${findings.length ? ` (${findings.join(', ')})` : ''}`);
          if (driverError) throw driverError;
          // Some official client modules classify deliberate rejection of an
          // optional capability as SKIPPED. Preserve that outcome and require a
          // successful independent client assertion; never count it as PASSED.
          if (expectedSkip && drive && info.status === 'FINISHED' && info.result === 'SKIPPED') return;
          if (info.status !== 'FINISHED' || !['PASSED', 'WARNING'].includes(info.result)) {
            throw failure(`Conformance module ${name} did not pass (${info.result ?? info.status})`);
          }
          if (info.result === 'WARNING' && (findings.length === 0 || findings.some((finding) =>
            !(expectedWarnings[name] ?? []).includes(finding)))) {
            throw failure(`Conformance module ${name} has a new warning`);
          }
          return;
        }
        if (!drive) {
          const pending = await api(`/api/runner/browser/${test.id}`);
          for (const url of pending.urls ?? []) {
            if (visited.has(url)) continue;
            visited.add(url);
            await page.goto(url);
            if (await page.getByLabel('Password', { exact: true }).isVisible()) {
              await page.getByLabel('Email address').fill(email);
              await page.getByLabel('Password', { exact: true }).fill(password);
              await page.getByRole('button', { name: 'Sign in', exact: true }).click();
            }
            if (await page.getByRole('button', { name: 'Authorize', exact: true }).isVisible()) {
              await page.getByRole('button', { name: 'Authorize', exact: true }).click();
            }
          }
        }
        await delay(500);
      }
      results.push({ module: name, scenario, variant, status: 'TIMEOUT', result: 'FAILED',
        clientAssertion: driverError ? (driverError.safeMessage ?? 'Client driver failed') : undefined });
      throw driverError ?? failure(`Conformance module ${name} timed out`);
    }

    const discovery = await api('/api/plan?planName=oidcc-config-certification-test-plan', configPath);
    await run(discovery.id, 'oidcc-discovery-endpoint-verification');
    const variant = { server_metadata: 'discovery', client_registration: 'static_client' };
    const provider = await api(`/api/plan?planName=oidcc-basic-certification-test-plan&variant=${encodeURIComponent(JSON.stringify(variant))}`, configPath);
    for (const name of ['oidcc-server', 'oidcc-scope-profile', 'oidcc-scope-email',
      'oidcc-server-client-secret-post', 'oidcc-ensure-request-with-valid-pkce-succeeds']) {
      await run(provider.id, name);
    }
    const combinedConfig = JSON.parse(await readFile(configPath, 'utf8'));
    combinedConfig.client.scope = 'openid profile email';
    const combinedPath = path.join(state, 'combined-config.json');
    await writeFile(combinedPath, JSON.stringify(combinedConfig), { mode: 0o600 });
    const combined = await api(`/api/plan?planName=oidcc-basic-certification-test-plan&variant=${encodeURIComponent(JSON.stringify(variant))}`, combinedPath);
    await run(combined.id, 'oidcc-server', { scenario: 'openid profile email' });
    if (clientDriver) {
      // An explicit external module owns its application's config and driver.
      // Authling remains runnable without that application or repository layout.
      const { runClientChecks } = await import(pathToFileURL(clientDriver).href);
      await runClientChecks({ api, run, suite, state, env: clientEnv, signal, fail: failure });
    }
  } finally {
    await browser.close();
    // Only outcomes are suitable for CI artifacts. Raw suite state contains secrets.
    await writeFile(summaryPath, JSON.stringify(results, null, 2));
  }
}
