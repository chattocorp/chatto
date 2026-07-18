import { afterEach, beforeEach, describe, expect, it, vi, type Mock } from 'vitest';
import { APIPresenceStatus } from '$lib/api-client/presence';
import { PresenceStatus } from '$lib/render/types';
import { presencePreference } from '$lib/state/presencePreference.svelte';
import { __presenceTrackingTest, initPresenceTracking, setPresenceMode } from './presenceTracking';

type UpdatePresence = (
	status: APIPresenceStatus,
	userSelected?: boolean
) => Promise<APIPresenceStatus>;
type PresenceStatusHandler = (status: PresenceStatus) => void;

const mocks = vi.hoisted(() => ({
	updatePresence: vi.fn()
}));

vi.mock('$lib/api-client/presence', async (importOriginal) => {
	const actual = await importOriginal<typeof import('$lib/api-client/presence')>();
	return {
		...actual,
		createPresenceAPI: () => ({
			updatePresence: mocks.updatePresence
		})
	};
});

let documentTarget: EventTarget;
let windowTarget: EventTarget;
let visibilityState: DocumentVisibilityState;
let cleanup: (() => void) | null;
let onStatusChange: Mock<PresenceStatusHandler>;

function dispatchDocumentEvent(type: string) {
	documentTarget.dispatchEvent(new Event(type));
}

function dispatchWindowEvent(type: string) {
	windowTarget.dispatchEvent(new Event(type));
}

function dispatchStorageMode(mode: string) {
	const event = new Event('storage') as StorageEvent;
	Object.defineProperties(event, {
		key: { value: __presenceTrackingTest.PRESENCE_MODE_STORAGE_KEY },
		newValue: { value: mode }
	});
	windowTarget.dispatchEvent(event);
}

function setVisibility(next: DocumentVisibilityState) {
	visibilityState = next;
	dispatchDocumentEvent('visibilitychange');
}

function startTracking() {
	onStatusChange = vi.fn<PresenceStatusHandler>();
	cleanup = initPresenceTracking(
		() => [{ serverId: 'origin', baseUrl: 'https://chat.example.test/api/connect', bearerToken: 't' }],
		onStatusChange
	);
}

function sentStatuses(): APIPresenceStatus[] {
	return mocks.updatePresence.mock.calls.map((call) => call[0]);
}

function sentUserSelectedFlags(): Array<boolean | undefined> {
	return mocks.updatePresence.mock.calls.map((call) => call[1]);
}

