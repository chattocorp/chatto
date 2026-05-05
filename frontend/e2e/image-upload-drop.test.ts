import { expect, type Page } from '@playwright/test';
import { test } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';
import { simulateDragEnter, simulateDragLeave, simulateFileDrop } from './helpers/dragDrop';
import * as routes from './routes';
import { TIMEOUTS } from './constants';

/**
 * Creates a space via GraphQL API and returns its ID.
 */
async function createSpaceViaAPI(page: Page): Promise<string> {
  const response = await page.request.post('/api/graphql', {
    headers: {
      'Content-Type': 'application/json',
      'X-REQUEST-TYPE': 'GraphQL'
    },
    data: {
      query: `
        mutation CreateSpace($input: CreateSpaceInput!) {
          createSpace(input: $input) { id }
        }
      `,
      variables: { input: { name: `Drop Test Space ${Date.now()}` } }
    }
  });

  const data = await response.json();
  return data.data.createSpace.id;
}

test.describe('drag and drop image upload on settings pages', () => {
  test.describe('user avatar', () => {
    test('drop zone overlay appears when dragging image over avatar section', async ({ page }) => {
      await createAndLoginTestUser(page);
      await page.goto(routes.settings);
      await page.getByPlaceholder('Enter your display name').waitFor({ state: 'visible' });

      const avatarDropZone = page.getByTestId('avatar-drop-zone');
      await simulateDragEnter(avatarDropZone);

      await expect(page.getByText('Drop image')).toBeVisible();
      await expect(page.getByText('Upload as your avatar')).toBeVisible();
    });

    test('drop zone overlay disappears when dragging away', async ({ page }) => {
      await createAndLoginTestUser(page);
      await page.goto(routes.settings);
      await page.getByPlaceholder('Enter your display name').waitFor({ state: 'visible' });

      const avatarDropZone = page.getByTestId('avatar-drop-zone');
      await simulateDragEnter(avatarDropZone);
      await expect(page.getByText('Drop image')).toBeVisible();

      await simulateDragLeave(avatarDropZone);
      await expect(page.getByText('Drop image')).not.toBeVisible();
    });

    test('dropping image uploads avatar', async ({ page }) => {
      await createAndLoginTestUser(page);
      await page.goto(routes.settings);
      await page.getByPlaceholder('Enter your display name').waitFor({ state: 'visible' });

      const avatarDropZone = page.getByTestId('avatar-drop-zone');
      await simulateFileDrop(avatarDropZone, 'e2e/fixtures/brighton.jpg');

      await expect(page.getByText('Avatar uploaded successfully')).toBeVisible({ timeout: TIMEOUTS.COMPLEX_OPERATION });
    });
  });

  // FIXME #330: createSpaceViaAPI relies on the createSpace mutation which is
  // gone. Re-enable in phase 5 once the test exercises the bootstrap space
  // with admin creds.
  test.describe.skip('space logo', () => {
    test('drop zone overlay appears when dragging image over logo section', async ({
      page,
      spaceAdminPage
    }) => {
      await createAndLoginTestUser(page);
      const spaceId = await createSpaceViaAPI(page);
      await spaceAdminPage.gotoGeneralDirectly(spaceId);

      const logoDropZone = page.getByTestId('logo-drop-zone');
      await simulateDragEnter(logoDropZone);

      await expect(page.getByText('Upload as space logo')).toBeVisible();
    });

    test('dropping image uploads logo', async ({ page, spaceAdminPage }) => {
      await createAndLoginTestUser(page);
      const spaceId = await createSpaceViaAPI(page);
      await spaceAdminPage.gotoGeneralDirectly(spaceId);

      const logoDropZone = page.getByTestId('logo-drop-zone');
      await simulateFileDrop(logoDropZone, 'e2e/fixtures/brighton.jpg');

      await expect(page.getByText('Logo uploaded successfully')).toBeVisible({ timeout: TIMEOUTS.COMPLEX_OPERATION });
    });
  });

  // FIXME #330: see "space logo" describe above.
  test.describe.skip('space banner', () => {
    test('drop zone overlay appears when dragging image over banner section', async ({
      page,
      spaceAdminPage
    }) => {
      await createAndLoginTestUser(page);
      const spaceId = await createSpaceViaAPI(page);
      await spaceAdminPage.gotoGeneralDirectly(spaceId);

      const bannerDropZone = page.getByTestId('banner-drop-zone');
      await simulateDragEnter(bannerDropZone);

      await expect(page.getByText('Upload as space banner')).toBeVisible();
    });

    test('dropping image uploads banner', async ({ page, spaceAdminPage }) => {
      await createAndLoginTestUser(page);
      const spaceId = await createSpaceViaAPI(page);
      await spaceAdminPage.gotoGeneralDirectly(spaceId);

      const bannerDropZone = page.getByTestId('banner-drop-zone');
      await simulateFileDrop(bannerDropZone, 'e2e/fixtures/brighton.jpg');

      await expect(page.getByText('Banner uploaded successfully')).toBeVisible({ timeout: TIMEOUTS.COMPLEX_OPERATION });
    });
  });
});
