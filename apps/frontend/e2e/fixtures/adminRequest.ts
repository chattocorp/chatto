import { expect, request, type APIRequestContext } from '@playwright/test';

interface GraphQLResponse<T> {
  data?: T;
  errors?: unknown[];
}

export async function createBootstrapAdminRequest(baseURL: string): Promise<APIRequestContext> {
  const adminRequest = await request.newContext({ baseURL });
  const loginResponse = await adminRequest.post('/auth/login', {
    data: { login: 'e2eadmin', password: 'adminpassword123' }
  });
  expect(loginResponse.ok()).toBeTruthy();
  return adminRequest;
}

export async function adminGraphql<T>(
  adminRequest: APIRequestContext,
  query: string,
  variables?: Record<string, unknown>
): Promise<T> {
  const response = await adminRequest.post('/api/graphql', {
    headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
    data: { query, variables }
  });
  expect(response.ok()).toBeTruthy();
  const json = (await response.json()) as GraphQLResponse<T>;
  if (json.errors) throw new Error(JSON.stringify(json.errors));
  if (!json.data) throw new Error('GraphQL response contained no data');
  return json.data;
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
