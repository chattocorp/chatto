import { createHash, randomUUID } from 'node:crypto';
import { waitForPasswordResetCode } from './fixtures/mailpit';
import { completeSignup } from './fixtures/signup';
import { expect, test } from './setup';

const password = 'correct horse battery staple';
test('completes a conventional OIDC Authorization Code flow', async ({ page, request, stack }) => {
  const email = `oidc-${randomUUID()}@example.invalid`;
  const preferredUsername = `profile-${randomUUID()}`;
  const fullName = 'OIDC Profile Person';
  const accountID = await completeSignup(
    page,
    request,
    stack,
    email,
    password
  );
  await page.getByRole('link', { name: 'Edit profile' }).click();
  await page.getByLabel('Preferred username').fill(preferredUsername);
  await page.getByLabel('Full name').fill(fullName);
  await page.getByRole('button', { name: 'Save profile' }).click();
  await expect(page.getByText('Your profile was updated.')).toBeVisible();

  const discoveryResponse = await request.get(`${stack.baseURL}/.well-known/openid-configuration`);
  expect(discoveryResponse.ok()).toBe(true);
  const discovery = (await discoveryResponse.json()) as Record<string, unknown>;
  expect(discovery).toMatchObject({
    issuer: stack.baseURL,
    authorization_endpoint: `${stack.baseURL}/oauth/authorize`,
    token_endpoint: `${stack.baseURL}/oauth/token`,
    userinfo_endpoint: `${stack.baseURL}/oauth/userinfo`,
    jwks_uri: `${stack.baseURL}/oauth/jwks`,
    scopes_supported: ['openid'],
    code_challenge_methods_supported: ['S256'],
    response_types_supported: ['code'],
    grant_types_supported: ['authorization_code'],
    client_id_metadata_document_supported: true
  });
  expect(discovery.claims_supported).toEqual(['sub', 'preferred_username', 'name', 'auth_time']);
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
  await expect(page.getByText('Signed in as')).toBeVisible();
  await expect(page.getByText(email)).toBeVisible();
  await expect(page.getByText('configured by this Authling operator', { exact: false })).toBeVisible();

  await expect(page.getByText(accountID, { exact: true })).toBeVisible();
  await expect(page.getByText(preferredUsername, { exact: true })).toBeVisible();
  await expect(page.getByText(fullName, { exact: true })).toBeVisible();
  await expect(page.getByText('This access includes future changes to your username and full name.', { exact: false })).toBeVisible();

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
  expect(claims).toMatchObject({ iss: stack.baseURL, sub: accountID, azp: 'authling-e2e', nonce: 'browser-nonce', preferred_username: preferredUsername, name: fullName });

  const userinfo = await request.get(`${stack.baseURL}/oauth/userinfo`, {
    headers: { Authorization: `Bearer ${tokens.access_token}` }
  });
  expect(userinfo.status()).toBe(200);
  expect(await userinfo.json()).toEqual({ sub: accountID, preferred_username: preferredUsername, name: fullName });

  // The disclosure covers live UserInfo reads as well as later sign-ins.
  await page.goto(`${stack.baseURL}/account/profile`);
  await page.getByLabel('Preferred username').fill('updated-username');
  await page.getByLabel('Full name').fill('Updated Profile Person');
  await page.getByRole('button', { name: 'Save profile' }).click();
  const updatedUserinfo = await request.get(`${stack.baseURL}/oauth/userinfo`, {
    headers: { Authorization: `Bearer ${tokens.access_token}` }
  });
  expect(updatedUserinfo.status()).toBe(200);
  expect(await updatedUserinfo.json()).toEqual({ sub: accountID, preferred_username: 'updated-username', name: 'Updated Profile Person' });

  await page.goto(`${stack.baseURL}/account`);
  const authorizedApps = page.getByRole('heading', { name: 'Authorized apps' }).locator('..');
  await expect(authorizedApps.getByText('Authling E2E client')).toBeVisible();
  await expect(authorizedApps.getByText('configured by this Authling operator', { exact: false })).toBeVisible();

  await page.goto(authorize.toString());
  await expect(page).toHaveURL(new RegExp(`^${escapeRegExp(stack.callbackURL)}\\?`));

  const forcedConsent = new URL(authorize);
  forcedConsent.searchParams.set('prompt', 'consent');
  await page.goto(forcedConsent.toString());
  await expect(page.getByRole('heading', { name: 'Authorize Authling E2E client?' })).toBeVisible();

  await page.goto(`${stack.baseURL}/account`);
  await authorizedApps.getByRole('button', { name: 'Revoke access' }).click();
  await expect(page.getByText('The app’s authorization was revoked', { exact: false })).toBeVisible();
  await expect(authorizedApps.getByText('You haven’t authorized any apps yet.')).toBeVisible();

  await page.goto(authorize.toString());
  await expect(page.getByRole('heading', { name: 'Authorize Authling E2E client?' })).toBeVisible();

  const reused = await request.post(`${stack.baseURL}/oauth/token`, {
    form: {
      grant_type: 'authorization_code', client_id: 'authling-e2e', redirect_uri: redirectURI,
      code: code ?? '', code_verifier: verifier
    }
  });
  expect(reused.status()).toBe(400);
  expect(await reused.json()).toMatchObject({ error: 'invalid_grant' });
});

