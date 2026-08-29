import { APIPresenceStatus, type PresenceAPI } from '$lib/api-client/presence';
import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import { presencePreference, type PresenceMode } from '$lib/state/presencePreference.svelte';

const PRESENCE_REFRESH_MS = 30_000;
const PRESENCE_MODE_STORAGE_KEY = 'chatto.presence.mode';

export type PresenceReporter = Pick<PresenceAPI, 'updatePresence'>;

let initialized = false;
let applyModeFromUI: ((mode: PresenceMode) => void) | null = null;

function apiStatusToPresenceStatus(status: APIPresenceStatus): PresenceStatus {
	switch (status) {
		case APIPresenceStatus.AWAY:
			return PresenceStatus.AWAY;
		case APIPresenceStatus.DO_NOT_DISTURB:
			return PresenceStatus.DO_NOT_DISTURB;
		default:
			return PresenceStatus.ONLINE;
	}
}

function presenceStatusToAPIStatus(status: PresenceStatus): APIPresenceStatus {
	switch (status) {
		case PresenceStatus.AWAY:
			return APIPresenceStatus.AWAY;
		case PresenceStatus.DO_NOT_DISTURB:
			return APIPresenceStatus.DO_NOT_DISTURB;
		default:
			return APIPresenceStatus.ONLINE;
	}
}

function modeToExplicitStatus(mode: PresenceMode): PresenceStatus {
	switch (mode) {
		case 'away':
			return PresenceStatus.AWAY;
		case 'doNotDisturb':
			return PresenceStatus.DO_NOT_DISTURB;
		case 'invisible':
			return PresenceStatus.OFFLINE;
		default:
			return PresenceStatus.ONLINE;
	}
}

function isPresenceMode(value: unknown): value is PresenceMode {
	return value === 'online' || value === 'away' || value === 'doNotDisturb' || value === 'invisible';
}

/**
 * Reads the stored presence mode. Legacy stored values such as 'auto'
 * (removed implicit idle-away mode) normalize to the explicit 'online' mode.
 */
function readStoredMode(): PresenceMode {
	if (typeof localStorage === 'undefined') return 'online';
	const stored = localStorage.getItem(PRESENCE_MODE_STORAGE_KEY);
	return isPresenceMode(stored) ? stored : 'online';
}

function storeMode(mode: PresenceMode) {
	if (typeof localStorage === 'undefined') return;
	localStorage.setItem(PRESENCE_MODE_STORAGE_KEY, mode);
}

/**
 * Rewrites the stored mode only when it holds a legacy or invalid value
 * (for example the retired 'auto' mode) so all tabs converge on explicit
 * modes. Never rewrites a valid or missing stored value: identical setItem
 * calls can surface as cross-tab storage events and needlessly re-apply
 * the same mode elsewhere.
 */
function normalizeStoredMode(mode: PresenceMode) {
	if (typeof localStorage === 'undefined') return;
	const stored = localStorage.getItem(PRESENCE_MODE_STORAGE_KEY);
	if (stored !== null && !isPresenceMode(stored)) storeMode(mode);
}

export function setPresenceMode(mode: PresenceMode) {
	storeMode(mode);
	presencePreference.mode = mode;
	applyModeFromUI?.(mode);
}

/**
 * Reports an explicit, user-selected presence status to all connected servers.
 * There is no automatic status: the client never switches modes on its own,
 * it only refreshes the currently chosen explicit status so server-side
 * presence TTLs do not expire.
 */
export function initPresenceTracking(
	getReporters: () => PresenceReporter[],
	onStatusChange?: (status: PresenceStatus) => void
): () => void {
	if (initialized) return () => {};
	initialized = true;

	let currentMode = readStoredMode();
	let currentVisibleStatus: PresenceStatus | null = null;
	let refreshTimer: ReturnType<typeof setInterval> | null = null;
	// Latest-request sequence. Out-of-order report responses must not let a
	// stale accepted status overwrite a newer explicit choice.
	let lastRequestSeq = 0;

	presencePreference.mode = currentMode;
	normalizeStoredMode(currentMode);

	function emitLocalStatus(status: PresenceStatus) {
		presencePreference.effectiveStatus = status;
		onStatusChange?.(status);
	}

	function sendPresenceReport(status: PresenceStatus) {
		const requestSeq = ++lastRequestSeq;
		for (const reporter of getReporters()) {
			reporter
				.updatePresence(presenceStatusToAPIStatus(status), true)
				.then((accepted) => {
					if (requestSeq !== lastRequestSeq || currentMode === 'invisible') return;
					const acceptedStatus = apiStatusToPresenceStatus(accepted);
					currentVisibleStatus = acceptedStatus;
					if (presencePreference.effectiveStatus !== acceptedStatus) {
						emitLocalStatus(acceptedStatus);
					}
				})
				.catch(() => {});
		}
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
			if (currentVisibleStatus === null) return;
			sendPresenceReport(currentVisibleStatus);
		}, PRESENCE_REFRESH_MS);
	}

	function applyMode(mode: PresenceMode, persist = false) {
		currentMode = mode;
		presencePreference.mode = mode;
		if (persist) storeMode(mode);

		if (mode === 'invisible') {
			clearRefreshTimer();
			lastRequestSeq++;
			currentVisibleStatus = null;
			emitLocalStatus(PresenceStatus.OFFLINE);
			return;
		}

		const explicitStatus = modeToExplicitStatus(mode);
		currentVisibleStatus = explicitStatus;
		emitLocalStatus(explicitStatus);
		sendPresenceReport(explicitStatus);
		ensureRefreshTimer();
	}

	applyModeFromUI = (mode) => applyMode(mode, true);

	function onStorage(event: StorageEvent) {
		if (event.key !== PRESENCE_MODE_STORAGE_KEY || !isPresenceMode(event.newValue)) return;
		applyMode(event.newValue);
	}

	window.addEventListener('storage', onStorage);

	applyMode(currentMode);

	return () => {
		window.removeEventListener('storage', onStorage);
		clearRefreshTimer();
		if (applyModeFromUI) applyModeFromUI = null;
		initialized = false;
	};
}

export const __presenceTrackingTest = {
	PRESENCE_MODE_STORAGE_KEY,
	apiStatusToPresenceStatus
};
