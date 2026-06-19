import { getActiveServer } from '$lib/state/activeServer.svelte';
import { wireEventBusManager, type WireEventHandler } from '$lib/state/server/wireEventBus.svelte';

export function useWireEvent(handler: WireEventHandler): void {
  $effect(() => {
    const serverId = getActiveServer();
    if (!serverId) return;

    const bus = wireEventBusManager.getBus(serverId);
    if (!bus) return;

    bus.handlers.add(handler);
    return () => {
      bus.handlers.delete(handler);
    };
  });
}
