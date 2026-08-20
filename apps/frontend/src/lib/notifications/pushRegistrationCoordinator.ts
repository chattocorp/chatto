type RegistrationOperation = (signal: AbortSignal) => Promise<boolean>;
type CleanupOperation = () => Promise<void>;
type CrossTabSuspension = 'disabled' | 'leaving';

const crossTabSuspensionKeyPrefix = 'chatto.push-registration.suspended.';
const operationTails = new Map<string, Promise<unknown>>();
const registrationEpochs = new Map<string, number>();
const suspendedServers = new Set<string>();
const activeRegistrations = new Map<string, AbortController>();

function epoch(serverId: string): number {
  return registrationEpochs.get(serverId) ?? 0;
}

function crossTabSuspensionKey(serverId: string): string {
  return crossTabSuspensionKeyPrefix + serverId;
}

function crossTabSuspension(serverId: string): CrossTabSuspension | null {
  if (typeof window === 'undefined') return null;
  try {
    const value = window.localStorage?.getItem(crossTabSuspensionKey(serverId));
    return value === 'disabled' || value === 'leaving' ? value : null;
  } catch {
    return null;
  }
}

function setCrossTabSuspension(serverId: string, suspension: CrossTabSuspension | null): void {
  if (typeof window === 'undefined') return;
  try {
    if (suspension) window.localStorage?.setItem(crossTabSuspensionKey(serverId), suspension);
    else window.localStorage?.removeItem(crossTabSuspensionKey(serverId));
  } catch {
    // Local cancellation still protects this tab when browser storage is unavailable.
  }
}

function isSuspended(serverId: string): boolean {
  return suspendedServers.has(serverId) || crossTabSuspension(serverId) !== null;
}

function suspendLocally(serverId: string): void {
  suspendedServers.add(serverId);
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
  setCrossTabSuspension(serverId, 'disabled');
  suspendLocally(serverId);
  return enqueue(serverId, cleanup);
}

/** Persists suspension across same-origin tabs before sign-out or removal. */
export function suspendPushRegistrationBeforeLeaving(
  serverId: string,
  cleanup: CleanupOperation
): Promise<void> {
  setCrossTabSuspension(serverId, 'leaving');
  suspendLocally(serverId);
  return enqueue(serverId, cleanup);
}

/** Reports cancellation to registration work after each browser/network await. */
export function isPushRegistrationSuspended(serverId: string, signal?: AbortSignal): boolean {
  return signal?.aborted === true || isSuspended(serverId);
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
