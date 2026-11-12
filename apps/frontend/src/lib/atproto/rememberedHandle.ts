/**
 * The AT Protocol handle that this device last used for sign-in (FDR-056).
 *
 * Sign-in forms suggest it, and the signed-out screen of another server
 * offers **Continue as @handle** with it. It is only a hint: each server
 * still signs the user in through its own provider. The handle stays in this
 * browser's storage until the user signs in with it.
 */

import { Codecs, globalSlot } from '@chatto/client/storage/slot';

const slot = globalSlot('atproto-handle', '', Codecs.string);

/** The remembered handle without `@`, or `''`. */
export function rememberedAtprotoHandle(): string {
  return slot.get();
}

/** Remember `handle` for later sign-ins. An empty handle forgets it. */
export function rememberAtprotoHandle(handle: string): void {
  const normalized = handle.trim().replace(/^@/, '');
  if (normalized) slot.set(normalized);
  else slot.remove();
}
