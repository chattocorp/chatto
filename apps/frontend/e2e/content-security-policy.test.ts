import { expect, test } from './setup';

test('production frontend enforces its content security policy', async ({ page }) => {
  const response = await page.goto('/');

  expect(response).not.toBeNull();
  expect(response?.headers()['content-security-policy']).toBe("frame-ancestors 'none'");
  expect(response?.headers()['content-security-policy-report-only']).toBeUndefined();
  await expect(page.getByRole('heading', { name: 'Sign In' })).toBeVisible();

  const unauthorizedInlineScriptRan = await page.evaluate(async () => {
    const marker = '__chattoUnauthorizedInlineScriptRan';
    const script = document.createElement('script');
    script.textContent = `globalThis.${marker} = true`;
    document.head.append(script);
    await new Promise((resolve) => setTimeout(resolve, 0));
    return Reflect.get(globalThis, marker) === true;
  });

  expect(unauthorizedInlineScriptRan).toBe(false);
});
