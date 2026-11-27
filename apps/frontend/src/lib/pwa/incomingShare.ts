import { Codecs, globalSlot } from '@chatto/client/storage/slot';
import type { ServerStateStore } from '@chatto/client/server/store';

/** Incoming files and text stay in memory until the member selects a conversation. */
export interface IncomingShare {
  text: string;
  files: File[];
}

/** Only the destination identifier is remembered across shares. */
export const lastShareDestination = globalSlot('lastShareDestination', '', Codecs.string);

/** Get the worker's single-use payload. An expired or interrupted handoff returns null. */
export async function receiveShare(id: string): Promise<IncomingShare | null> {
  if (!id || !('serviceWorker' in navigator)) return null;
  const registration = await navigator.serviceWorker.getRegistration('/share-target/');
  const worker = registration?.active;
  if (!worker) return null;
  const channel = new MessageChannel();
  return new Promise((resolve) => {
    const finish = (value: IncomingShare | null) => {
      clearTimeout(timeout);
      channel.port1.close();
      resolve(value);
    };
    const timeout = setTimeout(() => finish(null), 5000);
    channel.port1.onmessage = (event) => finish(event.data);
    channel.port1.onmessageerror = () => finish(null);
    worker.postMessage(id, [channel.port2]);
  });
}

type Delivery = {
  id: string;
  store: ServerStateStore;
  roomId: string;
  share: IncomingShare;
  destination: string;
};
let delivery: Delivery | null = null;

/** Release payloads when their history entry or account is no longer active. */
export function discardOtherShares(activeId?: string): void {
  if (delivery && delivery.id !== activeId) delivery = null;
}

/** Keep the payload in this window while the destination decides whether to accept it. */
export function prepareShare(
  store: ServerStateStore,
  roomId: string,
  share: IncomingShare,
  destination: string
): string {
  const id = crypto.randomUUID();
  delivery = { id, store, roomId, share, destination };
  return id;
}

/** The picker can recover a rejected delivery from the current account. */
export function pendingShare(id: string | undefined): Delivery | null {
  return id && delivery?.id === id ? delivery : null;
}

/** Only a successful composer commit acknowledges this exact delivery. */
export function acknowledgeShare(accepted: Delivery): void {
  if (delivery !== accepted) return;
  delivery = null;
  lastShareDestination.set(accepted.destination);
}
