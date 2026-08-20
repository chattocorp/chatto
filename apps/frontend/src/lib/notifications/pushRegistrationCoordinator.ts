type RegistrationOperation = (signal: AbortSignal) => Promise<boolean>;
type CleanupOperation = () => Promise<void>;
type CrossTabSuspension = 'disabled' | 'leaving';
type CrossTabSuspensionState = {
  available: boolean;
  suspension: CrossTabSuspension | null;
};

const crossTabSuspensionKeyPrefix = 'chatto.push-registration.suspended.';
const operationTails = new Map<string, Promise<unknown>>();
const registrationEpochs = new Map<string, number>();
const suspendedServers = new Map<string, { crossTabPersisted: boolean }>();
const activeRegistrations = new Map<string, AbortController>();

function epoch(serverId: string): number {
  return registrationEpochs.get(serverId) ?? 0;
}

function crossTabSuspensionKey(serverId: string): string {
  return crossTabSuspensionKeyPrefix + serverId;
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

function enqueue<T>(serverId: string, operation: () => Promise<T>): Promise<T> {
  const previous = operationTails.get(serverId) ?? Promise.resolve();
  const current = previous.catch(() => undefined).then(operation);
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
  return enqueue(serverId, cleanup);
}

/** Persists suspension across same-origin tabs before sign-out or removal. */
export function suspendPushRegistrationBeforeLeaving(
  serverId: string,
  cleanup: CleanupOperation
): Promise<void> {
  const crossTabPersisted = setCrossTabSuspension(serverId, 'leaving');
  suspendLocally(serverId, crossTabPersisted);
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
