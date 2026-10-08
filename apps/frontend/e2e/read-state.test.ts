/** Verify persisted read positions across browser visibility, scrolling, and RPC failures. */
import { expect, type Page } from '@playwright/test';
import { test } from './setup';
import { loginAndEnterRoom, withLoggedInServerWindow } from './fixtures/serverUser';
import {
  getReadMarkerViaConnect,
  postMessageViaConnect,
  postThreadReplyViaConnect,
  waitForReadMarkerViaConnect
} from './fixtures/connectHelpers';
import * as routes from './routes';

/** Synthetic visibility avoids dependence on how headless Chromium focuses windows. */
async function setVisible(page: Page, visible: boolean) {
  await page.evaluate((visible) => {
    Object.defineProperty(document, 'visibilityState', {
      value: visible ? 'visible' : 'hidden',
      configurable: true
    });
    document.dispatchEvent(new Event('visibilitychange'));
    window.dispatchEvent(new Event(visible ? 'focus' : 'blur'));
  }, visible);
}

for (const kind of ['room', 'thread'] as const) {
  test(`${kind}: scrolling unread history advances the saved position without reading newer messages`, async ({
    page
  }) => {
    const { roomPage } = await loginAndEnterRoom(page);
    const roomId = new URL(page.url()).pathname.split('/').at(-1)!;
    const root = await roomPage.sendMessage('History read boundary');
    const rootId = (await root.getEventId())!;
    await waitForReadMarkerViaConnect(page, roomId, rootId);
    const threadId = kind === 'thread' ? rootId : undefined;
    await page.goto(routes.serverOverview);
    const ids: string[] = [];
    for (let i = 0; i < 35; i++) {
      const body = `Unread history ${i}\n\n${'Visible history content. '.repeat(20)}`;
      ids.push(
        threadId
          ? await postThreadReplyViaConnect(page, roomId, body, threadId)
          : await postMessageViaConnect(page, roomId, body)
      );
    }
    // A permalink opens older history without first reading the latest message.
    await page.goto(routes.messageLink(roomId, ids[3]));
    const pane = threadId ? roomPage.threadPane : page;
    const scroller = pane.getByTestId('messages-container').first();
    await expect(pane.locator(`[data-event-id="${ids[3]}"]`)).toBeInViewport();
    await expect(pane.getByTestId('jump-to-present')).toBeVisible();
    await expect
      .poll(async () => ids.indexOf((await getReadMarkerViaConnect(page, roomId, threadId)) ?? ''))
      .toBeGreaterThanOrEqual(3);
    const before = ids.indexOf((await getReadMarkerViaConnect(page, roomId, threadId))!);
    expect(before).toBeLessThan(ids.length - 1);
    // Record actual intersections during scrolling. Layout corrections can move
    // a row out of view again, but that must not move the saved position back.
    await scroller.evaluate((element) => {
      const state = { seen: new Set<string>(), frame: 0 };
      const sample = () => {
        const viewport = element.getBoundingClientRect();
        for (const row of element.querySelectorAll<HTMLElement>('[data-event-id]')) {
          const bounds = row.getBoundingClientRect();
          if (bounds.height > 0 && bounds.bottom > viewport.top && bounds.top < viewport.bottom)
            state.seen.add(row.dataset.eventId!);
        }
        state.frame = requestAnimationFrame(sample);
      };
      Object.assign(element, { readTestVisibility: state });
      sample();
    });
    await scroller.hover();
    await page.mouse.wheel(0, 850);
    await expect
      .poll(async () => ids.indexOf((await getReadMarkerViaConnect(page, roomId, threadId)) ?? ''))
      .toBeGreaterThan(before);
    const after = (await getReadMarkerViaConnect(page, roomId, threadId))!;
    expect(ids.indexOf(after)).toBeLessThan(ids.length - 1);
    const seen = await scroller.evaluate((element) => {
      const state = (
        element as HTMLElement & { readTestVisibility: { seen: Set<string>; frame: number } }
      ).readTestVisibility;
      cancelAnimationFrame(state.frame);
      return [...state.seen];
    });
    expect(seen).toContain(after);
    expect(seen).not.toContain(ids.at(-1));
    await pane.getByTestId('jump-to-present').evaluate((button: HTMLElement) => button.click());
    await waitForReadMarkerViaConnect(page, roomId, ids.at(-1)!, threadId);
    if (threadId) expect(await getReadMarkerViaConnect(page, roomId)).toBe(rootId);
  });

  for (const position of ['hidden', 'history'] as const) {
    test(`${kind}: own arrival from another session stays unread while ${position}`, async ({
      page,
      browser,
      serverURL
    }) => {
      const { roomPage, user } = await loginAndEnterRoom(page);
      const roomId = new URL(page.url()).pathname.split('/').at(-1)!;
      const root = await roomPage.sendMessage('Read-state root');
      const rootId = (await root.getEventId())!;
      const threadId = kind === 'thread' ? rootId : undefined;
      if (threadId) await root.openThread();
      const post = (sender: Page, body: string) =>
        threadId
          ? postThreadReplyViaConnect(sender, roomId, body, threadId)
          : postMessageViaConnect(sender, roomId, body);
      // Long messages make history genuinely scrollable without a large fixture.
      let anchor = rootId;
      for (let i = 0; i < 25; i++)
        anchor = await post(page, `History ${i}\n\n${'A long history message. '.repeat(15)}`);
      await waitForReadMarkerViaConnect(page, roomId, anchor, threadId);
      const pane = threadId ? roomPage.threadPane : page;
      const scroller = pane.getByTestId('messages-container').first();

      await withLoggedInServerWindow(browser, serverURL, user, async ({ page: sender }) => {
        // The other session stays on Overview: sending through the API cannot read the target.
        await sender.goto(routes.serverOverview);
        await page.bringToFront();
        await setVisible(page, true);
        if (position === 'hidden') await setVisible(page, false);
        else {
          await scroller.hover();
          await page.mouse.wheel(0, -1800);
          await expect(pane.getByTestId('jump-to-present')).toBeVisible();
        }
        const reads: string[] = [];
        const record = (request: import('@playwright/test').Request) => {
          if (/\/Mark(Room|Thread)AsRead$/.test(request.url())) reads.push(request.url());
        };
        page.on('request', record);
        try {
          const hydrated = page.waitForResponse(
            async (response) =>
              response.url().endsWith('/BatchGetMessages') &&
              response.ok() &&
              (await response.text()).includes('Own message from the other session')
          );
          const arrival = await post(sender, 'Own message from the other session');
          // An offscreen row is not in the virtual DOM. Its hydration response
          // proves realtime delivery without scrolling it into view.
          await hydrated;
          await page.waitForLoadState('networkidle');
          expect(await getReadMarkerViaConnect(page, roomId, threadId)).toBe(anchor);
          expect(reads).toEqual([]);
          // The jump-to-present button also says "New messages". Check the
          // timeline separator itself, not that button's label.
          await expect(pane.getByTestId('unread-separator')).toHaveCount(0);
          if (position === 'hidden') await setVisible(page, true);
          else
            await pane
              .getByTestId('jump-to-present')
              .evaluate((button: HTMLElement) => button.click());
          await waitForReadMarkerViaConnect(page, roomId, arrival, threadId);
          // A thread read must leave the parent room cursor at its root.
          if (threadId) expect(await getReadMarkerViaConnect(page, roomId)).toBe(rootId);
        } finally {
          page.off('request', record);
        }
      });
    });
  }

  test(`${kind}: a successful send survives failed reads and retries without resending`, async ({
    page
  }) => {
    const { roomPage } = await loginAndEnterRoom(page);
    const roomId = new URL(page.url()).pathname.split('/').at(-1)!;
    const root = await roomPage.sendMessage('Before failed read');
    const rootId = (await root.getEventId())!;
    const threadId = kind === 'thread' ? rootId : undefined;
    if (threadId) await root.openThread();
    await waitForReadMarkerViaConnect(page, roomId, rootId, threadId);
    const routePattern = `**/chatto.api.v1.${threadId ? 'ThreadService/MarkThreadAsRead' : 'RoomService/MarkRoomAsRead'}`;
    let allowRead = false;
    let failedReads = 0;
    let sends = 0;
    page.on('request', (request) => {
      if (request.url().endsWith('/CreateMessage')) sends++;
    });
    await page.route(routePattern, async (route) => {
      if (allowRead) await route.continue();
      else {
        failedReads++;
        await route.abort('internetdisconnected');
      }
    });
    try {
      if (threadId) await roomPage.postThreadReply('Send succeeds while read fails');
      else await roomPage.sendMessage('Send succeeds while read fails');
      const message = threadId
        ? roomPage.getThreadMessage('Send succeeds while read fails')
        : roomPage.getMessage('Send succeeds while read fails');
      await expect(message.locator).toBeVisible();
      const sentId = (await message.getEventId())!;
      await expect.poll(() => failedReads).toBeGreaterThan(0);
      expect(await getReadMarkerViaConnect(page, roomId, threadId)).toBe(rootId);
      allowRead = true;
      // Let the hook's backoff retry run without another send or focus event.
      await waitForReadMarkerViaConnect(page, roomId, sentId, threadId);
      expect(sends).toBe(1);
    } finally {
      await page.unrouteAll({ behavior: 'wait' });
    }
  });
}
