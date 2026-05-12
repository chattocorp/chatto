/**
 * PKCE (Proof Key for Code Exchange) utilities for OAuth 2.0 public clients.
 * Implements RFC 7636 with S256 challenge method only.
 */

const VERIFIER_LENGTH = 64;
const VERIFIER_CHARSET = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-._~';

/** Generate a cryptographically random code verifier (43-128 chars, RFC 7636). */
export function generateCodeVerifier(): string {
	const bytes = crypto.getRandomValues(new Uint8Array(VERIFIER_LENGTH));
	return Array.from(bytes)
		.map((b) => VERIFIER_CHARSET[b % VERIFIER_CHARSET.length])
		.join('');
}

/** Compute the S256 code challenge: base64url(SHA-256(verifier)). */
export async function generateCodeChallenge(verifier: string): Promise<string> {
	const encoder = new TextEncoder();
	const data = encoder.encode(verifier);
	const hash = await crypto.subtle.digest('SHA-256', data);
	return base64UrlEncode(new Uint8Array(hash));
}

function base64UrlEncode(bytes: Uint8Array): string {
	const binString = Array.from(bytes)
		.map((b) => String.fromCharCode(b))
		.join('');
	return btoa(binString).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
}

/** Generate a random state parameter for CSRF protection. */
export function generateState(): string {
	const bytes = crypto.getRandomValues(new Uint8Array(32));
	return base64UrlEncode(bytes);
}

// Session storage keys for the OAuth flow (tab-scoped, survives redirects)
const STORAGE_PREFIX = 'chatto:oauth:';

export interface OAuthFlowState {
	verifier: string;
	state: string;
	remoteUrl: string;
	serverName: string;
	serverIconUrl: string | null;
}

/** Save OAuth flow state to sessionStorage before redirecting. */
export function saveFlowState(flow: OAuthFlowState): void {
	sessionStorage.setItem(STORAGE_PREFIX + 'flow', JSON.stringify(flow));
}

/** Load and clear OAuth flow state from sessionStorage after redirect. */
export function loadAndClearFlowState(): OAuthFlowState | null {
	const key = STORAGE_PREFIX + 'flow';
	const raw = sessionStorage.getItem(key);
	if (!raw) return null;
	sessionStorage.removeItem(key);
	try {
		return JSON.parse(raw) as OAuthFlowState;
	} catch {
		return null;
	}
}
