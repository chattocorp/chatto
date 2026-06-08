import type { Page } from '@playwright/test';
import type { TestInfo } from '@playwright/test';
import { startServer, stopServer, type ServerInfo } from './server';

/**
 * Starts a second Chatto server for multi-instance tests.
 * Uses parallelIndex + 5 to avoid port collisions with the primary server.
 */
export async function startSecondServer(testInfo: TestInfo): Promise<ServerInfo> {
	// Create a modified testInfo-like object with offset parallelIndex
	// to get a different port range from the primary server
	const modifiedTestInfo = {
		...testInfo,
		parallelIndex: testInfo.parallelIndex + 5
	} as TestInfo;

	return startServer(modifiedTestInfo);
}

/**
 * Stops a second server and cleans up.
 */
export async function stopSecondServer(server: ServerInfo, testInfo: TestInfo): Promise<void> {
	const modifiedTestInfo = {
		...testInfo,
		parallelIndex: testInfo.parallelIndex + 5
	} as TestInfo;

	await stopServer(server, modifiedTestInfo);
}

/**
 * Creates a user on a remote server and returns the auth token.
 * This simulates what AddInstanceModal does: register, then login to get a bearer token.
 */
export async function createUserOnRemote(
	remoteBaseURL: string,
	login: string,
	password: string
): Promise<{ token: string; userId: string }> {
	// Create user via the test-only endpoint (build-tagged; not in production
	// binaries). The production createUser GraphQL mutation was removed for
	// security — see #175 — so e2e tests use this build-gated path instead.
	const createResponse = await fetch(`${remoteBaseURL}/auth/test/create-user`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({
			login,
			displayName: `User ${login}`,
			password
		})
	});

	if (!createResponse.ok) {
		throw new Error(`Failed to create user on remote: ${await createResponse.text()}`);
	}

	const createData = await createResponse.json();
	const userId = createData.id;
	if (!userId) {
		throw new Error(`No userId returned from remote test/create-user: ${JSON.stringify(createData)}`);
	}

	// Login to get bearer token
	const loginResponse = await fetch(`${remoteBaseURL}/auth/login`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ login, password })
	});

	if (!loginResponse.ok) {
		throw new Error(`Failed to login on remote: ${await loginResponse.text()}`);
	}

	const loginData = await loginResponse.json();
	if (!loginData.token) {
		throw new Error(`No token returned from remote login: ${JSON.stringify(loginData)}`);
	}

	// Auto-join the bootstrap default rooms (announcements + general) on the
	// remote — the auto-join feature was retired alongside `joinSpace`, so a
	// freshly-created remote user starts out with an empty sidebar. Most
	// cross-server tests assume `# general` is in scope (e.g. typing
	// indicators, editing own messages), so do that join here once.
	await joinSpaceOnRemote(remoteBaseURL, loginData.token);

	return { token: loginData.token, userId };
}

/**
 * Returns the remote server's primary space ID — the bootstrap space every
 * remote server seeds at startup. Issue #330 / ADR-027: createSpace is gone,
 * so multi-instance tests just reuse the bootstrap primary instead of minting
 * a fresh space per test. The `_spaceName` arg is ignored for backwards
 * compatibility with existing call sites.
 */
export async function createSpaceOnRemote(
	remoteBaseURL: string,
	token: string,
	_spaceName: string
): Promise<string> {
	// Sanity-check that the remote is reachable; the actual ID is the
	// kind discriminator constant (post-ADR-030).
	const response = await fetch(`${remoteBaseURL}/api/graphql`, {
		method: 'POST',
		headers: {
			'Content-Type': 'application/json',
			'X-REQUEST-TYPE': 'GraphQL',
			Authorization: `Bearer ${token}`
		},
		body: JSON.stringify({
			query: `query { server { profile { name } } }`
		})
	});

	if (!response.ok) {
		throw new Error(`Failed to reach remote server: ${await response.text()}`);
	}
	return 'server';
}

/**
 * Vestigial fixture kept for source-compat: post-#330 PR(a) `joinSpace` is
 * gone from the API — every authenticated user is implicitly a member of the
 * deployment's server space. We now also auto-join the bootstrap default
 * rooms (announcements + general) on this user so cross-server tests that
 * land directly in `# general` find a real membership instead of an
 * empty-sidebar guest view (auto-join was retired alongside `joinSpace`).
 */
export async function joinSpaceOnRemote(
	remoteBaseURL: string,
	token: string,
	_spaceId?: string
): Promise<void> {
	const roomsResp = await fetch(`${remoteBaseURL}/api/graphql`, {
		method: 'POST',
		headers: {
			'Content-Type': 'application/json',
			'X-REQUEST-TYPE': 'GraphQL',
			Authorization: `Bearer ${token}`
		},
		body: JSON.stringify({ query: `query { server { rooms(type: CHANNEL) { id name } } }` })
	});
	if (!roomsResp.ok) return;
	const roomsData = (await roomsResp.json()) as {
		data?: { server?: { rooms?: Array<{ id: string; name: string }> } };
	};
	const defaults = new Set(['general', 'announcements']);
	const targets = (roomsData.data?.server?.rooms ?? []).filter((r) => defaults.has(r.name));
	for (const room of targets) {
		await fetch(`${remoteBaseURL}/api/graphql`, {
			method: 'POST',
			headers: {
				'Content-Type': 'application/json',
				'X-REQUEST-TYPE': 'GraphQL',
				Authorization: `Bearer ${token}`
			},
			body: JSON.stringify({
				query: `mutation($input: JoinRoomInput!) { joinRoom(input: $input) { id } }`,
				variables: { input: { roomId: room.id } }
			})
		});
	}
}

