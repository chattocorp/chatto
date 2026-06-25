import type { Page } from '@playwright/test';

const CSRF_COOKIE_NAME = 'chatto_csrf';
export const CSRF_HEADER_NAME = 'X-CSRF-Token';

export async function csrfHeaders(page: Page): Promise<Record<string, string>> {
  // API-only login helpers can create or swap cookie sessions without a page
  // navigation. Hit a safe route first so the server mirrors a fresh CSRF
  // cookie for the current session before we read it from the browser context.
  await page.request.get('/');

  const cookies = await page.context().cookies();
  const token = cookies.find((cookie) => cookie.name === CSRF_COOKIE_NAME)?.value;
  return token ? { [CSRF_HEADER_NAME]: token } : {};
}
