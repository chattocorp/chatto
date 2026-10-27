import { m } from '$lib/i18n/messages';

/** Typed popup outcomes distinguish a deliberate close from a failed sign-in. */
export class OAuthPopupError extends Error {
  constructor(
    message: string,
    readonly reason: 'failed' | 'blocked' | 'cancelled' = 'failed'
  ) {
    super(message);
    this.name = 'OAuthPopupError';
  }
}

/** Return safe UI feedback, or an empty string when the person closed the window. */
export function popupErrorMessage(error: unknown, fallback: string): string {
  if (error instanceof OAuthPopupError) {
    if (error.reason === 'cancelled') return '';
    if (error.reason === 'blocked') return m('auth.popup.blocked');
  }
  return fallback;
}
