import { afterEach, beforeEach, describe, expect, it, vi, type Mock } from 'vitest';
import { APIPresenceStatus } from '$lib/api-client/presence';
import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
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

let windowTarget: EventTarget;
let cleanup: (() => void) | null;
let onStatusChange: Mock<PresenceStatusHandler>;

function dispatchStorageMode(mode: string) {
	const event = new Event('storage') as StorageEvent;
	Object.defineProperties(event, {
		key: { value: __presenceTrackingTest.PRESENCE_MODE_STORAGE_KEY },
		newValue: { value: mode }
	});
	windowTarget.dispatchEvent(event);
}

function startTracking() {
	onStatusChange = vi.fn<PresenceStatusHandler>();
	cleanup = initPresenceTracking(
		() => [{ updatePresence: mocks.updatePresence }],
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
		windowTarget = new EventTarget();
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

	it('reports online as the default explicit status', () => {
		startTracking();

		expect(sentStatuses()).toEqual([APIPresenceStatus.ONLINE]);
		expect(sentUserSelectedFlags()).toEqual([true]);
	});

	it('reports away only when the user explicitly selects it and never returns automatically', () => {
		startTracking();

		setPresenceMode('away');

		expect(sentStatuses().at(-1)).toBe(APIPresenceStatus.AWAY);
		expect(onStatusChange).toHaveBeenLastCalledWith(PresenceStatus.AWAY);

		vi.advanceTimersByTime(10 * 60 * 1000);

		// The idle/hidden detectors are gone; only the 30s refresh keeps away alive.
		const statuses = sentStatuses();
		for (const status of statuses.slice(1)) {
			expect(status).toBe(APIPresenceStatus.AWAY);
		}
		// The initial online report plus the explicit away transition; activity
		// never returns the status to Online.
		expect(onStatusChange.mock.calls.flat().filter((s) => s === PresenceStatus.ONLINE)).toHaveLength(
			1
		);
	});

	it('normalizes a legacy stored auto mode to explicit online', () => {
		localStorage.setItem(__presenceTrackingTest.PRESENCE_MODE_STORAGE_KEY, 'auto');

		startTracking();

		expect(presencePreference.mode).toBe('online');
		expect(localStorage.getItem(__presenceTrackingTest.PRESENCE_MODE_STORAGE_KEY)).toBe('online');
		expect(sentStatuses()).toEqual([APIPresenceStatus.ONLINE]);
	});

	it('refreshes the chosen status so server-side presence TTLs do not expire', () => {
		startTracking();
		setPresenceMode('doNotDisturb');

		vi.advanceTimersByTime(30_000);

		expect(sentStatuses()).toEqual([
			APIPresenceStatus.ONLINE,
			APIPresenceStatus.DO_NOT_DISTURB,
			APIPresenceStatus.DO_NOT_DISTURB
		]);
		expect(sentUserSelectedFlags()).toEqual([true, true, true]);
	});

	it('reconciles local status to the server-accepted presence', async () => {
		mocks.updatePresence.mockImplementationOnce(() =>
			Promise.resolve(APIPresenceStatus.DO_NOT_DISTURB)
		);

		startTracking();

		expect(onStatusChange).toHaveBeenLastCalledWith(PresenceStatus.ONLINE);

		await Promise.resolve();

		expect(onStatusChange).toHaveBeenLastCalledWith(PresenceStatus.DO_NOT_DISTURB);
		expect(presencePreference.effectiveStatus).toBe(PresenceStatus.DO_NOT_DISTURB);

		vi.advanceTimersByTime(30_000);

		expect(sentStatuses()).toEqual([
			APIPresenceStatus.ONLINE,
			APIPresenceStatus.DO_NOT_DISTURB
		]);
	});

	it('does not update presence while invisible and resumes when online is selected again', () => {
		startTracking();
		setPresenceMode('invisible');
		vi.advanceTimersByTime(60_000);

		expect(sentStatuses()).toEqual([APIPresenceStatus.ONLINE]);
		expect(onStatusChange).toHaveBeenLastCalledWith(PresenceStatus.OFFLINE);

		setPresenceMode('online');

		expect(sentStatuses()).toEqual([APIPresenceStatus.ONLINE, APIPresenceStatus.ONLINE]);
		expect(sentUserSelectedFlags()).toEqual([true, true]);
	});

	it('starts without reporting presence when look offline was persisted', () => {
		localStorage.setItem(__presenceTrackingTest.PRESENCE_MODE_STORAGE_KEY, 'invisible');

		startTracking();
		vi.advanceTimersByTime(60_000);

		expect(sentStatuses()).toEqual([]);
		expect(onStatusChange).toHaveBeenLastCalledWith(PresenceStatus.OFFLINE);
	});

	it('follows another tab switching between explicit modes without rewriting shared state', () => {
		startTracking();

		dispatchStorageMode('away');

		expect(sentStatuses().at(-1)).toBe(APIPresenceStatus.AWAY);
		expect(presencePreference.mode).toBe('away');

		dispatchStorageMode('online');

		expect(sentStatuses().at(-1)).toBe(APIPresenceStatus.ONLINE);
		expect(sentUserSelectedFlags().at(-1)).toBe(true);
		expect(onStatusChange).toHaveBeenLastCalledWith(PresenceStatus.ONLINE);
		expect(presencePreference.effectiveStatus).toBe(PresenceStatus.ONLINE);

		// Applying another tab's mode must not rewrite the shared storage value;
		// identical writes can surface as spurious storage events in other tabs.
		const localSetItem = vi.mocked(localStorage.setItem);
		const modeWrites = localSetItem.mock.calls.filter(
			([key]) => key === __presenceTrackingTest.PRESENCE_MODE_STORAGE_KEY
		);
		expect(modeWrites).toHaveLength(0);
	});
});
