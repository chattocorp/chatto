import { expect, request, type APIRequestContext } from '@playwright/test';

export async function createBootstrapAdminRequest(baseURL: string): Promise<APIRequestContext> {
  const adminRequest = await request.newContext({ baseURL });
  const loginResponse = await adminRequest.post('/auth/login', {
    data: { login: 'e2eadmin', password: 'adminpassword123' }
  });
  expect(loginResponse.ok()).toBeTruthy();
  return adminRequest;
}

export async function withBootstrapAdminRequest<T>(
  baseURL: string,
  run: (adminRequest: APIRequestContext) => Promise<T>
): Promise<T> {
  const adminRequest = await createBootstrapAdminRequest(baseURL);

  try {
    return await run(adminRequest);
  } finally {
    await adminRequest.dispose();
  }
}
