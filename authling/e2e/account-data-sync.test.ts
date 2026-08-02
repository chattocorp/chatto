import { randomUUID } from 'node:crypto';
import path from 'node:path';
import { completeSignup } from './fixtures/signup';
import { restartAuthling } from './fixtures/stack';
import { expect, test } from './setup';

const password = 'correct horse battery staple';
const clientBundle = path.resolve('e2e/.generated/tinybase-client.js');

test('syncs account data across devices, offline edits, deletion, and restart', async ({
  browser,
  page,
  request,
  stack
}, testInfo) => {
  const email = `data-${randomUUID()}@example.invalid`;
  await page.context().addInitScript({ path: clientBundle });
  await completeSignup(page, request, stack, email, password);

  const secondContext = await browser.newContext({ baseURL: stack.baseURL });
  await secondContext.addInitScript({ path: clientBundle });
  const secondPage = await secondContext.newPage();
  try {
    await secondPage.goto('/login');
    await secondPage.getByLabel('Email address').fill(email);
    await secondPage.getByLabel('Password').fill(password);
    await secondPage.getByRole('button', { name: 'Sign in' }).click();
    await expect(secondPage.getByRole('heading', { name: 'Your account' })).toBeVisible();

    await page.evaluate(async () => {
      await authlingTinyBase.create('first', 'browser-device-a');
      authlingTinyBase.setRow('first', 'servers', 'one', {
        name: 'First server',
        url: 'https://one.example'
      });
      authlingTinyBase.setValue('first', 'preferences', {
        nested: { __authling_tinybase_undefined: true },
        reserved: '\uFFFC'
      });
      await authlingTinyBase.connect('first');
    });
    await secondPage.evaluate(async () => {
      await authlingTinyBase.create('second', 'browser-device-b');
      await authlingTinyBase.connect('second');
    });
    await expect
      .poll(() =>
        secondPage.evaluate(() => authlingTinyBase.getCell('second', 'servers', 'one', 'name'))
      )
      .toBe('First server');
    await expect
      .poll(() => secondPage.evaluate(() => authlingTinyBase.getValue('second', 'preferences')))
      .toEqual({ nested: { __authling_tinybase_undefined: true }, reserved: '\uFFFC' });

    await page.evaluate(async () => {
      await authlingTinyBase.disconnect('first');
      authlingTinyBase.setValue('first', 'theme', 'light');
    });
    await new Promise((resolve) => setTimeout(resolve, 10));
    const beforeDark = await secondPage.evaluate(() => authlingTinyBase.syncStats('second'));
    await secondPage.evaluate(() => authlingTinyBase.setValue('second', 'theme', 'dark'));
    await expect
      .poll(async () => {
        const current = await secondPage.evaluate(() => authlingTinyBase.syncStats('second'));
        return current.sends > beforeDark.sends && current.receives > beforeDark.receives;
      })
      .toBe(true);
    await page.evaluate(() => authlingTinyBase.reconnect('first'));
    await expect
      .poll(async () => {
        const [firstTheme, secondTheme] = await Promise.all([
          page.evaluate(() => authlingTinyBase.getValue('first', 'theme')),
          secondPage.evaluate(() => authlingTinyBase.getValue('second', 'theme'))
        ]);
        return firstTheme === secondTheme && (firstTheme === 'light' || firstTheme === 'dark');
      })
      .toBe(true);

    await restartAuthling(stack, testInfo);
    await Promise.all([
      page.evaluate(() => authlingTinyBase.reconnect('first')),
      secondPage.evaluate(() => authlingTinyBase.reconnect('second'))
    ]);
    await expect
      .poll(() =>
        secondPage.evaluate(() => authlingTinyBase.getCell('second', 'servers', 'one', 'url'))
      )
      .toBe('https://one.example');
    await expect
      .poll(() => page.evaluate(() => authlingTinyBase.getValue('first', 'preferences')))
      .toEqual({ nested: { __authling_tinybase_undefined: true }, reserved: '\uFFFC' });

    await secondPage.evaluate(() => authlingTinyBase.delRow('second', 'servers', 'one'));
    await expect
      .poll(() => page.evaluate(() => authlingTinyBase.hasRow('first', 'servers', 'one')))
      .toBe(false);
  } finally {
    await secondContext.close();
  }
});