/**
 * Posts a message in a room on a remote server. Returns the new event ID.
 */
export async function postMessageOnRemote(
	remoteBaseURL: string,
	token: string,
	roomId: string,
	body: string
): Promise<string> {
	const response = await fetch(`${remoteBaseURL}/api/graphql`, {
		method: 'POST',
		headers: {
			'Content-Type': 'application/json',
			'X-REQUEST-TYPE': 'GraphQL',
			Authorization: `Bearer ${token}`
		},
		body: JSON.stringify({
			query: `mutation($input: PostMessageInput!) { postMessage(input: $input) { id } }`,
			variables: { input: { roomId, body } }
		})
	});

	if (!response.ok) {
		throw new Error(`Failed to post message on remote: ${await response.text()}`);
	}

	const data = await response.json();
	const id = data.data?.postMessage?.id;
	if (!id) {
		throw new Error(`No event ID returned from remote postMessage: ${JSON.stringify(data)}`);
	}
	return id;
}

/**
 * Starts a DM conversation on a remote server and posts an initial message.
 * Returns the conversation (room) ID.
 */
export async function startDMOnRemote(
	remoteBaseURL: string,
	senderToken: string,
	receiverUserId: string,
	message: string
): Promise<string> {
	const startResp = await fetch(`${remoteBaseURL}/api/graphql`, {
		method: 'POST',
		headers: {
			'Content-Type': 'application/json',
			'X-REQUEST-TYPE': 'GraphQL',
			Authorization: `Bearer ${senderToken}`
		},
		body: JSON.stringify({
			query: `mutation StartDM($input: StartDMInput!) { startDM(input: $input) { id } }`,
			variables: { input: { participantIds: [receiverUserId] } }
		})
	});
	const startData = await startResp.json();
	const roomId = startData.data?.startDM?.id;
	if (!roomId) throw new Error(`Failed to start DM on remote: ${JSON.stringify(startData)}`);

	await postMessageOnRemote(remoteBaseURL, senderToken, 'DM', roomId, message);
	return roomId;
}

/**
 * Sends a typing indicator on a remote server via GraphQL mutation.
 */
export async function sendTypingOnRemote(
	remoteBaseURL: string,
	token: string,
	roomId: string
): Promise<void> {
	const response = await fetch(`${remoteBaseURL}/api/graphql`, {
		method: 'POST',
		headers: {
			'Content-Type': 'application/json',
			'X-REQUEST-TYPE': 'GraphQL',
			Authorization: `Bearer ${token}`
		},
		body: JSON.stringify({
			query: `
				mutation SendTypingIndicator($input: SendTypingIndicatorInput!) {
					sendTypingIndicator(input: $input)
				}
			`,
			variables: { input: { roomId } }
		})
	});

	if (!response.ok) {
		throw new Error(`Failed to send typing on remote: ${await response.text()}`);
	}
}

/**
 * Gets a room by name on a remote server. Returns the room's ID.
 */
export async function getRoomOnRemote(
	remoteBaseURL: string,
	token: string,
	roomName: string
): Promise<string> {
	const response = await fetch(`${remoteBaseURL}/api/graphql`, {
		method: 'POST',
		headers: {
			'Content-Type': 'application/json',
			'X-REQUEST-TYPE': 'GraphQL',
			Authorization: `Bearer ${token}`
		},
		body: JSON.stringify({
			query: `
				query InstanceRooms {
					server {
						rooms(type: CHANNEL) { id name }
					}
				}
			`
		})
	});

	if (!response.ok) {
		throw new Error(`Failed to get rooms on remote: ${await response.text()}`);
	}

	const data = await response.json();
	const rooms = data.data?.server?.rooms;
	if (!rooms) {
		throw new Error(`No rooms returned: ${JSON.stringify(data)}`);
	}

	const room = rooms.find((r: { name: string }) => r.name === roomName);
	if (!room) {
		throw new Error(`Room "${roomName}" not found in instance: ${JSON.stringify(rooms)}`);
	}

	return room.id;
}

/**
 * Logs in as the bootstrap admin user (`e2eadmin`) on a remote server and
 * returns a bearer token. Mirrors `loginAsAdmin()` for the origin server.
 */