function escapeRegExp(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

test('returns a missing-PKCE error to the validated client before consent', async ({ request, stack }) => {
  const state = randomUUID();
  const response = await request.get(`${stack.baseURL}/oauth/authorize`, {
    params: { client_id: 'authling-e2e', redirect_uri: stack.callbackURL, response_type: 'code', scope: 'openid', state },
    maxRedirects: 0
  });
  expect(response.status()).toBe(302);
  const location = new URL(response.headers().location ?? '');
  expect(`${location.origin}${location.pathname}`).toBe(stack.callbackURL);
  expect(location.searchParams.get('error')).toBe('invalid_request');
  expect(location.searchParams.get('state')).toBe(state);
});

for (const { freshness, recover } of [
  { freshness: { prompt: 'login' }, recover: false },
  { freshness: { max_age: '0' }, recover: false },
  { freshness: { prompt: 'login consent' }, recover: false },
  { freshness: { max_age: '0' }, recover: true }
]) {
  test(`requires fresh authentication for ${JSON.stringify(freshness)} (recovery=${recover}) and preserves consent`, async ({ page, request, stack }) => {
    const email = `freshness-${randomUUID()}@example.invalid`;
    await completeSignup(page, request, stack, email, password);
    const verifier = 'playwright-freshness-verifier-with-at-least-forty-three-characters';
    const authorize = new URL('/oauth/authorize', stack.baseURL);
    authorize.search = new URLSearchParams({
      client_id: 'authling-e2e', redirect_uri: stack.callbackURL, response_type: 'code', scope: 'openid',
      code_challenge: createHash('sha256').update(verifier).digest('base64url'), code_challenge_method: 'S256'
    }).toString();
    // Establish a durable grant before asking for fresh authentication.
    await page.goto(authorize.toString());
    await page.getByRole('button', { name: 'Authorize', exact: true }).click();
    await expect(page).toHaveURL(new RegExp(`^${escapeRegExp(stack.callbackURL)}\\?`));
    for (const [key, value] of Object.entries(freshness)) authorize.searchParams.set(key, value);
    await page.goto(authorize.toString());
    await expect(page).toHaveURL(/\/login\?id=/);
    const pendingID = new URL(page.url()).searchParams.get('id');
    let loginStart = Math.floor(Date.now() / 1000);
    if (recover) {
      await page.getByRole('link', { name: 'Forgot your password?' }).click();
      await page.getByLabel('Email address').fill(email);
      await page.getByRole('button', { name: 'Email me a reset code' }).click();
      await page.getByLabel('Password reset code').fill(await waitForPasswordResetCode(request, stack.mailpitURL));
      await page.getByRole('button', { name: 'Verify code' }).click();
      await page.getByLabel('New password').fill('a new uncommon recovery password');
      await page.getByRole('button', { name: 'Reset password' }).click();
    } else {
      await page.getByLabel('Email address').fill(email);
      await page.getByLabel('Password', { exact: true }).fill('wrong password');
      await page.getByRole('button', { name: 'Sign in', exact: true }).click();
      await expect(page.getByText('The email address or password is incorrect.')).toBeVisible();
      await expect(page.locator('input[name="oidc_request"]')).toHaveValue(pendingID ?? '');
      loginStart = Math.floor(Date.now() / 1000);
      await page.getByLabel('Email address').fill(email);
      await page.getByLabel('Password', { exact: true }).fill(password);
      await page.getByRole('button', { name: 'Sign in', exact: true }).click();
    }
    if (freshness.prompt?.includes('consent')) {
      await expect(page.getByRole('heading', { name: 'Authorize Authling E2E client?' })).toBeVisible();
      await page.getByRole('button', { name: 'Authorize', exact: true }).click();
    }
    await expect(page).toHaveURL(new RegExp(`^${escapeRegExp(stack.callbackURL)}\\?`));
    const code = new URL(page.url()).searchParams.get('code');
    const response = await request.post(`${stack.baseURL}/oauth/token`, { form: {
      grant_type: 'authorization_code', client_id: 'authling-e2e', redirect_uri: stack.callbackURL,
      code: code ?? '', code_verifier: verifier
    } });
    expect(response.status()).toBe(200);
    const tokens = await response.json() as { id_token: string };
    const claims = JSON.parse(Buffer.from(tokens.id_token.split('.')[1], 'base64url').toString()) as { auth_time: number };
    expect(claims.auth_time).toBeGreaterThanOrEqual(loginStart);
    expect(claims.auth_time).toBeLessThanOrEqual(Math.floor(Date.now() / 1000));
    // The new session can satisfy a positive max_age without another login.
    authorize.searchParams.delete('prompt');
    authorize.searchParams.set('max_age', '60');
    await page.goto(authorize.toString());
    await expect(page).toHaveURL(new RegExp(`^${escapeRegExp(stack.callbackURL)}\\?`));
  });
}

test('rechecks max_age when consent is submitted after authentication expires', async ({ page, request, stack }) => {
  const email = `expired-consent-${randomUUID()}@example.invalid`;
  await completeSignup(page, request, stack, email, password);
  const authorize = new URL('/oauth/authorize', stack.baseURL);
  authorize.search = new URLSearchParams({
    client_id: 'authling-e2e', redirect_uri: stack.callbackURL, response_type: 'code', scope: 'openid', max_age: '5',
    code_challenge: createHash('sha256').update('playwright-delayed-consent-verifier-at-least-forty-three-characters').digest('base64url'),
    code_challenge_method: 'S256'
  }).toString();
  await page.goto(authorize.toString());
  await expect(page.getByRole('heading', { name: 'Authorize Authling E2E client?' })).toBeVisible();
  const consentID = new URL(page.url()).searchParams.get('id');
  // Real server time governs freshness; a browser clock mock cannot exercise it.
  const expiresBefore = Date.now() + 5100;
  await expect.poll(() => Date.now(), { timeout: 8000 }).toBeGreaterThan(expiresBefore);
  await page.getByRole('button', { name: 'Authorize', exact: true }).click();
  await expect(page).toHaveURL(/\/login\?id=/);
  expect(new URL(page.url()).searchParams.get('id')).toBe(consentID);
  await page.getByLabel('Email address').fill(email);
  await page.getByLabel('Password', { exact: true }).fill(password);
  await page.getByRole('button', { name: 'Sign in', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Authorize Authling E2E client?' })).toBeVisible();
  await page.getByRole('button', { name: 'Authorize', exact: true }).click();
  await expect(page).toHaveURL(new RegExp(`^${escapeRegExp(stack.callbackURL)}\\?`));
});
