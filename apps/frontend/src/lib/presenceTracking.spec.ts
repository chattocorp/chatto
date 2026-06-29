import { afterEach, beforeEach, describe, expect, it, vi, type Mock } from 'vitest';
import { APIPresenceStatus } from '@chatto/api-client/presence';
import { PresenceStatus } from '$lib/render/types';
import { presencePreference } from '$lib/state/presencePreference.svelte';
import { initPresenceTracking, setPresenceMode } from './presenceTracking';

type ReportPresence = (
	status: APIPresenceStatus,
	userSelected?: boolean
) => Promise<APIPresenceStatus>;
type PresenceStatusHandler = (status: PresenceStatus) => void;

const mocks = vi.hoisted(() => ({
	reportPresence: vi.fn()
}));

vi.mock('@chatto/api-client/presence', async (importOriginal) => {
	const actual = await importOriginal<typeof import('@chatto/api-client/presence')>();
	return {
		...actual,
		createPresenceAPI: () => ({
			reportPresence: mocks.reportPresence
		})
	};
});

let documentTarget: EventTarget;
let windowTarget: EventTarget;
let visibilityState: DocumentVisibilityState;
let cleanup: (() => void) | null;
let onStatusChange: Mock<PresenceStatusHandler>;
let onPauseLiveEvents: Mock<() => void>;
let onResumeLiveEvents: Mock<() => void>;

function dispatchDocumentEvent(type: string) {
	documentTarget.dispatchEvent(new Event(type));
}

function dispatchWindowEvent(type: string) {
	windowTarget.dispatchEvent(new Event(type));
}

function setVisibility(next: DocumentVisibilityState) {
	visibilityState = next;
	dispatchDocumentEvent('visibilitychange');
}

function startTracking() {
	onStatusChange = vi.fn<PresenceStatusHandler>();
	onPauseLiveEvents = vi.fn();
	onResumeLiveEvents = vi.fn();
	cleanup = initPresenceTracking(
		() => [{ serverId: 'origin', baseUrl: 'https://chat.example.test/api/connect', bearerToken: 't' }],
		onStatusChange,
		{ onPauseLiveEvents, onResumeLiveEvents }
	);
}

function sentStatuses(): APIPresenceStatus[] {
	return mocks.reportPresence.mock.calls.map((call) => call[0]);
}

function sentUserSelectedFlags(): Array<boolean | undefined> {
	return mocks.reportPresence.mock.calls.map((call) => call[1]);
}

describe('initPresenceTracking', () => {
	beforeEach(() => {
		vi.useFakeTimers({ now: 0 });
		mocks.reportPresence = vi.fn<ReportPresence>((status) => Promise.resolve(status));
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
		mocks.reportPresence.mockImplementation((status, userSelected) =>
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

	it('does not report presence while invisible and pauses live events', () => {
		startTracking();
		setPresenceMode('invisible');
		vi.advanceTimersByTime(60_000);
		dispatchDocumentEvent('pointermove');

		expect(sentStatuses()).toEqual([APIPresenceStatus.ONLINE]);
		expect(onStatusChange).toHaveBeenLastCalledWith(PresenceStatus.Offline);
		expect(onPauseLiveEvents).toHaveBeenCalledOnce();

		setPresenceMode('auto');

		expect(onResumeLiveEvents).toHaveBeenCalled();
		expect(sentStatuses()).toEqual([APIPresenceStatus.ONLINE, APIPresenceStatus.ONLINE]);
		expect(sentUserSelectedFlags()).toEqual([false, true]);
	});
});
