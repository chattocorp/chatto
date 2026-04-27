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
 * Creates a space on a remote server using a bearer token.
 */
export async function createSpaceOnRemote(
	remoteBaseURL: string,
	token: string,
	spaceName: string
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
				mutation CreateSpace($input: CreateSpaceInput!) {
					createSpace(input: $input) { id name }
				}
			`,
			variables: { input: { name: spaceName } }
		})
	});

	if (!response.ok) {
		throw new Error(`Failed to create space on remote: ${await response.text()}`);
	}

	const data = await response.json();
	const spaceId = data.data?.createSpace?.id;
	if (!spaceId) {
		throw new Error(`No spaceId returned: ${JSON.stringify(data)}`);
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
 * Gets the instance name from a remote server.
 */
async function getInstanceName(remoteBaseURL: string): Promise<string> {
	const response = await fetch(`${remoteBaseURL}/api/instance`);
	if (!response.ok) {
		return 'Remote Instance';
	}
	const data = await response.json();
	return data.name ?? 'Remote Instance';
}

/**
 * Injects a remote instance into the browser's localStorage so the
 * frontend treats it as a connected instance. After injection, the page
 * must be reloaded for the instance registry to pick it up.
 */
export async function injectRemoteInstance(
	page: Page,
	remoteServer: ServerInfo,
	token: string,
	userId: string,
	nameOverride?: string
): Promise<string> {
	const instanceName = nameOverride ?? (await getInstanceName(remoteServer.baseURL));

	const instanceId = await page.evaluate(
		({ url, token, userId, name }) => {
			const STORAGE_KEY = 'chatto:instances';
			const instances = JSON.parse(localStorage.getItem(STORAGE_KEY) || '[]');
			const existingIds = instances.map((i: { id: string }) => i.id);

			// Generate ID from hostname (mirrors generateInstanceId in instanceRegistry)
			let hostname: string;
			try {
				hostname = new URL(url).hostname;
			} catch {
				hostname = url.replace(/[^a-z0-9-]/gi, '-');
			}
			let id = hostname.replace(/\./g, '-').replace(/^-+|-+$/g, '');

			// Handle duplicate IDs (e.g., multiple localhost instances)
			if (existingIds.includes(id)) {
				let suffix = 2;
				while (existingIds.includes(`${id}-${suffix}`)) {
					suffix++;
				}
				id = `${id}-${suffix}`;
			}

			instances.push({
				id,
				url,
				name,
				iconUrl: null,
				token,
				userId,
				addedAt: Date.now()
			});

			localStorage.setItem(STORAGE_KEY, JSON.stringify(instances));
			return id;
		},
		{ url: remoteServer.baseURL, token, userId, name: instanceName }
	);

	return instanceId;
}
