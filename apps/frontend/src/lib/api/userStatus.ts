export * from '@chatto/api-client/userStatus';
import { clearCustomStatus as baseClearCustomStatus, setCustomStatus as baseSetCustomStatus } from '@chatto/api-client/userStatus';
import { withAuthenticationRequired } from './clientHooks';
import type { CustomUserStatusAPIConfig } from '@chatto/api-client/userStatus';

export function setCustomStatus(
  config: CustomUserStatusAPIConfig,
  input: { emoji: string; text: string; expiresAt?: string | null }
) {
  return baseSetCustomStatus(withAuthenticationRequired(config), input);
}

export function clearCustomStatus(config: CustomUserStatusAPIConfig) {
  return baseClearCustomStatus(withAuthenticationRequired(config));
}
