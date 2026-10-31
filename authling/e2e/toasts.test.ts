// Verify toast lifecycle across real navigation and the non-JavaScript fallback.
import { test, expect } from './setup';
import { completeSignup } from './fixtures/signup';

test('stacks feedback, pauses while focused, dismisses, and consumes result flags', async ({
  page,
  request,
  stack
}) => {
  await completeSignup(
    page,
    request,
    stack,
    'toasts@example.invalid',
    'a deliberately uncommon password'
  );
  await page.clock.install();
  await page.goto('/account?profile_updated=1&session_revoked=1');
  const notifications = page.getByRole('region', { name: 'Notifications' });
  await expect(notifications.getByRole('status')).toHaveCount(2);
  await expect(page).toHaveURL(`${stack.baseURL}/account`);
  const close = notifications.getByRole('button', { name: 'Dismiss notification' }).first();
  await close.focus();
  await page.clock.fastForward(9000);
  await expect(notifications.getByRole('status')).toHaveCount(1);
  await close.click();
  await expect(notifications).toHaveCount(0);
  await expect(page.locator('main')).toBeFocused();
  await page.reload();
  await expect(notifications).toHaveCount(0);
  await page.goto('/account?profile_updated=1');
  await expect(notifications).toBeVisible();
  await page.getByRole('link', { name: 'Security', exact: true }).click();
  await expect(notifications).toHaveCount(0);
});

test.describe('without JavaScript', () => {
  test.use({ javaScriptEnabled: false });
  test('keeps feedback visible inline', async ({ page, request, stack }) => {
    await completeSignup(
      page,
      request,
      stack,
      'native-toast@example.invalid',
      'a deliberately uncommon password'
    );
    await page.goto('/account?profile_updated=1');
    await expect(page.getByRole('status')).toHaveText('Your profile was updated.');
    await expect(page.getByRole('button', { name: 'Dismiss notification' })).toHaveCount(0);
  });
});
