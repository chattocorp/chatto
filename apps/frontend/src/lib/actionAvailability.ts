/** Presentation of shared action decisions; policy belongs to the server/client. */
import {
  ActionUnavailableReason,
  type ActionAvailability
} from '@chatto/client/api/actionAvailability';
import { m } from '$lib/i18n/messages';
import { formatCooldownRemaining } from '$lib/validation/login';

export function actionUnavailableMessage(
  action: ActionAvailability,
  context: 'username' | 'password' | 'disconnect' | 'account',
  provider = ''
): string {
  if (action.allowed) return '';
  switch (action.reason) {
    case ActionUnavailableReason.EXTERNALLY_MANAGED:
      if (context === 'username') return m('settings.account.sso.username_managed');
      break;
    case ActionUnavailableReason.DISABLED_BY_OPERATOR:
      if (context === 'password') return m('settings.account.password.sso_managed');
      if (context === 'disconnect') return m('settings.account.sso.unlink_disabled');
      break;
    case ActionUnavailableReason.PERMISSION_DENIED:
      return m('common.error.permission_denied');
    case ActionUnavailableReason.LAST_SIGN_IN_METHOD:
      return m('settings.account.sso.disconnect_blocked_modal.body', { provider });
    case ActionUnavailableReason.COOLDOWN: {
      const remaining = action.availableAt ? Date.parse(action.availableAt) - Date.now() : 0;
      if (context === 'username' && remaining > 0) {
        return m('settings.profile.username.cooldown_notice', {
          remaining: formatCooldownRemaining(remaining)
        });
      }
      break;
    }
  }
  return m('common.action_unavailable');
}
