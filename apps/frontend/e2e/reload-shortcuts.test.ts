import { test, expect } from './setup';

test('production reload shortcuts remain unhandled while a modal input is focused', async ({
  page
}) => {
  await page.goto('/chat/servers');
  await page.getByRole('button', { name: 'Connect by address', exact: true }).click();
  const input = page.getByRole('dialog').getByLabel('Server URL');
  await input.fill('unfinished.example');
  await page.evaluate(() => {
    window.addEventListener(
      'keydown',
      (event) => {
        if (event.key === 'F5' || event.key.toLowerCase() === 'r') {
          queueMicrotask(() => {
            document.documentElement.dataset.reloadPrevented = String(event.defaultPrevented);
          });
        }
      },
      { capture: true }
    );
  });
  for (const shortcut of [
    'Control+r',
    'Meta+r',
    'Control+Shift+r',
    'Meta+Shift+r',
    'F5',
    'Control+F5',
    'Shift+F5'
  ]) {
    await page.evaluate(() => delete document.documentElement.dataset.reloadPrevented);
    await page.keyboard.press(shortcut);
    await expect(page.locator('html')).toHaveAttribute('data-reload-prevented', 'false');
  }
});
