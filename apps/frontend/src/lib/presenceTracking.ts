import { createPresenceAPI, APIPresenceStatus, type PresenceAPIConfig } from '@chatto/api-client/presence';
import { PresenceStatus } from '$lib/render/types';
import { presencePreference, type PresenceMode } from '$lib/state/presencePreference.svelte';

const IDLE_TIMEOUT_MS = 5 * 60 * 1000;
const HIDDEN_DELAY_MS = 10_000;
const NOISY_ACTIVITY_THROTTLE_MS = 1_000;
const PRESENCE_REFRESH_MS = 30_000;
const PRESENCE_MODE_STORAGE_KEY = 'chatto.presence.mode';

type ActivityState = 'active' | 'idle' | 'hidden';

export type PresenceReporterConfig = PresenceAPIConfig;

export type PresenceTrackingOptions = {
	onPauseLiveEvents?: () => void;
	onResumeLiveEvents?: () => void;
};

let initialized = false;
let applyModeFromUI: ((mode: PresenceMode) => void) | null = null;

function apiStatusToPresenceStatus(status: APIPresenceStatus): PresenceStatus {
	switch (status) {
		case APIPresenceStatus.AWAY:
			return PresenceStatus.Away;
		case APIPresenceStatus.DO_NOT_DISTURB:
			return PresenceStatus.DoNotDisturb;
		default:
			return PresenceStatus.Online;
	}
}

function presenceStatusToAPIStatus(status: PresenceStatus): APIPresenceStatus {
	switch (status) {
		case PresenceStatus.Away:
			return APIPresenceStatus.AWAY;
		case PresenceStatus.DoNotDisturb:
			return APIPresenceStatus.DO_NOT_DISTURB;
		default:
			return APIPresenceStatus.ONLINE;
	}
}

function modeToExplicitStatus(mode: PresenceMode): PresenceStatus | null {
	switch (mode) {
		case 'away':
			return PresenceStatus.Away;
		case 'doNotDisturb':
			return PresenceStatus.DoNotDisturb;
		case 'invisible':
			return PresenceStatus.Offline;
		default:
			return null;
	}
}

function readStoredMode(): PresenceMode {
	if (typeof localStorage === 'undefined') return 'auto';
	const stored = localStorage.getItem(PRESENCE_MODE_STORAGE_KEY);
	if (
		stored === 'auto' ||
		stored === 'away' ||
		stored === 'doNotDisturb' ||
		stored === 'invisible'
	) {
		return stored;
	}
	return 'auto';
}

export function shouldPauseLiveEventsForStoredPresence(): boolean {
	return readStoredMode() === 'invisible';
}

function storeMode(mode: PresenceMode) {
	if (typeof localStorage === 'undefined') return;
	localStorage.setItem(PRESENCE_MODE_STORAGE_KEY, mode);
}

export function setPresenceMode(mode: PresenceMode) {
	storeMode(mode);
	presencePreference.mode = mode;
	applyModeFromUI?.(mode);
}

