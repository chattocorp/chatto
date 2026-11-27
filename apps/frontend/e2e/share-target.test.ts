import { test, expect } from './setup';
import { withBootstrapAdminRequest } from './fixtures/adminRequest';
import {
  createRoomViaConnect,
  getDefaultRoomGroupIdViaConnect,
  joinRoomViaConnect
} from './fixtures/connectHelpers';
import { createAndLoginTestUser } from './fixtures/testUser';

test('a rejected share returns to the picker and appends once to the next draft', async ({
  page,
  roomPage,
  serverURL,
  chatPage
}) => {
  await createAndLoginTestUser(page);
  const roomIds = await withBootstrapAdminRequest(serverURL, async (request) => {
    const group = await getDefaultRoomGroupIdViaConnect(request);
    const ids = [];
    for (const name of ['field-notes', 'photography', 'seafront', 'trips', 'weekend']) {
      ids.push(await createRoomViaConnect(request, name, group));
    }
    return ids;
  });
  for (const id of roomIds) await joinRoomViaConnect(page, id);
  await chatPage.goto();
  await chatPage.enterRoom('photography');
  await roomPage.messageInput.fill('Photo notes');
  await chatPage.enterRoom('general');
  await roomPage.messageInput.fill('Already here');
  await expect
    .poll(() =>
      page.evaluate(
        async () => (await navigator.serviceWorker.getRegistration('/share-target/'))?.active?.state
      )
    )
    .toBe('activated');

  // A top-level multipart navigation is how the Android share target delivers files.
  // The sender is outside the worker's scope, just as on a cold app launch.
  const sent: string[] = [];
  page.on('request', (request) => {
    if (request.url().endsWith('/CreateMessage')) sent.push(request.url());
  });
  await page.evaluate(async () => {
    const form = document.createElement('form');
    const manifestLink = document.querySelector<HTMLLinkElement>('link[rel=manifest]')!;
    const manifest = await (await fetch(manifestLink.href)).json();
    const target = manifest.share_target;
    form.method = target.method;
    form.enctype = target.enctype;
    form.action = target.action;
    for (const [name, value] of [
      ['text', 'Interesting https://example.org/'],
      ['url', 'https://example.org/']
    ]) {
      const input = document.createElement('input');
      input.name = target.params[name];
      input.value = value;
      form.append(input);
    }
    const transfer = new DataTransfer();
    const png = await (await fetch('/icons/icon-192.png')).blob();
    transfer.items.add(new File([png], 'photo.png', { type: 'image/png' }));
    const input = document.createElement('input');
    input.name = target.params.files[0].name;
    input.type = 'file';
    input.files = transfer.files;
    form.append(input);
    document.body.append(form);
    form.submit();
  });
  await expect(page.getByText('photo.png', { exact: true })).toBeVisible();
  const usedTicket = page.url();
  const search = page.getByRole('textbox', { name: 'Search rooms...' });
  await search.fill('#gen');
  await expect(page.getByRole('button', { name: 'photography' })).toHaveCount(0);
  // Reject the first draft write as a browser with exhausted session storage would.
  // The original draft and the pending images must survive this failure.
  await page.evaluate(() => {
    const set = Storage.prototype.setItem;
    Storage.prototype.setItem = function (key, value) {
      if (
        this === sessionStorage &&
        key.startsWith('chatto:draft:') &&
        value.includes('Interesting')
      ) {
        Storage.prototype.setItem = set;
        throw new DOMException('Draft storage is full', 'QuotaExceededError');
      }
      return set.call(this, key, value);
    };
  });
  await page.getByRole('button', { name: 'general', exact: true }).click();
  await expect(page.getByText('photo.png', { exact: true })).toBeVisible();
  await expect(page.getByRole('button', { name: 'photography', exact: true })).toBeVisible();
  expect(
    await page.evaluate(() =>
      Object.keys(sessionStorage)
        .filter((key) => key.startsWith('chatto:draft:'))
        .map((key) => sessionStorage.getItem(key))
    )
  ).toContain('Already here');
  await page.getByRole('button', { name: 'photography', exact: true }).click();
  await expect(roomPage.messageInput).toHaveText(
    /^Photo notes\s*Interesting https:\/\/example\.org\/$/
  );
  await expect(page.getByTestId('composer-attachment-preview')).toHaveCount(1);
  expect(sent).toEqual([]);
  await page.reload();
  await expect(roomPage.messageInput).toHaveText(
    /^Photo notes\s*Interesting https:\/\/example\.org\/$/
  );
  await page.goto(usedTicket);
  await expect(page.getByText('This share is unavailable.', { exact: false })).toBeVisible();
  await expect(page.getByRole('button', { name: 'general', exact: true })).toHaveCount(0);
  await page.getByRole('button', { name: 'Cancel' }).click();
  await expect(page).not.toHaveURL(/\/share/);
});