export async function loginAdminOnRemote(
	remoteBaseURL: string
): Promise<{ token: string; userId: string }> {
	const loginResp = await fetch(`${remoteBaseURL}/auth/login`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ login: 'e2eadmin', password: 'adminpassword123' })
	});
	if (!loginResp.ok) {
		throw new Error(`Failed to login admin on remote: ${await loginResp.text()}`);
	}
	const loginData = await loginResp.json();
	if (!loginData.token) {
		throw new Error(`No token returned from remote admin login: ${JSON.stringify(loginData)}`);
	}

	const meResp = await fetch(`${remoteBaseURL}/api/graphql`, {
		method: 'POST',
		headers: {
			'Content-Type': 'application/json',
			'X-REQUEST-TYPE': 'GraphQL',
			Authorization: `Bearer ${loginData.token}`
		},
		body: JSON.stringify({ query: `query { viewer { user { id } } }` })
	});
	const meData = await meResp.json();
	const userId = meData.data?.viewer?.user?.id;
	if (!userId) {
		throw new Error(`No userId returned from remote viewer query: ${JSON.stringify(meData)}`);
	}
	return { token: loginData.token, userId };
}

/**
 * Updates the MOTD on a remote server via the admin GraphQL mutation.
 * The token must belong to a user with admin/owner permission.
 */
export async function setMotdOnRemote(
	remoteBaseURL: string,
	token: string,
	motd: string
): Promise<void> {
	const resp = await fetch(`${remoteBaseURL}/api/graphql`, {
		method: 'POST',
		headers: {
			'Content-Type': 'application/json',
			'X-REQUEST-TYPE': 'GraphQL',
			Authorization: `Bearer ${token}`
		},
		body: JSON.stringify({
			query: `
				mutation SetMotd($input: UpdateServerConfigInput!) {
					updateServerConfig(input: $input) { motd }
				}
			`,
			variables: { input: { motd } }
		})
	});
	if (!resp.ok) {
		throw new Error(`Failed to set MOTD on remote: ${await resp.text()}`);
	}
	const data = await resp.json();
	if (data.errors) {
		throw new Error(`updateServerConfig on remote returned errors: ${JSON.stringify(data.errors)}`);
	}
}

/**
 * Drives the real Add-Server dialog → /oauth/authorize → /servers/callback
 * flow to add `remoteServer` as a connected instance, while bypassing the
 * human OAuth login form. The remote's `/oauth/authorize` request is
 * intercepted via Playwright's `page.route`; we POST the PKCE params to the
 * test-only `/auth/test/oauth-authorize` endpoint to mint a real authorization
 * code, then fulfill the navigation with a 302 to the callback URL. From
 * there the origin's callback page runs unchanged: PKCE verifier exchange via
 * `/oauth/token`, real bearer token, real `serverRegistry.addServer()`.
 *
 * The user identified by `userId` must already exist on the remote (use
 * `createUserOnRemote` to create one).
 */
export async function connectRemoteInstance(
	page: Page,
	remoteServer: ServerInfo,
	userId: string
): Promise<void> {
	const remoteBaseURL = remoteServer.baseURL;
	const remoteOrigin = new URL(remoteBaseURL).origin;
	const hostname = new URL(remoteBaseURL).host;

	// Intercept the navigation to the remote's /oauth/authorize and fulfill
	// with a 302 to the callback URL carrying a real authorization code.
	await page.route(`${remoteOrigin}/oauth/authorize*`, async (route) => {
		const requestUrl = new URL(route.request().url());
		const codeChallenge = requestUrl.searchParams.get('code_challenge') ?? '';
		const codeChallengeMethod =
			requestUrl.searchParams.get('code_challenge_method') ?? '';
		const redirectUri = requestUrl.searchParams.get('redirect_uri') ?? '';
		const state = requestUrl.searchParams.get('state') ?? '';

		const resp = await fetch(`${remoteBaseURL}/auth/test/oauth-authorize`, {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({
				userId,
				redirectUri,
				codeChallenge,
				codeChallengeMethod,
				state
			})
		});

		if (!resp.ok) {
			throw new Error(
				`test/oauth-authorize failed (${resp.status}): ${await resp.text()}`
			);
		}

		const { redirectURL } = (await resp.json()) as { redirectURL: string };
		await route.fulfill({
			status: 302,
			headers: { Location: redirectURL }
		});
	});

	// Drive the real UI: open dialog from sidebar → URL → preview →
	// would-redirect to /oauth/authorize (intercepted) → /servers/callback
	// → token exchange → addServer.
	if (!/\/chat\//.test(page.url())) {
		await page.goto('/chat/-');
	}
	await page.getByTitle('Add Server').click();
	await page.getByLabel('Server URL').fill(hostname);
	await page.getByRole('button', { name: 'Connect' }).click();
	await page.getByRole('button', { name: 'Sign in', exact: true }).click();

	// Callback page redirects into the newly-added remote instance's chat
	// tree on success — `/chat/<hostname>/...` (post-PR(a) there is no
	// `/chat/spaces` landing). The hostname is whatever segment was passed
	// in (typically "127.0.0.1").
	const hostnameOnly = hostname.split(':')[0]!.replace(/\./g, '\\.');
	await page.waitForURL(new RegExp(`/chat/${hostnameOnly}(/|$)`));
}
