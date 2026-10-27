/** Action-specific notices. Shared wording does not make reasons interchangeable. */
import {
  UsernameChangeReason,
  UsernameChoiceReason,
  PasswordChangeReason,
  IdentityDisconnectReason,
  type UsernameChangeAvailability,
  type UsernameChoiceAvailability,
  type PasswordChangeAvailability,
  type IdentityDisconnectAvailability
} from '@chatto/client/api/actionAvailability';
import { m } from '$lib/i18n/messages';
import { formatCooldownRemaining } from '$lib/validation/login';

/** Explain why an existing account's username cannot change. */
export function usernameChangeUnavailableMessage(action: UsernameChangeAvailability): string {
  if (action.allowed) return '';
  if (action.reason === UsernameChangeReason.PROVIDER_MANAGED)
    return m('settings.account.sso.username_managed');
  if (action.reason === UsernameChangeReason.COOLDOWN) {
    const remaining = action.availableAt ? Date.parse(action.availableAt) - Date.now() : 0;
    if (remaining > 0)
      return m('settings.profile.username.cooldown_notice', {
        remaining: formatCooldownRemaining(remaining)
      });
  }
  return m('common.action_unavailable');
}

/** Signup username choice has no account cooldown or deletion policy. */
export function usernameChoiceUnavailableMessage(action: UsernameChoiceAvailability): string {
  if (action.allowed) return '';
  return action.reason === UsernameChoiceReason.PROVIDER_MANAGED
    ? m('settings.account.sso.username_managed')
    : m('common.action_unavailable');
}

/** Explain password self-service policy; unknown reasons remain generic. */
export function passwordChangeUnavailableMessage(action: PasswordChangeAvailability): string {
  if (action.allowed) return '';
  return action.reason === PasswordChangeReason.DIRECT_LOGIN_DISABLED
    ? m('settings.account.password.sso_managed')
    : m('common.action_unavailable');
}

/** Explain the restriction on this particular linked identity. */
export function identityDisconnectUnavailableMessage(
  action: IdentityDisconnectAvailability,
  provider: string
): string {
  if (action.allowed) return '';
  switch (action.reason) {
    case IdentityDisconnectReason.DISABLED_BY_OPERATOR:
      return m('settings.account.sso.unlink_disabled');
    case IdentityDisconnectReason.LAST_SIGN_IN_METHOD:
      return m('settings.account.sso.disconnect_blocked_modal.body', { provider });
    default:
      return m('common.action_unavailable');
  }
}
