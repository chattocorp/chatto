type RegistrationOperation = (signal: AbortSignal) => Promise<boolean>;
type CleanupOperation = () => Promise<void>;
/**
 * Sign-out or server removal in progress. Earlier versions also stored a
 * per-server `disabled` value; `takeLegacyDisabledPushRegistration` turns it
 * into the device-wide opt-out.
 */
type CrossTabSuspension = 'leaving';
type CrossTabSuspensionState = {
  available: boolean;
  suspension: CrossTabSuspension | null;
};

const crossTabSuspensionKeyPrefix = 'chatto.push-registration.suspended.';
/**
 * Device-wide push opt-out. While it is set, no tab registers any server.
 * Push is on for every registered server or for none of them.
 */
export const pushDisabledOnDeviceKey = 'chatto.push-registration.disabled';
const crossTabRefreshKeyPrefix = 'chatto.push-registration.refresh.';
const crossTabStorageProbeKeyPrefix = 'chatto.push-registration.storage-probe.';
const crossTabLockNamePrefix = 'chatto.push-registration.';
const crossTabChannelName = 'chatto-push-registration';
const operationTails = new Map<string, Promise<unknown>>();
const registrationEpochs = new Map<string, number>();
const suspendedServers = new Map<string, { crossTabPersisted: boolean }>();
const activeRegistrations = new Map<string, AbortController>();
const refreshListeners = new Set<(serverId: string, requestId: string) => void>();
let coordinationChannel: BroadcastChannel | null | undefined;
let storageListenerInstalled = false;

/** Whether push-registration suspension can survive reloads and coordinate future tabs. */
export function hasDurablePushCoordinationStorage(): boolean {
  if (typeof window === 'undefined') return false;
  const key =
    crossTabStorageProbeKeyPrefix + Date.now().toString(36) + Math.random().toString(36).slice(2);
  try {
    const storage = window.localStorage;
    if (!storage) return false;
    storage.setItem(key, '1');
    const stored = storage.getItem(key) === '1';
    storage.removeItem(key);
    return stored;
  } catch {
    return false;
  }
}

function epoch(serverId: string): number {
  return registrationEpochs.get(serverId) ?? 0;
}

function crossTabSuspensionKey(serverId: string): string {
  return crossTabSuspensionKeyPrefix + serverId;
}

function crossTabRefreshKey(serverId: string): string {
  return crossTabRefreshKeyPrefix + serverId;
}

function crossTabSuspensionState(serverId: string): CrossTabSuspensionState {
  if (typeof window === 'undefined') return { available: false, suspension: null };
  try {
    const storage = window.localStorage;
    if (!storage) return { available: false, suspension: null };
    const value = storage.getItem(crossTabSuspensionKey(serverId));
    return {
      available: true,
      suspension: value === 'leaving' ? value : null
    };
  } catch {
    return { available: false, suspension: null };
  }
}

function crossTabSuspension(serverId: string): CrossTabSuspension | null {
  return crossTabSuspensionState(serverId).suspension;
}

function setCrossTabSuspension(serverId: string, suspension: CrossTabSuspension | null): boolean {
  if (typeof window === 'undefined') return false;
  try {
    const storage = window.localStorage;
    if (!storage) return false;
    if (suspension) storage.setItem(crossTabSuspensionKey(serverId), suspension);
    else storage.removeItem(crossTabSuspensionKey(serverId));
    return true;
  } catch {
    // Local cancellation still protects this tab when browser storage is unavailable.
    return false;
  }
}

function isSuspended(serverId: string): boolean {
  return (
    isPushDisabledOnDevice() ||
    suspendedServers.has(serverId) ||
    crossTabSuspension(serverId) !== null
  );
}

/** Whether the user turned push notifications off on this device. */
export function isPushDisabledOnDevice(): boolean {
  if (typeof window === 'undefined') return false;
  try {
    return window.localStorage?.getItem(pushDisabledOnDeviceKey) === '1';
  } catch {
    return false;
  }
}

function abortActiveRegistrations(): void {
  for (const controller of activeRegistrations.values()) controller.abort();
}

/**
 * Turns push off on this device for every server. The opt-out is stored
 * before any cleanup starts, so every tab stops registering at its next
 * check; storage events abort registrations that other tabs run now.
 * `cleanup` then runs for each server after its active registration.
 * Rejects when the opt-out cannot be stored.
 */