describe('initPresenceTracking', () => {
	beforeEach(() => {
		vi.useFakeTimers({ now: 0 });
		mocks.updatePresence = vi.fn<UpdatePresence>((status) => Promise.resolve(status));
		documentTarget = new EventTarget();
		windowTarget = new EventTarget();
		visibilityState = 'visible';
		cleanup = null;

		const storage = new Map<string, string>();
		vi.stubGlobal('localStorage', {
			getItem: vi.fn((key: string) => storage.get(key) ?? null),
			setItem: vi.fn((key: string, value: string) => {
				storage.set(key, value);
			}),
			removeItem: vi.fn((key: string) => {
				storage.delete(key);
			})
		});
		vi.stubGlobal('document', {
			addEventListener: documentTarget.addEventListener.bind(documentTarget),
			removeEventListener: documentTarget.removeEventListener.bind(documentTarget),
			dispatchEvent: documentTarget.dispatchEvent.bind(documentTarget),
			get visibilityState() {
				return visibilityState;
			}
		});
		vi.stubGlobal('window', {
			addEventListener: windowTarget.addEventListener.bind(windowTarget),
			removeEventListener: windowTarget.removeEventListener.bind(windowTarget),
			dispatchEvent: windowTarget.dispatchEvent.bind(windowTarget)
		});
	});

	afterEach(() => {
		cleanup?.();
		vi.unstubAllGlobals();
		vi.useRealTimers();
	});

	it('reports online immediately and does not report away while activity continues', () => {
		startTracking();

		expect(sentStatuses()).toEqual([APIPresenceStatus.ONLINE]);

		vi.advanceTimersByTime(4 * 60 * 1000 + 59 * 1000);
		dispatchDocumentEvent('pointermove');
		vi.advanceTimersByTime(4 * 60 * 1000 + 59 * 1000);

		expect(sentStatuses()).not.toContain(APIPresenceStatus.AWAY);
		expect(onStatusChange).not.toHaveBeenCalledWith(PresenceStatus.Away);
	});

	it('reconciles local status to the server-accepted presence', async () => {
		mocks.updatePresence.mockImplementation((status, userSelected) =>
			Promise.resolve(
				status === APIPresenceStatus.ONLINE && !userSelected
					? APIPresenceStatus.DO_NOT_DISTURB
					: status
			)
		);

		startTracking();

		expect(sentStatuses()).toEqual([APIPresenceStatus.ONLINE]);
		expect(onStatusChange).toHaveBeenLastCalledWith(PresenceStatus.Online);

		await Promise.resolve();

		expect(onStatusChange).toHaveBeenLastCalledWith(PresenceStatus.DoNotDisturb);
		expect(presencePreference.effectiveStatus).toBe(PresenceStatus.DoNotDisturb);

		vi.advanceTimersByTime(30_000);

		expect(sentStatuses()).toEqual([
			APIPresenceStatus.ONLINE,
			APIPresenceStatus.DO_NOT_DISTURB
		]);
		expect(sentUserSelectedFlags()).toEqual([false, false]);
	});

	it('returns online when broad activity resumes after idle', () => {
		startTracking();

		vi.advanceTimersByTime(5 * 60 * 1000);
		expect(sentStatuses().at(-1)).toBe(APIPresenceStatus.AWAY);
		expect(onStatusChange).toHaveBeenLastCalledWith(PresenceStatus.Away);

		dispatchDocumentEvent('pointermove');

		expect(sentStatuses().at(-1)).toBe(APIPresenceStatus.ONLINE);
		expect(onStatusChange).toHaveBeenLastCalledWith(PresenceStatus.Online);
	});

	it('reports away after the hidden delay and returns online when visible again in auto mode', () => {
		startTracking();

		setVisibility('hidden');
		vi.advanceTimersByTime(9_999);
		expect(sentStatuses()).toEqual([APIPresenceStatus.ONLINE]);

		vi.advanceTimersByTime(1);
		expect(sentStatuses()).toEqual([APIPresenceStatus.ONLINE, APIPresenceStatus.AWAY]);
		expect(onStatusChange).toHaveBeenLastCalledWith(PresenceStatus.Away);

		setVisibility('visible');

		expect(sentStatuses()).toEqual([
			APIPresenceStatus.ONLINE,
			APIPresenceStatus.AWAY,
			APIPresenceStatus.ONLINE
		]);
		expect(onStatusChange).toHaveBeenLastCalledWith(PresenceStatus.Online);
	});

	it('does not auto-return from explicit away on activity', () => {
		startTracking();
		setPresenceMode('away');

		dispatchDocumentEvent('pointermove');
		dispatchWindowEvent('focus');
		vi.advanceTimersByTime(5 * 60 * 1000);

		expect(sentStatuses()).toContain(APIPresenceStatus.AWAY);
		expect(sentStatuses().slice(1)).not.toContain(APIPresenceStatus.ONLINE);
		expect(sentUserSelectedFlags().at(1)).toBe(true);
		expect(onStatusChange).toHaveBeenLastCalledWith(PresenceStatus.Away);
	});

	it('returns online when another tab clears explicit away while this tab is hidden', () => {
		startTracking();
		setVisibility('hidden');
		vi.advanceTimersByTime(10_000);
		dispatchStorageMode('away');

		expect(sentStatuses().at(-1)).toBe(APIPresenceStatus.AWAY);
		expect(onStatusChange).toHaveBeenLastCalledWith(PresenceStatus.Away);

		dispatchStorageMode('auto');

		expect(sentStatuses().at(-1)).toBe(APIPresenceStatus.ONLINE);
		expect(sentUserSelectedFlags().at(-1)).toBe(true);
		expect(onStatusChange).toHaveBeenLastCalledWith(PresenceStatus.Online);
		expect(presencePreference.effectiveStatus).toBe(PresenceStatus.Online);
	});

	it('keeps do not disturb through activity and refreshes it', () => {
		startTracking();
		setPresenceMode('doNotDisturb');

		dispatchDocumentEvent('pointermove');
		vi.advanceTimersByTime(30_000);

		expect(sentStatuses()).toEqual([
			APIPresenceStatus.ONLINE,
			APIPresenceStatus.DO_NOT_DISTURB,
			APIPresenceStatus.DO_NOT_DISTURB
		]);
		expect(sentUserSelectedFlags()).toEqual([false, true, true]);
		expect(onStatusChange).toHaveBeenLastCalledWith(PresenceStatus.DoNotDisturb);
	});

	it('does not update presence while invisible and returns online when automatic mode resumes', () => {
		startTracking();
		setPresenceMode('invisible');
		vi.advanceTimersByTime(60_000);
		dispatchDocumentEvent('pointermove');

		expect(sentStatuses()).toEqual([APIPresenceStatus.ONLINE]);
		expect(onStatusChange).toHaveBeenLastCalledWith(PresenceStatus.Offline);

		setPresenceMode('auto');

		expect(sentStatuses()).toEqual([APIPresenceStatus.ONLINE, APIPresenceStatus.ONLINE]);
		expect(sentUserSelectedFlags()).toEqual([false, true]);
	});

	it('starts without reporting presence when look offline was persisted', () => {
		localStorage.setItem(__presenceTrackingTest.PRESENCE_MODE_STORAGE_KEY, 'invisible');

		startTracking();
		vi.advanceTimersByTime(60_000);

		expect(sentStatuses()).toEqual([]);
		expect(onStatusChange).toHaveBeenLastCalledWith(PresenceStatus.Offline);
	});
});
