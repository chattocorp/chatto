const CSRF_COOKIE_NAME = 'chatto_csrf';
export const CSRF_HEADER_NAME = 'X-CSRF-Token';

export function csrfToken(): string | null {
	if (typeof document === 'undefined') return null;

	for (const cookie of document.cookie.split(';')) {
		const [rawName, ...valueParts] = cookie.trim().split('=');
		if (rawName === CSRF_COOKIE_NAME) {
			return decodeURIComponent(valueParts.join('='));
		}
	}
	return null;
}

export function csrfHeaders(): Record<string, string> {
	const token = csrfToken();
	return token ? { [CSRF_HEADER_NAME]: token } : {};
}

export function withCSRFHeaders(headers?: HeadersInit): Headers {
	const merged = new Headers(headers);
	const token = csrfToken();
	if (token) {
		merged.set(CSRF_HEADER_NAME, token);
	}
	return merged;
}

export function csrfFetch(input: RequestInfo | URL, init: RequestInit = {}): Promise<Response> {
	return fetch(input, {
		...init,
		headers: withCSRFHeaders(init.headers)
	});
}