export function disablePushRegistrationOnDevice(
  serverIds: string[],
  cleanup: (serverId: string) => Promise<void>
): Promise<void> {
  ensureCrossTabCoordination();
  try {
    window.localStorage.setItem(pushDisabledOnDeviceKey, '1');
  } catch (error) {
    return Promise.reject(error);
  }
  abortActiveRegistrations();
  return Promise.all(serverIds.map((serverId) => enqueue(serverId, () => cleanup(serverId)))).then(
    () => undefined
  );
}

/** Turns push on again on this device. Registration starts separately. */
export function enablePushRegistrationOnDevice(): void {
  try {
    window.localStorage.removeItem(pushDisabledOnDeviceKey);
  } catch {
    // Registration checks read the opt-out and stay off until storage works.
  }
}

/**
 * Removes the per-server `disabled` values of earlier versions and reports
 * whether one existed. Push is now on for all servers or for none, so the
 * caller turns push off on the device to keep the user's earlier opt-out.
 */
export function takeLegacyDisabledPushRegistration(): boolean {
  if (typeof window === 'undefined') return false;
  try {
    const storage = window.localStorage;
    const keys: string[] = [];
    for (let index = 0; index < storage.length; index++) {
      const key = storage.key(index);
      if (key?.startsWith(crossTabSuspensionKeyPrefix) && storage.getItem(key) === 'disabled') {
        keys.push(key);
      }
    }
    for (const key of keys) storage.removeItem(key);
    return keys.length > 0;
  } catch {
    return false;
  }
}

function suspendLocally(serverId: string, crossTabPersisted: boolean): void {
  suspendedServers.set(serverId, { crossTabPersisted });
  registrationEpochs.set(serverId, epoch(serverId) + 1);
  activeRegistrations.get(serverId)?.abort();
}

function ensureCrossTabCoordination(): void {
  if (typeof window === 'undefined') return;

  if (!storageListenerInstalled && typeof window.addEventListener === 'function') {
    window.addEventListener('storage', (event) => {
      if (event.key === pushDisabledOnDeviceKey) {
        if (event.newValue === '1') abortActiveRegistrations();
      } else if (event.key?.startsWith(crossTabSuspensionKeyPrefix)) {
        if (event.newValue !== 'leaving') return;
        const serverId = event.key.slice(crossTabSuspensionKeyPrefix.length);
        if (serverId) suspendLocally(serverId, true);
      } else if (event.key?.startsWith(crossTabRefreshKeyPrefix) && event.newValue) {
        const serverId = event.key.slice(crossTabRefreshKeyPrefix.length);
        if (serverId) notifyRefreshListeners(serverId, event.newValue);
      }
    });
    storageListenerInstalled = true;
  }

  if (coordinationChannel !== undefined) return;
  coordinationChannel = null;
  if (typeof window.BroadcastChannel === 'undefined') return;

  try {
    coordinationChannel = new window.BroadcastChannel(crossTabChannelName);
    coordinationChannel.addEventListener('message', (event: MessageEvent<unknown>) => {
      if (event.data === null || typeof event.data !== 'object') return;
      const message = event.data as {
        type?: unknown;
        serverId?: unknown;
        crossTabPersisted?: unknown;
      };
      if (typeof message.serverId !== 'string') return;
      if (message.type === 'suspend') {
        suspendLocally(message.serverId, message.crossTabPersisted === true);
      }
    });
  } catch {
    coordinationChannel = null;
  }
}

function notifyRefreshListeners(serverId: string, requestId: string): void {
  for (const listener of refreshListeners) listener(serverId, requestId);
}

/** Returns the durable reassertion request currently pending for one server. */
export function pendingPushRegistrationRefresh(serverId: string): string | null {
  if (typeof window === 'undefined') return null;
  try {
    return window.localStorage?.getItem(crossTabRefreshKey(serverId)) ?? null;
  } catch {
    return null;
  }
}

/** Clears a reassertion request only after the save covering it succeeds. */
export function completePushRegistrationRefresh(serverId: string, requestId: string): void {
  if (typeof window === 'undefined') return;
  try {
    const key = crossTabRefreshKey(serverId);
    if (window.localStorage?.getItem(key) === requestId) window.localStorage.removeItem(key);
  } catch {
    // A later startup refresh remains safe when the marker cannot be cleared.
  }
}

