/**
 * The AT Protocol handle of this device's last successful sign-in (FDR-058).
 *
 * The handle page suggests it, and the signed-out screen of another server
 * offers **Continue as @handle** with it. It is only a hint: each server
 * still signs the user in through its own provider. The handle page stores
 * the entered handle as pending. The app keeps it when it next loads with a
 * signed-in origin session.
 */

import { Codecs, globalSlot } from '@chatto/client/storage/slot';

const slot = globalSlot('atproto-handle', '', Codecs.string);
const pendingSlot = globalSlot('atproto-handle-pending', '', Codecs.string);

/** The remembered handle without `@`, or `''`. */
export function rememberedAtprotoHandle(): string {
  return slot.get();
}

/** Remember `handle` after a successful sign-in. */
export function rememberAtprotoHandle(handle: string): void {
  const normalized = handle.trim().replace(/^@/, '');
  if (normalized) slot.set(normalized);
}

/** Forget the remembered handle. */
export function forgetAtprotoHandle(): void {
  slot.remove();
  pendingSlot.remove();
}

/** Store the handle of a sign-in that has started but not finished. */
export function setPendingAtprotoHandle(handle: string): void {
  pendingSlot.set(handle.trim().replace(/^@/, ''));
}

/** Remember the pending handle after its sign-in succeeded. */
export function confirmPendingAtprotoHandle(): void {
  const pending = pendingSlot.get();
  pendingSlot.remove();
  if (pending) rememberAtprotoHandle(pending);
}
