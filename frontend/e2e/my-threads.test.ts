import { expect } from '@playwright/test';
import { createAndLoginTestUser, openServer } from './fixtures/testUser';
import { waitForRoomReady } from './fixtures/realtimeSync';
import { test } from './setup';
import { ChatPage, RoomPage, MyThreadsPage } from './pages';
import { TIMEOUTS, POLLING_INTERVALS } from './constants';
import * as routes from './routes';

test.describe('My Threads', () => {
	test('shows empty state when no threads are followed', async ({
		page,
		chatPage
	}) => {
		await createAndLoginTestUser(page);
		await chatPage.goto();

		const spaceId = await chatPage.getServerScopeId();
		const myThreads = new MyThreadsPage(page);

		await myThreads.goto(spaceId);

		// Should show empty state
		await expect(page.getByText('No followed threads')).toBeVisible();
		await expect(
			page.getByText('Threads you follow will appear here')
		).toBeVisible();
	});

	test('followed thread appears in My Threads list', async ({
		page,
		chatPage,
		roomPage
	}) => {
		await createAndLoginTestUser(page);
		await chatPage.goto();
		await chatPage.enterRoom('general');

		const spaceId = await chatPage.getServerScopeId();
		const myThreads = new MyThreadsPage(page);

		// Post a message, open thread, reply (auto-follows)
		const rootText = `Thread root ${Date.now()}`;
		const rootMsg = await roomPage.sendMessage(rootText);
		await rootMsg.openThread();
		await roomPage.expectThreadPaneVisible();
		await roomPage.postThreadReply(`Reply ${Date.now()}`);
		await roomPage.closeThread();

		await myThreads.goto(spaceId);

		// Thread should appear with room name, root message preview, and reply count
		const threadItem = myThreads.threadItems;
		await expect(threadItem).toBeVisible();
		await expect(threadItem.getByText(/in #general:/)).toBeVisible();
		await expect(threadItem.getByText(rootText)).toBeVisible();
		await expect(threadItem.getByText('1 reply')).toBeVisible();
	});

	test('clicking a thread navigates to the room with thread pane open', async ({
		page,
		chatPage,
		roomPage
	}) => {
		await createAndLoginTestUser(page);
		await chatPage.goto();
		await chatPage.enterRoom('general');

		const spaceId = await chatPage.getServerScopeId();
		const myThreads = new MyThreadsPage(page);

		// Create a thread
		const rootText = `Clickable thread ${Date.now()}`;
		const rootMsg = await roomPage.sendMessage(rootText);
		await rootMsg.openThread();
		await roomPage.expectThreadPaneVisible();
		await roomPage.postThreadReply(`Reply ${Date.now()}`);
		await roomPage.closeThread();

		await myThreads.goto(spaceId);

		// Click the thread entry
		await myThreads.threadItems.click();

		// Should navigate to the room with thread pane open
		await expect(page).toHaveURL(routes.patterns.anyThread);
		await roomPage.expectThreadPaneVisible();
	});

	test('unfollowing a thread removes it from the list', async ({
		page,
		chatPage,
		roomPage
	}) => {
		await createAndLoginTestUser(page);
		await chatPage.goto();
		await chatPage.enterRoom('general');

		const spaceId = await chatPage.getServerScopeId();
		const myThreads = new MyThreadsPage(page);

		// Create a thread (auto-followed)
		const rootText = `Unfollow me ${Date.now()}`;
		const rootMsg = await roomPage.sendMessage(rootText);
		await rootMsg.openThread();
		await roomPage.expectThreadPaneVisible();
		await roomPage.postThreadReply(`Reply ${Date.now()}`);
		await roomPage.closeThread();

		// Unfollow the thread
		await rootMsg.toggleThreadFollow();
		await rootMsg.expectNotFollowingThread();

		await myThreads.goto(spaceId);

		// Thread should not appear
		await expect(page.getByText('No followed threads')).toBeVisible();
	});

	test('new reply updates reply count in real-time', async ({
		page,
		chatPage,
		roomPage,
		browser,
		serverURL
	}) => {
		await createAndLoginTestUser(page);
		await chatPage.goto();
		await chatPage.enterRoom('general');

		const spaceId = await chatPage.getServerScopeId();
		const myThreads = new MyThreadsPage(page);

		// User A creates a thread
		const rootText = `Realtime thread ${Date.now()}`;
		const rootMsg = await roomPage.sendMessage(rootText);
		await rootMsg.openThread();
		await roomPage.expectThreadPaneVisible();
		await roomPage.postThreadReply(`Reply from A ${Date.now()}`);
		await roomPage.closeThread();

		await myThreads.goto(spaceId);

		// Verify 1 reply shown
		await expect(page.getByText('1 reply')).toBeVisible();

		// User B joins and replies
		const context2 = await browser!.newContext({ baseURL: serverURL });
		const page2 = await context2.newPage();

		try {
			await createAndLoginTestUser(page2);
			await openServer(page2);
			await page2.goto(routes.space());

			const chatPage2 = new ChatPage(page2);
			const roomPage2 = new RoomPage(page2);

			await chatPage2.enterRoom('general');
			await waitForRoomReady(page2, 'general');

			// User B opens the thread and posts a reply
			const rootMsg2 = roomPage2.getMessage(rootText);
			await rootMsg2.openThread();
			await roomPage2.expectThreadPaneVisible();
			await roomPage2.postThreadReply(`Reply from B ${Date.now()}`);
		} finally {
			await context2.close();
		}

		// User A should see the reply count update
		await expect(page.getByText('2 replies')).toBeVisible({
			timeout: TIMEOUTS.REALTIME_EVENT
		});
	});

	test('unread indicator appears when new replies arrive', async ({
		page,
		chatPage,
		roomPage,
		browser,
		serverURL
	}) => {
		await createAndLoginTestUser(page);
		await chatPage.goto();
		await chatPage.enterRoom('general');

		const spaceId = await chatPage.getServerScopeId();
		const myThreads = new MyThreadsPage(page);

		// User A creates a thread and navigates to My Threads
		const rootText = `Unread thread ${Date.now()}`;
		const rootMsg = await roomPage.sendMessage(rootText);
		await rootMsg.openThread();
		await roomPage.expectThreadPaneVisible();
		await roomPage.postThreadReply(`Reply from A ${Date.now()}`);
		await roomPage.closeThread();

		await myThreads.goto(spaceId);

		// Initially no unread dot (we just replied so it's "read")
		// Use toPass() to allow subscriptions to settle before asserting absence
		const threadItem = myThreads.threadItems;
		const unreadDot = threadItem.locator('.bg-warning');
		await expect(async () => {
			await expect(unreadDot).not.toBeVisible();
		}).toPass({ timeout: TIMEOUTS.UI_STANDARD, intervals: [500, 1000, 2000] });

		// User B joins and replies to the thread
		const context2 = await browser!.newContext({ baseURL: serverURL });
		const page2 = await context2.newPage();

		try {
			await createAndLoginTestUser(page2);
			await openServer(page2);
			await page2.goto(routes.space());

			const chatPage2 = new ChatPage(page2);
			const roomPage2 = new RoomPage(page2);

			await chatPage2.enterRoom('general');
			await waitForRoomReady(page2, 'general');

			const rootMsg2 = roomPage2.getMessage(rootText);
			await rootMsg2.openThread();
			await roomPage2.expectThreadPaneVisible();
			await roomPage2.postThreadReply(`Reply from B ${Date.now()}`);
		} finally {
			await context2.close();
		}

		// User A should see the unread indicator (orange dot inside the reply button)
		await expect(unreadDot).toBeVisible({
			timeout: TIMEOUTS.REALTIME_EVENT
		});
	});

	test('sidebar unread dot appears when another user replies', async ({
		page,
		chatPage,
		roomPage,
		browser,
		serverURL
	}) => {
		await createAndLoginTestUser(page);
		await chatPage.goto();
		await chatPage.enterRoom('general');

		const spaceId = await chatPage.getServerScopeId();
		const myThreads = new MyThreadsPage(page);

		// User A creates a thread
		const rootText = `Sidebar dot thread ${Date.now()}`;
		const rootMsg = await roomPage.sendMessage(rootText);
		await rootMsg.openThread();
		await roomPage.expectThreadPaneVisible();
		await roomPage.postThreadReply(`Reply from A ${Date.now()}`);
		await roomPage.closeThread();

		// Stay on the room page (not My Threads) — sidebar dot should not be visible yet
		// Use toPass() to allow subscriptions to settle before asserting absence
		await expect(async () => {
			await expect(myThreads.sidebarUnreadDot).not.toBeVisible();
		}).toPass({ timeout: TIMEOUTS.UI_STANDARD, intervals: [500, 1000, 2000] });

		// User B joins and replies to the thread
		const context2 = await browser!.newContext({ baseURL: serverURL });
		const page2 = await context2.newPage();

		try {
			await createAndLoginTestUser(page2);
			await openServer(page2);
			await page2.goto(routes.space());

			const chatPage2 = new ChatPage(page2);
			const roomPage2 = new RoomPage(page2);

			await chatPage2.enterRoom('general');
			await waitForRoomReady(page2, 'general');

			const rootMsg2 = roomPage2.getMessage(rootText);
			await rootMsg2.openThread();
			await roomPage2.expectThreadPaneVisible();
			await roomPage2.postThreadReply(`Reply from B ${Date.now()}`);
		} finally {
			await context2.close();
		}

		// Sidebar unread dot should appear (User A is still on room page)
		await expect(myThreads.sidebarUnreadDot).toBeVisible({
			timeout: TIMEOUTS.REALTIME_EVENT
		});

		// Navigate to My Threads — sidebar dot should still be visible (visiting the list
		// doesn't mark threads as read; you need to open each thread individually)
		await myThreads.goto(spaceId);
		await expect(myThreads.sidebarUnreadDot).toBeVisible();
	});

	test('sidebar unread dot clears after opening the thread', async ({
		page,
		chatPage,
		roomPage,
		browser,
		serverURL
	}) => {
		await createAndLoginTestUser(page);
		await chatPage.goto();
		await chatPage.enterRoom('general');

		const spaceId = await chatPage.getServerScopeId();
		const myThreads = new MyThreadsPage(page);

		// User A creates a thread
		const rootText = `Dot clears thread ${Date.now()}`;
		const rootMsg = await roomPage.sendMessage(rootText);
		await rootMsg.openThread();
		await roomPage.expectThreadPaneVisible();
		await roomPage.postThreadReply(`Reply from A ${Date.now()}`);
		await roomPage.closeThread();

		// User B joins and replies to the thread
		const context2 = await browser!.newContext({ baseURL: serverURL });
		const page2 = await context2.newPage();

		try {
			await createAndLoginTestUser(page2);
			await openServer(page2);
			await page2.goto(routes.space());

			const chatPage2 = new ChatPage(page2);
			const roomPage2 = new RoomPage(page2);

			await chatPage2.enterRoom('general');
			await waitForRoomReady(page2, 'general');

			const rootMsg2 = roomPage2.getMessage(rootText);
			await rootMsg2.openThread();
			await roomPage2.expectThreadPaneVisible();
			await roomPage2.postThreadReply(`Reply from B ${Date.now()}`);
		} finally {
			await context2.close();
		}

		// Sidebar unread dot should appear
		await expect(myThreads.sidebarUnreadDot).toBeVisible({
			timeout: TIMEOUTS.REALTIME_EVENT
		});

		// Open the thread that has unread replies
		const rootMsgAgain = roomPage.getMessage(rootText);
		await rootMsgAgain.openThread();
		await roomPage.expectThreadPaneVisible();

		// Sidebar unread dot should clear after opening the thread
		await expect(async () => {
			await expect(myThreads.sidebarUnreadDot).not.toBeVisible();
		}).toPass({ timeout: TIMEOUTS.UI_STANDARD, intervals: POLLING_INTERVALS });
	});

	test('manually following a thread from room view shows it in My Threads', async ({
		page,
		chatPage,
		roomPage,
		browser,
		serverURL
	}) => {
		await createAndLoginTestUser(page);
		await chatPage.goto();
		await chatPage.enterRoom('general');

		const spaceId = await chatPage.getServerScopeId();
		const myThreads = new MyThreadsPage(page);

		// User B creates a thread (so User A doesn't auto-follow)
		const context2 = await browser!.newContext({ baseURL: serverURL });
		const page2 = await context2.newPage();

		const rootText = `Manual follow thread ${Date.now()}`;

		try {
			await createAndLoginTestUser(page2);
			await openServer(page2);
			await page2.goto(routes.space());

			const chatPage2 = new ChatPage(page2);
			const roomPage2 = new RoomPage(page2);

			await chatPage2.enterRoom('general');
			await waitForRoomReady(page2, 'general');

			// User B posts root message and a reply to create a thread
			const rootMsg2 = await roomPage2.sendMessage(rootText);
			await rootMsg2.openThread();
			await roomPage2.expectThreadPaneVisible();
			await roomPage2.postThreadReply(`Reply from B ${Date.now()}`);
			await roomPage2.closeThread();
		} finally {
			await context2.close();
		}

		// Wait for User B's messages to appear for User A
		await expect(page.getByText(rootText)).toBeVisible({
			timeout: TIMEOUTS.REALTIME_EVENT
		});

		// User A manually follows the thread (bell icon on the message)
		const rootMsg = roomPage.getMessage(rootText);
		await rootMsg.expectNotFollowingThread();
		await rootMsg.toggleThreadFollow();
		await rootMsg.expectFollowingThread();

		// Navigate to My Threads — the manually followed thread should appear
		await myThreads.goto(spaceId);

		const threadItem = myThreads.threadItems;
		await expect(threadItem).toBeVisible();
		await expect(threadItem.getByText(rootText)).toBeVisible();
	});

	test('unread filter hides read threads and shows "all caught up" when none are unread', async ({
		page,
		chatPage,
		roomPage
	}) => {
		await createAndLoginTestUser(page);
		await chatPage.goto();
		await chatPage.enterRoom('general');

		const spaceId = await chatPage.getServerScopeId();
		const myThreads = new MyThreadsPage(page);

		// Create a thread (we just replied, so it's "read")
		const rootText = `Read thread ${Date.now()}`;
		const rootMsg = await roomPage.sendMessage(rootText);
		await rootMsg.openThread();
		await roomPage.expectThreadPaneVisible();
		await roomPage.postThreadReply(`Reply ${Date.now()}`);
		await roomPage.closeThread();

		await myThreads.goto(spaceId);

		// Default filter is "All" — thread is visible
		await expect(myThreads.threadItems).toBeVisible();

		// Switch to "Unread" filter
		await page.getByRole('radio', { name: 'Unread' }).click();

		// Thread should be hidden (it's read), show "all caught up"
		await expect(myThreads.threadItems).not.toBeVisible();
		await expect(page.getByText('All caught up')).toBeVisible();

		// Switch back to "All" — thread reappears
		await page.getByRole('radio', { name: 'All' }).click();
		await expect(myThreads.threadItems).toBeVisible();
	});

	test('unread filter shows only threads with unread replies', async ({
		page,
		chatPage,
		roomPage,
		browser,
		serverURL
	}) => {
		await createAndLoginTestUser(page);
		await chatPage.goto();
		await chatPage.enterRoom('general');

		const spaceId = await chatPage.getServerScopeId();
		const myThreads = new MyThreadsPage(page);

		// Create two threads
		const readRoot = `Read thread ${Date.now()}`;
		const readMsg = await roomPage.sendMessage(readRoot);
		await readMsg.openThread();
		await roomPage.expectThreadPaneVisible();
		await roomPage.postThreadReply(`Reply ${Date.now()}`);
		await roomPage.closeThread();

		const unreadRoot = `Unread thread ${Date.now()}`;
		const unreadMsg = await roomPage.sendMessage(unreadRoot);
		await unreadMsg.openThread();
		await roomPage.expectThreadPaneVisible();
		await roomPage.postThreadReply(`Reply ${Date.now()}`);
		await roomPage.closeThread();

		// User B replies to only the second thread (making it unread for User A)
		const context2 = await browser!.newContext({ baseURL: serverURL });
		const page2 = await context2.newPage();

		try {
			await createAndLoginTestUser(page2);
			await openServer(page2);
			await page2.goto(routes.space());

			const chatPage2 = new ChatPage(page2);
			const roomPage2 = new RoomPage(page2);

			await chatPage2.enterRoom('general');
			await waitForRoomReady(page2, 'general');

			const msg2 = roomPage2.getMessage(unreadRoot);
			await msg2.openThread();
			await roomPage2.expectThreadPaneVisible();
			await roomPage2.postThreadReply(`Reply from B ${Date.now()}`);
		} finally {
			await context2.close();
		}

		await myThreads.goto(spaceId);

		// "All" shows both threads
		await expect(myThreads.threadItems).toHaveCount(2, {
			timeout: TIMEOUTS.REALTIME_EVENT
		});

		// Switch to "Unread" — only the unread thread is visible
		await page.getByRole('radio', { name: 'Unread' }).click();
		await expect(myThreads.threadItems).toHaveCount(1);
		await expect(page.getByText(unreadRoot)).toBeVisible();
		await expect(page.getByText(readRoot)).not.toBeVisible();
	});

	test('filter selection persists across back navigation', async ({
		page,
		chatPage,
		roomPage
	}) => {
		await createAndLoginTestUser(page);
		await chatPage.goto();
		await chatPage.enterRoom('general');

		const spaceId = await chatPage.getServerScopeId();
		const myThreads = new MyThreadsPage(page);

		// Create a thread
		const rootText = `Sticky filter thread ${Date.now()}`;
		const rootMsg = await roomPage.sendMessage(rootText);
		await rootMsg.openThread();
		await roomPage.expectThreadPaneVisible();
		await roomPage.postThreadReply(`Reply ${Date.now()}`);
		await roomPage.closeThread();

		await myThreads.goto(spaceId);

		// Switch to "Unread" filter
		await page.getByRole('radio', { name: 'Unread' }).click();
		await expect(page.getByRole('radio', { name: 'Unread' })).toHaveAttribute(
			'aria-checked',
			'true'
		);

		// Click the thread to navigate to it
		// (thread is read, so switch back to All first to see it)
		await page.getByRole('radio', { name: 'All' }).click();
		await myThreads.threadItems.click();
		await expect(page).toHaveURL(routes.patterns.anyThread);

		// Go back — should return to My Threads
		await page.goBack();
		await page.waitForURL(routes.threads);

		// The "All" filter should still be selected (was last set before navigating)
		await expect(page.getByRole('radio', { name: 'All' })).toHaveAttribute(
			'aria-checked',
			'true'
		);
	});

	test('navigating from My Threads thread to a different room does not crash', async ({
		page,
		chatPage,
		roomPage
	}) => {
		await createAndLoginTestUser(page);
		await chatPage.goto();
		await chatPage.enterRoom('general');

		const spaceId = await chatPage.getServerScopeId();
		const myThreads = new MyThreadsPage(page);

		// Create a second room to navigate to
		const secondRoom = `other-${Date.now()}`;
		await chatPage.createRoom(secondRoom);

		// Navigate back to general (createRoom navigates to the new room)
		await chatPage.enterRoom('general');

		// Create a thread in general
		const rootText = `Nav test ${Date.now()}`;
		const rootMsg = await roomPage.sendMessage(rootText);
		await rootMsg.openThread();
		await roomPage.expectThreadPaneVisible();
		await roomPage.postThreadReply(`Reply ${Date.now()}`);
		await roomPage.closeThread();

		// Track page errors (the bug causes TypeError: Cannot read properties of undefined)
		const pageErrors: string[] = [];
		page.on('pageerror', (err) => pageErrors.push(err.message));

		// Navigate to My Threads
		await myThreads.goto(spaceId);
		await expect(myThreads.threadItems).toBeVisible();

		// Click the thread — opens room with thread pane
		await myThreads.threadItems.click();
		await roomPage.expectThreadPaneVisible();

		// Click a different room in the sidebar — this is the trigger for the bug
		await chatPage.enterRoom(secondRoom);

		// Navigate back to the original room to confirm no deadlock
		await chatPage.enterRoom('general');
		// ThreadPane's exit `transition:fly` keeps it in the DOM for ~200ms after
		// the URL drops the thread suffix. Wait for it to actually unmount before
		// asserting rootText is uniquely visible — otherwise the still-fading
		// pane and the main timeline both match (strict-mode violation).
		await expect(page.getByTestId('thread-pane')).toBeHidden();
		await expect(page.getByText(rootText)).toBeVisible({
			timeout: TIMEOUTS.UI_STANDARD
		});

		// No TypeError should have occurred
		const criticalErrors = pageErrors.filter(
			(e) => e.includes('Cannot read properties of undefined')
		);
		expect(criticalErrors).toEqual([]);
	});

	test('multiple followed threads are sorted by last activity', async ({
		page,
		chatPage,
		roomPage
	}) => {
		await createAndLoginTestUser(page);
		await chatPage.goto();
		await chatPage.enterRoom('general');

		const spaceId = await chatPage.getServerScopeId();
		const myThreads = new MyThreadsPage(page);

		// Create first thread
		const root1 = `First thread ${Date.now()}`;
		const msg1 = await roomPage.sendMessage(root1);
		await msg1.openThread();
		await roomPage.expectThreadPaneVisible();
		await roomPage.postThreadReply(`Reply to first ${Date.now()}`);
		await roomPage.closeThread();

		// Create second thread (more recent activity)
		const root2 = `Second thread ${Date.now()}`;
		const msg2 = await roomPage.sendMessage(root2);
		await msg2.openThread();
		await roomPage.expectThreadPaneVisible();
		await roomPage.postThreadReply(`Reply to second ${Date.now()}`);
		await roomPage.closeThread();

		await myThreads.goto(spaceId);

		// Both threads should be visible
		const items = myThreads.threadItems;
		await expect(items).toHaveCount(2);

		// Most recent thread should be first (second thread has more recent activity)
		const firstItem = items.nth(0);
		const secondItem = items.nth(1);
		await expect(firstItem.getByText(root2)).toBeVisible();
		await expect(secondItem.getByText(root1)).toBeVisible();
	});
});
