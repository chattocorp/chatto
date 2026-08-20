type RegistrationOperation = (signal: AbortSignal) => Promise<boolean>;
type CleanupOperation = () => Promise<void>;
type CrossTabSuspension = 'disabled' | 'leaving';
type CrossTabSuspensionState = {
  available: boolean;
  suspension: CrossTabSuspension | null;
};

const crossTabSuspensionKeyPrefix = 'chatto.push-registration.suspended.';
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

/** Whether push opt-out state can survive reloads and coordinate future tabs. */
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
      suspension: value === 'disabled' || value === 'leaving' ? value : null
    };
  } catch {
    return { available: false, suspension: null };
  }
}

function crossTabSuspension(serverId: string): CrossTabSuspension | null {
  return crossTabSuspensionState(serverId).suspension;
}

function setCrossTabSuspension(
  serverId: string,
  suspension: CrossTabSuspension | null
): boolean {
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
  return suspendedServers.has(serverId) || crossTabSuspension(serverId) !== null;
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
      if (event.key?.startsWith(crossTabSuspensionKeyPrefix)) {
        if (event.newValue !== 'disabled' && event.newValue !== 'leaving') return;
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
  const current = previous
    .catch(() => undefined)
    .then(() => withCrossTabLock(serverId, operation));
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

/** Cancels queued registration and runs cleanup after any active registration. */
export function suspendPushRegistration(
  serverId: string,
  cleanup: CleanupOperation
): Promise<void> {
  const crossTabPersisted = setCrossTabSuspension(serverId, 'disabled');
  suspendLocally(serverId, crossTabPersisted);
  broadcastSuspension(serverId, crossTabPersisted);
  return enqueue(serverId, cleanup);
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
  const shared = crossTabSuspensionState(serverId);
  if (shared.suspension !== null) return true;
  const local = suspendedServers.get(serverId);
  return local !== undefined && (!local.crossTabPersisted || !shared.available);
}

/** Allows registration again after a new authenticated session is installed. */
export function resumePushRegistration(serverId: string): void {
  if (crossTabSuspension(serverId) === 'disabled') {
    setCrossTabSuspension(serverId, null);
  }
  suspendedServers.delete(serverId);
  registrationEpochs.set(serverId, epoch(serverId) + 1);
}

/** Clears cross-tab sign-out suspension once new authentication is installed. */
export function resumePushRegistrationAfterAuthentication(serverId: string): void {
  setCrossTabSuspension(serverId, null);
  resumePushRegistration(serverId);
}
