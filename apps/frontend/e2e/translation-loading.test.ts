import { readFileSync } from 'node:fs';
import { test, expect } from './setup';
import { loginAsAdmin } from './fixtures/testUser';

/** Use the emitted graph because production chunk filenames contain content hashes. */
function managementPayloads(): Map<string, string> {
  const manifest = JSON.parse(
    readFileSync(
      new URL('../.svelte-kit/output/client/.vite/manifest.json', import.meta.url),
      'utf8'
    )
  ) as Record<string, { name?: string; file: string }>;
  return new Map(
    Object.values(manifest)
      .filter((chunk) => chunk.name?.endsWith('-management'))
      .map((chunk) => [`/${chunk.file}`, chunk.name!])
  );
}

for (const locale of ['en-GB', 'de-DE']) {
  test(`management translations load on entry and stay cached: ${locale}`, async ({ page }) => {
    const payloads = managementPayloads();
    const requests: string[] = [];
    const errors: string[] = [];
    page.on('pageerror', (error) => errors.push(error.message));
    page.on('request', (request) => {
      const name = payloads.get(new URL(request.url()).pathname);
      if (name) requests.push(name);
    });
    await page.addInitScript((language) => localStorage.setItem('chatto:locale', language), locale);
    await loginAsAdmin(page);
    await page.goto('/chat/-/settings/appearance');
    const general = page.locator('a[href="/chat/-/manage/server/general"]');
    await expect(general).toBeVisible();
    expect(requests).toEqual([]);
    await expect(page.locator('body')).not.toContainText('⟦');

    await general.click();
    await expect(page.locator('input#name')).toBeVisible();
    await expect(page.locator('body')).not.toContainText('⟦');
    const expected =
      locale === 'en-GB'
        ? ['lingua-en-GB-management']
        : ['lingua-en-GB-management', 'lingua-de-DE-management'];
    expect(requests.sort()).toEqual(expected.sort());

    await page.locator('a[href="/chat/-/settings/appearance"]').click();
    await expect(general).toBeVisible();
    await general.click();
    await expect(page.locator('input#name')).toBeVisible();
    expect(requests.sort()).toEqual(expected.sort());
    expect(errors).toEqual([]);
  });
}

test('a failed English management download offers retry and a direct reload succeeds', async ({
  page
}) => {
  const payloads = managementPayloads();
  const english = [...payloads].find(([, name]) => name === 'lingua-en-GB-management');
  expect(english).toBeDefined();
  await page.addInitScript(() => localStorage.setItem('chatto:locale', 'en-GB'));
  await loginAsAdmin(page);
  await page.goto('/chat/-/settings/appearance');
  await page.route(`**${english![0]}`, (route) => route.abort());
  await page.locator('a[href="/chat/-/manage/server/general"]').click();
  await expect(page.getByRole('button', { name: 'Try Again', exact: true })).toBeVisible();
  await page.unroute(`**${english![0]}`);
  await page.getByRole('button', { name: 'Try Again', exact: true }).click();
  await expect(page.locator('input#name')).toBeVisible();
  await expect(page.locator('body')).not.toContainText('⟦');
});
