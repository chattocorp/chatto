import { test } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';
import { ExplorePage } from './pages/ExplorePage';
import * as routes from './routes';

// Issue #330 / ADR-027: in a single-server world the Browse Spaces page lists
// exactly one space (the bootstrap primary). Tests that depended on multiple
// spaces or a "not joined yet" state were removed; what remains exercises the
// "I land on Browse Spaces and see my server" flow.
const BOOTSTRAP_SPACE = 'E2E Test Server';

test.describe('Browse Spaces Directory', () => {
  test('shows the bootstrap space with a Joined badge for the signed-in user', async ({
    page,
    chatPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();

    const explorePage = new ExplorePage(page);
    await explorePage.goto();

    await explorePage.expectSpaceJoined(BOOTSTRAP_SPACE);
  });

  test('clicking the joined space card navigates into the server', async ({
    page,
    chatPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();

    const explorePage = new ExplorePage(page);
    await explorePage.goto();

    const spaceCard = explorePage.getSpaceItem(BOOTSTRAP_SPACE);
    await spaceCard.getByRole('link', { name: 'Joined' }).click();

    await page.waitForURL(routes.patterns.anySpace);
  });
});
