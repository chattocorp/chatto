import { test, expect } from './setup';
import { seedData, loginSeededUser } from './fixtures/seed';

test('seeded conversations and thread replies are readable in the browser', async ({
  page,
  chatPage,
  roomPage
}) => {
  const browserErrors: string[] = [];
  page.on('pageerror', (error) => browserErrors.push(error.message));
  page.on('console', (message) => {
    if (message.type() === 'error') browserErrors.push(message.text());
  });
  const scene = await seedData(page.request, {
    seed: 42,
    users: 3,
    rooms: 2,
    messages: 12,
    threadReplies: 4
  });
  await loginSeededUser(page.request, scene.users[0]);
  const reply = scene.messages[8];
  const room = scene.rooms.find((room) => room.id === reply.roomId)!;
  const root = scene.messages.find((message) => message.id === reply.threadRootId)!;
  await chatPage.goto();
  await chatPage.enterRoom(room.name);
  await roomPage.expectMessageVisible(root.body);
  await roomPage.getMessageByEventId(root.id).openThread();
  await roomPage.expectThreadPaneVisible();
  await expect(page.getByText(reply.body, { exact: true })).toBeVisible();
  expect(browserErrors).toEqual([]);
});
