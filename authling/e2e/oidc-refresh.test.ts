// Check resource consent, JWT verification, rotation and grant revocation through HTTP.
import { createHash, createPublicKey, verify, type JsonWebKey } from 'node:crypto';
import { test as base, expect } from '@playwright/test';
import { startStack, stopStack, type TestStack } from './fixtures/stack';
import { completeSignup } from './fixtures/signup';

const resource = 'https://cloud.example.invalid';
const test = base.extend<{ stack: TestStack }>({
  stack: async ({}, use, testInfo) => {
    const stack = await startStack(testInfo, {
      additionalConfig: `resources = ['${resource}']\naccess_token_claims = { '${resource}' = ['email'] }\n`
    });
    try {
      await use(stack);
    } finally {
      await stopStack(stack, testInfo);
    }
  },
  baseURL: async ({ stack }, use) => {
    await use(stack.baseURL);
  }
});

test('discloses resource and offline access, rotates tokens, and revokes the grant', async ({
  page,
  request,
  stack
}, testInfo) => {
  await completeSignup(
    page,
    request,
    stack,
    'refresh@example.invalid',
    'a deliberately uncommon password'
  );
  const verifier = 'resource-and-refresh-verifier-with-at-least-forty-three-characters';
  const authorize = new URL('/oauth/authorize', stack.baseURL);
  authorize.search = new URLSearchParams({
    client_id: 'authling-e2e',
    redirect_uri: stack.callbackURL,
    response_type: 'code',
    scope: 'openid email offline_access',
    resource,
    code_challenge: createHash('sha256').update(verifier).digest('base64url'),
    code_challenge_method: 'S256'
  }).toString();
  await page.setViewportSize({ width: 390, height: 720 });
  await page.goto(authorize.toString());
  await expect(page.getByText('API this app can use for you')).toBeVisible();
  await expect(page.getByText('This API also receives your verified email address.')).toBeVisible();
  await expect(page.getByText(resource, { exact: true })).toBeVisible();
  await expect(page.getByText('Access while you are signed out')).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: testInfo.outputPath('resource-consent.png'), fullPage: true });
  await page.getByRole('button', { name: 'Authorize', exact: true }).click();
  await page.waitForURL(`${stack.callbackURL}?**`);
  const code = new URL(page.url()).searchParams.get('code');
  expect(code).toBeTruthy();
  const response = await request.post(`${stack.baseURL}/oauth/token`, {
    form: {
      grant_type: 'authorization_code',
      client_id: 'authling-e2e',
      redirect_uri: stack.callbackURL,
      code: code!,
      code_verifier: verifier,
      resource
    }
  });
  expect(response.ok()).toBe(true);
  const tokens = (await response.json()) as { access_token: string; refresh_token: string };
  const [header, payload, signature] = tokens.access_token.split('.');
  const protectedHeader = JSON.parse(Buffer.from(header, 'base64url').toString()) as {
    typ: string;
    kid: string;
    alg: string;
  };
  expect(protectedHeader.typ).toBe('at+jwt');
  expect(protectedHeader.alg).toBe('RS256');
  const jwks = (await (await request.get(`${stack.baseURL}/oauth/jwks`)).json()) as {
    keys: Array<JsonWebKey & { kid: string }>;
  };
  const key = jwks.keys.find((candidate) => candidate.kid === protectedHeader.kid);
  expect(key).toBeDefined();
  expect(
    verify(
      'RSA-SHA256',
      Buffer.from(`${header}.${payload}`),
      createPublicKey({ key: key!, format: 'jwk' }),
      Buffer.from(signature, 'base64url')
    )
  ).toBe(true);
  const claims = JSON.parse(Buffer.from(payload, 'base64url').toString()) as Record<
    string,
    unknown
  >;
  expect(claims).toMatchObject({
    iss: stack.baseURL,
    aud: [resource],
    client_id: 'authling-e2e',
    email: 'refresh@example.invalid',
    email_verified: true
  });
  const renew = (token: string) =>
    request.post(`${stack.baseURL}/oauth/token`, {
      form: {
        grant_type: 'refresh_token',
        client_id: 'authling-e2e',
        refresh_token: token,
        resource
      }
    });
  const renewed = await renew(tokens.refresh_token);
  expect(renewed.ok()).toBe(true);
  const next = (await renewed.json()) as { refresh_token: string };
  expect(next.refresh_token).not.toBe(tokens.refresh_token);
  await page.goto(`${stack.baseURL}/account/apps`);
  await expect(page.getByText(`API: ${resource}`)).toBeVisible();
  await expect(page.getByText('Access while signed out')).toBeVisible();
  await page.getByRole('button', { name: 'Revoke access' }).click();
  expect((await renew(next.refresh_token)).ok()).toBe(false);
});
