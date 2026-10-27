// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: Apache-2.0

/** Submit a password only to the selected server. Never follow redirects with credentials. */
export async function passwordLogin(request, fetchRequest = fetch) {
  const { serverUrl, identifier, password } = request ?? {};
  if (
    typeof serverUrl !== 'string' ||
    typeof identifier !== 'string' ||
    !identifier.trim() ||
    identifier.length > 1024 ||
    typeof password !== 'string' ||
    !password ||
    password.length > 4096
  )
    throw new Error('Invalid sign-in request.');
  let url;
  try {
    url = new URL(serverUrl);
  } catch {
    throw new Error('Invalid server address.');
  }
  const loopback =
    ['localhost', '127.0.0.1', '[::1]'].includes(url.hostname) ||
    url.hostname.endsWith('.localhost');
  if (
    (url.protocol !== 'https:' && !(url.protocol === 'http:' && loopback)) ||
    url.username ||
    url.password ||
    url.search ||
    url.hash ||
    url.pathname !== '/'
  )
    throw new Error('Sign-in requires an HTTPS server or a local development server.');
  url.pathname = '/auth/login';
  try {
    const response = await fetchRequest(url.href, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ identifier, password }),
      redirect: 'error',
      credentials: 'omit',
      signal: AbortSignal.timeout(15_000)
    });
    // Do not relay arbitrary server error text, which can contain submitted credentials.
    if (!response.ok) return { ok: false };
    return { ok: true, session: await response.json() };
  } catch {
    return { ok: false };
  }
}
