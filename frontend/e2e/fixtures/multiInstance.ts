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
	const response = await fetch(`${remoteBaseURL}/api/graphql`, {
		method: 'POST',
		headers: {
			'Content-Type': 'application/json',
			'X-REQUEST-TYPE': 'GraphQL',
			Authorization: `Bearer ${token}`
		},
		body: JSON.stringify({
			query: `query { instance { primarySpaceId } }`
		})
	});

	if (!response.ok) {
		throw new Error(`Failed to read primary space on remote: ${await response.text()}`);
	}

	const data = await response.json();
	const spaceId: string = data.data?.instance?.primarySpaceId;
	if (!spaceId) {
		throw new Error(`No primary space configured on remote: ${JSON.stringify(data)}`);
	}
	return spaceId;
}

/**
 * Joins an existing space on a remote server. The user must already exist.
 * Returns the space ID for convenience.
 */
export async function joinSpaceOnRemote(
	remoteBaseURL: string,
	token: string,
	spaceId: string
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
				mutation JoinSpace($input: JoinSpaceInput!) { joinSpace(input: $input)
				}
			`,
			variables: { input: { spaceId } }
		})
	});

	if (!response.ok) {
		throw new Error(`Failed to join space on remote: ${await response.text()}`);
	}
}

/**
 * Posts a message in a room on a remote server. Returns the new event ID.
 */
export async function postMessageOnRemote(
	remoteBaseURL: string,
	token: string,
	spaceId: string,
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
			variables: { input: { spaceId, roomId, body } }
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
	spaceId: string,
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
			variables: { input: { spaceId, roomId } }
		})
	});

	if (!response.ok) {
		throw new Error(`Failed to send typing on remote: ${await response.text()}`);
	}
}

/**
 * Gets the rooms for a space on a remote server. Returns the first room's ID.
 */
export async function getRoomOnRemote(
	remoteBaseURL: string,
	token: string,
	spaceId: string,
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
				query SpaceRooms($spaceId: ID!) {
					space(id: $spaceId) {
						rooms { id name }
					}
				}
			`,
			variables: { spaceId }
		})
	});

	if (!response.ok) {
		throw new Error(`Failed to get rooms on remote: ${await response.text()}`);
	}

	const data = await response.json();
	const rooms = data.data?.space?.rooms;
	if (!rooms) {
		throw new Error(`No rooms returned: ${JSON.stringify(data)}`);
	}

	const room = rooms.find((r: { name: string }) => r.name === roomName);
	if (!room) {
		throw new Error(`Room "${roomName}" not found in space: ${JSON.stringify(rooms)}`);
	}

	return room.id;
}

/**
 * Drives the real Add-Server dialog → /oauth/authorize → /instances/callback
 * flow to add `remoteServer` as a connected instance, while bypassing the
 * human OAuth login form. The remote's `/oauth/authorize` request is
 * intercepted via Playwright's `page.route`; we POST the PKCE params to the
 * test-only `/auth/test/oauth-authorize` endpoint to mint a real authorization
 * code, then fulfill the navigation with a 302 to the callback URL. From
 * there the origin's callback page runs unchanged: PKCE verifier exchange via
 * `/oauth/token`, real bearer token, real `instanceRegistry.addInstance()`.
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

	// Drive the real UI: open dialog → URL → preview → would-redirect to
	// /oauth/authorize (intercepted) → /instances/callback → token exchange →
	// addInstance.
	await page.goto('/instances');
	await openAddServerDialogFromInstancesPage(page);
	await page.getByLabel('Server URL').fill(hostname);
	await page.getByRole('button', { name: 'Connect' }).click();
	await page.getByRole('button', { name: 'Sign in', exact: true }).click();

	// Callback page redirects to /chat/spaces on success.
	await page.waitForURL(/\/chat\/spaces/);
}

/**
 * Click the "Add Server" button in the /instances PaneHeader. The sidebar
 * "+" icon also exposes the accessible name "Add Server" (via title), so we
 * disambiguate by visible text — only the header button has it.
 */
async function openAddServerDialogFromInstancesPage(page: Page): Promise<void> {
	await page
		.getByRole('button', { name: 'Add Server', exact: true })
		.filter({ hasText: 'Add Server' })
		.click();
}