/** Durably requests that active same-origin realms reassert one subscription. */
export function requestPushRegistrationRefresh(serverId: string): void {
  ensureCrossTabCoordination();
  const requestId = Date.now().toString(36) + Math.random().toString(36).slice(2);
  try {
    window.localStorage?.setItem(crossTabRefreshKey(serverId), requestId);
  } catch {
    // The local foreground realm can still repair state immediately.
  }
  notifyRefreshListeners(serverId, requestId);
}

/** Subscribes a long-lived registration owner to cross-tab refresh requests. */
export function onPushRegistrationRefresh(
  listener: (serverId: string, requestId: string) => void
): () => void {
  refreshListeners.add(listener);
  ensureCrossTabCoordination();
  return () => refreshListeners.delete(listener);
}

function broadcastSuspension(serverId: string, crossTabPersisted: boolean): void {
  ensureCrossTabCoordination();
  try {
    coordinationChannel?.postMessage({ type: 'suspend', serverId, crossTabPersisted });
  } catch {
    // Storage events still propagate the suspension when messaging is unavailable.
  }
}

function withCrossTabLock<T>(serverId: string, operation: () => Promise<T>): Promise<T> {
  if (typeof navigator === 'undefined' || !navigator.locks) return operation();
  return navigator.locks.request(crossTabLockNamePrefix + serverId, () => operation());
}

function enqueue<T>(serverId: string, operation: () => Promise<T>): Promise<T> {
  ensureCrossTabCoordination();
  const previous = operationTails.get(serverId) ?? Promise.resolve();
  const current = previous.catch(() => undefined).then(() => withCrossTabLock(serverId, operation));
  operationTails.set(serverId, current);
  return current.finally(() => {
    if (operationTails.get(serverId) === current) operationTails.delete(serverId);
  });
}

/** Queues registration behind earlier work and skips it after sign-out begins. */
export function enqueuePushRegistration(
  serverId: string,
  operation: RegistrationOperation
): Promise<boolean> {
  if (isSuspended(serverId)) return Promise.resolve(false);
  const queuedEpoch = epoch(serverId);
  return enqueue(serverId, async () => {
    if (isSuspended(serverId) || epoch(serverId) !== queuedEpoch) return false;

    const controller = new AbortController();
    activeRegistrations.set(serverId, controller);
    let resolveCancellation!: (value: boolean) => void;
    const cancellation = new Promise<boolean>((resolve) => {
      resolveCancellation = resolve;
    });
    const onAbort = () => resolveCancellation(false);
    controller.signal.addEventListener('abort', onAbort, { once: true });
    try {
      return await Promise.race([operation(controller.signal), cancellation]);
    } finally {
      controller.signal.removeEventListener('abort', onAbort);
      if (activeRegistrations.get(serverId) === controller) {
        activeRegistrations.delete(serverId);
      }
    }
  });
}

/** Persists suspension across same-origin tabs before sign-out or removal. */
export function suspendPushRegistrationBeforeLeaving(
  serverId: string,
  cleanup: CleanupOperation
): Promise<void> {
  const crossTabPersisted = setCrossTabSuspension(serverId, 'leaving');
  suspendLocally(serverId, crossTabPersisted);
  broadcastSuspension(serverId, crossTabPersisted);
  return enqueue(serverId, cleanup);
}

/** Reports cancellation to registration work after each browser/network await. */
export function isPushRegistrationSuspended(serverId: string, signal?: AbortSignal): boolean {
  return signal?.aborted === true || isSuspended(serverId);
}

/** Whether stale work still owns cleanup after another realm may have resumed. */
export function shouldInvalidateCancelledPushRegistration(serverId: string): boolean {
  if (isPushDisabledOnDevice()) return true;
  const shared = crossTabSuspensionState(serverId);
  if (shared.suspension !== null) return true;
  const local = suspendedServers.get(serverId);
  return local !== undefined && (!local.crossTabPersisted || !shared.available);
}

/** Clears cross-tab sign-out suspension once new authentication is installed. */
export function resumePushRegistrationAfterAuthentication(serverId: string): void {
  setCrossTabSuspension(serverId, null);
  suspendedServers.delete(serverId);
  registrationEpochs.set(serverId, epoch(serverId) + 1);
}
