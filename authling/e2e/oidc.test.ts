import { createHash, randomUUID } from 'node:crypto';
import { completeSignup } from './fixtures/signup';
import { expect, test } from './setup';

const password = 'correct horse battery staple';
test('completes a conventional OIDC Authorization Code flow', async ({ page, request, stack }) => {
  const accountID = await completeSignup(
    page,
    request,
    stack,
    `oidc-${randomUUID()}@example.invalid`,
    password
  );

  const discoveryResponse = await request.get(`${stack.baseURL}/.well-known/openid-configuration`);
  expect(discoveryResponse.ok()).toBe(true);
  const discovery = (await discoveryResponse.json()) as Record<string, unknown>;
  expect(discovery).toMatchObject({
    issuer: stack.baseURL,
    authorization_endpoint: `${stack.baseURL}/oauth/authorize`,
    token_endpoint: `${stack.baseURL}/oauth/token`,
    userinfo_endpoint: `${stack.baseURL}/oauth/userinfo`,
    jwks_uri: `${stack.baseURL}/oauth/jwks`,
    scopes_supported: ['openid', 'account_data'],
    code_challenge_methods_supported: ['S256'],
    response_types_supported: ['code'],
    grant_types_supported: ['authorization_code'],
    client_id_metadata_document_supported: true
  });
  expect(discovery).not.toHaveProperty('registration_endpoint');
  expect(discovery).not.toHaveProperty('revocation_endpoint');

  const verifier = 'playwright-verifier-with-at-least-forty-three-characters';
  const challenge = createHash('sha256').update(verifier).digest('base64url');
  const redirectURI = stack.callbackURL;
  const authorize = new URL('/oauth/authorize', stack.baseURL);
  authorize.search = new URLSearchParams({
    client_id: 'authling-e2e',
    redirect_uri: redirectURI,
    response_type: 'code',
    scope: 'openid',
    state: 'browser-state',
    nonce: 'browser-nonce',
    code_challenge: challenge,
    code_challenge_method: 'S256'
  }).toString();

  await page.goto(authorize.toString());
  await expect(page.getByRole('heading', { name: 'Authorize Authling E2E client?' })).toBeVisible();
  await expect(page.getByText('configured by this Authling operator', { exact: false })).toBeVisible();

  const callbackRequest = page.waitForRequest((request) =>
    request.url().startsWith(`${stack.callbackURL}?`)
  );
  await page.getByRole('button', { name: 'Authorize' }).click();
  const callback = new URL((await callbackRequest).url());
  expect(callback.searchParams.get('state')).toBe('browser-state');
  const code = callback.searchParams.get('code');
  expect(code).not.toBeNull();

  const tokenResponse = await request.post(`${stack.baseURL}/oauth/token`, {
    form: {
      grant_type: 'authorization_code',
      client_id: 'authling-e2e',
      redirect_uri: redirectURI,
      code: code ?? '',
      code_verifier: verifier
    }
  });
  expect(tokenResponse.status()).toBe(200);
  const tokens = (await tokenResponse.json()) as {
    access_token: string;
    id_token: string;
    token_type: string;
  };
  expect(tokens.token_type).toBe('Bearer');
  const claims = JSON.parse(Buffer.from(tokens.id_token.split('.')[1], 'base64url').toString()) as Record<string, unknown>;
  expect(claims).toMatchObject({ iss: stack.baseURL, sub: accountID, azp: 'authling-e2e', nonce: 'browser-nonce' });

  const userinfo = await request.get(`${stack.baseURL}/oauth/userinfo`, {
    headers: { Authorization: `Bearer ${tokens.access_token}` }
  });
  expect(userinfo.status()).toBe(200);
  expect(await userinfo.json()).toEqual({ sub: accountID });

  const reused = await request.post(`${stack.baseURL}/oauth/token`, {
    form: {
      grant_type: 'authorization_code', client_id: 'authling-e2e', redirect_uri: redirectURI,
      code: code ?? '', code_verifier: verifier
    }
  });
  expect(reused.status()).toBe(400);
  expect(await reused.json()).toMatchObject({ error: 'invalid_grant' });
});

test('rejects authorization without S256 PKCE before starting consent', async ({ request, stack }) => {
  const response = await request.get(`${stack.baseURL}/oauth/authorize`, {
    params: { client_id: 'authling-e2e', redirect_uri: stack.callbackURL, response_type: 'code', scope: 'openid' },
    maxRedirects: 0
  });
  expect(response.status()).toBe(400);
  expect(await response.text()).toBe('invalid authorization request\n');
});
