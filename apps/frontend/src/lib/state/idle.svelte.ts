import { serverRegistry } from '$lib/client';
import { serverUi } from '$lib/state/server/serverUi';

/** Provides cross-server activity state for browser integrations. */
class IdleState {
  /** True iff the user is connected to any voice call on any registered server. */
  get isInAnyCall(): boolean {
    for (const server of serverRegistry.servers) {
      const store = serverRegistry.tryGetStore(server.id);
      if (store && serverUi(store).voiceCall.isInAnyCall) return true;
    }
    return false;
  }
}

export const idleState = new IdleState();
