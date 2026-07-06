import { redirect } from '@sveltejs/kit';
import type { PageLoad } from './$types';

/**
 * Returns true if `value` is safe to navigate to without validating against
 * a server-side allow-list — i.e. it points to *this* origin.
 *
 * Rejects:
 *   - protocol-relative URLs (`//attacker`)
 *   - backslash variants (`/\\attacker`, which some routers normalize to `//`)
 *   - absolute URLs (`https://...`, `http://...`, `javascript:...`, etc.)
 *   - empty / non-string values
 *
 * Without this check, `?redirect=https://evil.example/` chains with
 * `goto()` / `window.location.href = url` to phish credentials post-login.
 */
function isSafeInternalPath(value: string): boolean {
  return (
    typeof value === 'string' &&
    value.startsWith('/') &&
    !value.startsWith('//') &&
    !value.startsWith('/\\')
  );
}

export const load: PageLoad = async ({ parent, url }) => {
	const { user } = await parent();
	const raw = url.searchParams.get('redirect') ?? '';
	const redirectUrl = isSafeInternalPath(raw) ? raw : '/';

	if (user) {
		redirect(302, redirectUrl.startsWith('/oauth/') ? redirectUrl : '/chat');
	}

	return {
		/** URL to redirect to after login (default: /). Must be a same-origin path. */
		redirectUrl,

		/** Known provider/auth redirect error code to render on the login page. */
		loginErrorCode: url.searchParams.get('error') ?? '',

		/** Whether the user just completed a password reset */
		passwordResetSuccess: url.searchParams.get('reset') === 'success'
	};
};