export function initPresenceTracking(
	getReporters: () => PresenceReporterConfig[],
	onStatusChange?: (status: PresenceStatus) => void,
	options: PresenceTrackingOptions = {}
): () => void {
	if (initialized) return () => {};
	initialized = true;

	let currentMode = readStoredMode();
	let currentState: ActivityState = document.visibilityState === 'hidden' ? 'hidden' : 'active';
	let currentVisibleStatus: PresenceStatus | null = null;
	let idleTimer: ReturnType<typeof setTimeout> | null = null;
	let hiddenTimer: ReturnType<typeof setTimeout> | null = null;
	let refreshTimer: ReturnType<typeof setInterval> | null = null;
	let lastTimerResetAt = 0;
	let reportRevision = 0;

	presencePreference.mode = currentMode;

	function emitLocalStatus(status: PresenceStatus) {
		presencePreference.effectiveStatus = status;
		onStatusChange?.(status);
	}

	function applyAcceptedStatus(accepted: APIPresenceStatus, revision: number) {
		if (revision !== reportRevision || currentMode === 'invisible') return;
		const acceptedStatus = apiStatusToPresenceStatus(accepted);
		currentVisibleStatus = acceptedStatus;
		if (presencePreference.effectiveStatus !== acceptedStatus) {
			emitLocalStatus(acceptedStatus);
		}
	}

	function sendPresenceReport(status: PresenceStatus, userSelected: boolean, revision: number) {
		for (const config of getReporters()) {
			createPresenceAPI(config)
				.reportPresence(presenceStatusToAPIStatus(status), userSelected)
				.then((accepted) => applyAcceptedStatus(accepted, revision))
				.catch(() => {});
		}
	}

	function reportStatus(status: PresenceStatus, userSelected = false) {
		const revision = ++reportRevision;
		currentVisibleStatus = status;
		emitLocalStatus(status);
		sendPresenceReport(status, userSelected, revision);
	}

	function clearRefreshTimer() {
		if (refreshTimer) {
			clearInterval(refreshTimer);
			refreshTimer = null;
		}
	}

	function ensureRefreshTimer() {
		if (refreshTimer) return;
		refreshTimer = setInterval(() => {
			if (currentMode === 'invisible' || currentVisibleStatus === null) return;
			const userSelected = currentMode !== 'auto';
			sendPresenceReport(currentVisibleStatus, userSelected, ++reportRevision);
		}, PRESENCE_REFRESH_MS);
	}

	function resetIdleTimer() {
		if (idleTimer) clearTimeout(idleTimer);
		lastTimerResetAt = Date.now();
		idleTimer = setTimeout(() => transition('idle'), IDLE_TIMEOUT_MS);
	}

	function statusForAutoState(state: ActivityState): PresenceStatus {
		return state === 'active' ? PresenceStatus.Online : PresenceStatus.Away;
	}

	function applyMode(mode: PresenceMode, persist = false) {
		currentMode = mode;
		presencePreference.mode = mode;
		if (persist) storeMode(mode);

		if (mode === 'invisible') {
			clearRefreshTimer();
			reportRevision++;
			currentVisibleStatus = null;
			emitLocalStatus(PresenceStatus.Offline);
			options.onPauseLiveEvents?.();
			return;
		}

		options.onResumeLiveEvents?.();
		if (mode === 'auto') {
			currentState = document.visibilityState === 'hidden' ? 'hidden' : 'active';
			reportStatus(statusForAutoState(currentState), persist);
			ensureRefreshTimer();
			resetIdleTimer();
			return;
		}

		const explicitStatus = modeToExplicitStatus(mode);
		reportStatus(explicitStatus ?? statusForAutoState(currentState), true);
		ensureRefreshTimer();
	}

	applyModeFromUI = (mode) => applyMode(mode, true);

	function transition(newState: ActivityState) {
		if (newState === currentState) return;
		currentState = newState;
		if (currentMode !== 'auto') return;
		reportStatus(statusForAutoState(newState));
		if (newState === 'active') resetIdleTimer();
	}

	function onActivity(noisy = false) {
		if (currentMode !== 'auto') return;

		if (currentState !== 'active') {
			transition('active');
			return;
		}

		if (!noisy || Date.now() - lastTimerResetAt >= NOISY_ACTIVITY_THROTTLE_MS) {
			resetIdleTimer();
		}
	}

	function onQuietActivity() {
		onActivity(false);
	}

	function onNoisyActivity() {
		onActivity(true);
	}

	const quietActivityEvents = ['pointerdown', 'keydown', 'touchstart'] as const;
	const noisyActivityEvents = ['pointermove', 'wheel', 'scroll'] as const;

	for (const event of quietActivityEvents) {
		document.addEventListener(event, onQuietActivity, { passive: true });
	}
	for (const event of noisyActivityEvents) {
		document.addEventListener(event, onNoisyActivity, { passive: true });
	}

	function onVisibilityChange() {
		if (document.visibilityState === 'hidden') {
			hiddenTimer = setTimeout(() => transition('hidden'), HIDDEN_DELAY_MS);
		} else {
			if (hiddenTimer) {
				clearTimeout(hiddenTimer);
				hiddenTimer = null;
			}
			transition('active');
		}
	}

	function onStorage(event: StorageEvent) {
		if (event.key !== PRESENCE_MODE_STORAGE_KEY || event.newValue === null) return;
		if (
			event.newValue === 'auto' ||
			event.newValue === 'away' ||
			event.newValue === 'doNotDisturb' ||
			event.newValue === 'invisible'
		) {
			applyMode(event.newValue);
		}
	}

	document.addEventListener('visibilitychange', onVisibilityChange);
	window.addEventListener('focus', onQuietActivity);
	window.addEventListener('storage', onStorage);

	resetIdleTimer();
	applyMode(currentMode);

	return () => {
		for (const event of quietActivityEvents) {
			document.removeEventListener(event, onQuietActivity);
		}
		for (const event of noisyActivityEvents) {
			document.removeEventListener(event, onNoisyActivity);
		}
		document.removeEventListener('visibilitychange', onVisibilityChange);
		window.removeEventListener('focus', onQuietActivity);
		window.removeEventListener('storage', onStorage);
		if (idleTimer) clearTimeout(idleTimer);
		if (hiddenTimer) clearTimeout(hiddenTimer);
		clearRefreshTimer();
		if (applyModeFromUI) applyModeFromUI = null;
		initialized = false;
	};
}

export const __presenceTrackingTest = {
	PRESENCE_MODE_STORAGE_KEY,
	apiStatusToPresenceStatus
};
