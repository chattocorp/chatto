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

async function isRejectedCSRFResponse(response: Response): Promise<boolean> {
  if (response.status !== 403) return false;
  const body = await response
    .clone()
    .json()
    .catch(() => null);
  return body?.error === 'CSRF token missing or invalid';
}

function isSameOriginRequest(input: RequestInfo | URL): boolean {
  if (typeof location === 'undefined') return false;
  try {
    const url =
      input instanceof Request ? new URL(input.url) : new URL(input.toString(), location.href);
    return url.origin === location.origin;
  } catch {
    return false;
  }
}

export async function csrfFetch(
  input: RequestInfo | URL,
  init: RequestInit = {}
): Promise<Response> {
  const retryInput = input instanceof Request ? input.clone() : input;
  const send = (requestInput: RequestInfo | URL) =>
    fetch(requestInput, {
      ...init,
      headers: withCSRFHeaders(init.headers)
    });
  const response = await send(input);
  if (!isSameOriginRequest(input) || !(await isRejectedCSRFResponse(response))) return response;

  // The authentication cookie can outlive a browser-readable CSRF cookie
  // after automatic session renewal. Repair the bound token once, then replay
  // the original request with its newly issued value.
  const repair = await fetch('/auth/browser/csrf', { method: 'GET' });
  if (!repair.ok) return response;
  return send(retryInput);
}
