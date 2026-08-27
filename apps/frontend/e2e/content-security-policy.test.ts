import { expect, test } from './setup';

test('production frontend enforces CSP and Trusted Types', async ({ page }) => {
  const response = await page.goto('/');

  expect(response).not.toBeNull();
  expect(response?.headers()['content-security-policy']).toBe("frame-ancestors 'none'");
  expect(response?.headers()['content-security-policy-report-only']).toBeUndefined();
  await expect(page.getByRole('heading', { name: 'Sign In' })).toBeVisible();

  const enforcement = await page.evaluate(() => {
    const result = {
      stringHtmlRejected: false,
      unknownPolicyRejected: false
    };

    try {
      document.createElement('div').innerHTML = '<p>untrusted</p>';
    } catch (error) {
      result.stringHtmlRejected = error instanceof TypeError;
    }

    try {
      globalThis.trustedTypes?.createPolicy('chatto-e2e-unknown', {
        createHTML: (value) => value
      });
    } catch (error) {
      result.unknownPolicyRejected = error instanceof TypeError;
    }

    return result;
  });

  expect(enforcement).toEqual({
    stringHtmlRejected: true,
    unknownPolicyRejected: true
  });
});
