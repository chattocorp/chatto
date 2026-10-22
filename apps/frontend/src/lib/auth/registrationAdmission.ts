import { browserCookieAuthenticationHeaders } from '@chatto/client/auth/authenticationMode';
import { m } from '$lib/i18n/messages';

const selectionKey = 'chatto.registration-admission';

/** Only a tab's selector is readable by JavaScript. The server keeps the
 * invitation binding in its signed HttpOnly session cookie. */
export function rememberRegistrationAdmission(id: string): void {
  try {
    sessionStorage.setItem(selectionKey, id);
  } catch {
    // The current page still has the selector when storage is unavailable.
  }
}

/** Revalidate remembered admission before showing registration choices. */
export async function getRegistrationAdmission(fetcher: typeof fetch): Promise<string> {
  let selection = '';
  try {
    selection = sessionStorage.getItem(selectionKey) ?? '';
  } catch {
    // Server-side loading and restricted browser storage have no tab selector.
  }
  const response = await fetcher(
    `/auth/browser/registration-invitation?admission=${encodeURIComponent(selection)}`
  );
  if (!response.ok) throw new Error(m('auth.register.failed'));
  const body: { admissionId: string } = await response.json();
  rememberRegistrationAdmission(body.admissionId);
  return body.admissionId;
}

/** Validate without consuming, and optionally bind an already verified SSO flow. */
export async function acceptRegistrationInvitation(code: string, externalIdentityToken: string) {
  const response = await fetch('/auth/browser/registration-invitation', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', ...browserCookieAuthenticationHeaders },
    body: JSON.stringify({ code, externalIdentityToken })
  });
  if (response.status === 410) throw new Error(m('auth.register.complete.invalid_text'));
  if (response.status === 400) return null;
  if (!response.ok) throw new Error(m('auth.register.failed'));
  const body: { admissionId: string; externalIdentityToken: string } = await response.json();
  rememberRegistrationAdmission(body.admissionId);
  return body;
}
