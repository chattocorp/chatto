/** Action-specific notices. Shared wording does not make reasons interchangeable. */
import {
  UsernameChangeState,
  UsernameChoiceAvailability,
  PasswordChangeAvailability,
  DisconnectEligibility,
  type UsernameChangeAvailability,
  canChangeUsername
} from '@chatto/client/api/actionAvailability';
import { m } from '$lib/i18n/messages';
import { formatCooldownRemaining } from '$lib/validation/login';

/** Explain why an existing account's username cannot change. */
export function usernameChangeUnavailableMessage(
  action: UsernameChangeAvailability,
  now = Date.now()
): string {
  if (canChangeUsername(action, now)) return '';
  if (action.state === UsernameChangeState.PROVIDER_MANAGED)
    return m('settings.account.sso.username_managed');
  if (action.state === UsernameChangeState.ALLOWED) {
    const remaining = action.availableAt ? Date.parse(action.availableAt) - now : 0;
    if (remaining > 0)
      return m('settings.profile.username.cooldown_notice', {
        remaining: formatCooldownRemaining(remaining)
      });
  }
  return m('common.action_unavailable');
}

/** Signup username choice has no account cooldown or deletion policy. */
export function usernameChoiceUnavailableMessage(action: UsernameChoiceAvailability): string {
  if (action === UsernameChoiceAvailability.ALLOWED) return '';
  return action === UsernameChoiceAvailability.PROVIDER_MANAGED
    ? m('settings.account.sso.username_managed')
    : m('common.action_unavailable');
}

/** Explain password self-service policy; unknown reasons remain generic. */
export function passwordChangeUnavailableMessage(action: PasswordChangeAvailability): string {
  if (action === PasswordChangeAvailability.ALLOWED) return '';
  return action === PasswordChangeAvailability.DIRECT_LOGIN_DISABLED
    ? m('settings.account.password.sso_managed')
    : m('common.action_unavailable');
}

/** Explain the restriction on this particular linked identity. */
export function identityDisconnectUnavailableMessage(
  action: DisconnectEligibility,
  provider: string
): string {
  if (action === DisconnectEligibility.ALLOWED) return '';
  switch (action) {
    case DisconnectEligibility.DISABLED_BY_OPERATOR:
      return m('settings.account.sso.unlink_disabled');
    case DisconnectEligibility.LAST_ENABLED_SIGN_IN_METHOD:
      return m('settings.account.sso.disconnect_blocked_modal.body', { provider });
    default:
      return m('common.action_unavailable');
  }
}
