type RegistrationOperation = () => Promise<boolean>;
type CleanupOperation = () => Promise<void>;

const operationTails = new Map<string, Promise<unknown>>();
const registrationEpochs = new Map<string, number>();
const suspendedServers = new Set<string>();

function epoch(serverId: string): number {
  return registrationEpochs.get(serverId) ?? 0;
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
  if (suspendedServers.has(serverId)) return Promise.resolve(false);
  const queuedEpoch = epoch(serverId);
  return enqueue(serverId, async () => {
    if (suspendedServers.has(serverId) || epoch(serverId) !== queuedEpoch) return false;
    return operation();
  });
}

/** Cancels queued registration and runs cleanup after any active registration. */
export function suspendPushRegistration(
  serverId: string,
  cleanup: CleanupOperation
): Promise<void> {
  suspendedServers.add(serverId);
  registrationEpochs.set(serverId, epoch(serverId) + 1);
  return enqueue(serverId, cleanup);
}

/** Allows registration again after a new authenticated session is installed. */
export function resumePushRegistration(serverId: string): void {
  suspendedServers.delete(serverId);
  registrationEpochs.set(serverId, epoch(serverId) + 1);
}
