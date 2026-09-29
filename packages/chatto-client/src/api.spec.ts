// @vitest-environment node
import { expect, it, vi } from 'vitest';
import { ViewerService } from '@chatto/api-types/api/v1/viewer_connect';
import { createApi } from './api.js';

it('sends Connect JSON with the bearer token and rejects redirects', async () => {
  const request = vi.fn<typeof fetch>(async () =>
    Response.json({ user: { profile: { id: 'bot' } } })
  );
  const api = createApi({
    serverUrl: 'https://chat.example/x',
    apiKey: 'key',
    fetch: request
  });
  const response = await api.service(ViewerService).getViewer({});
  expect(response.user?.profile?.id).toBe('bot');
  const [url, init] = request.mock.calls[0]!;
  expect(String(url)).toBe(
    'https://chat.example/api/connect/chatto.api.v1.ViewerService/GetViewer'
  );
  expect(init?.redirect).toBe('error');
  expect(new Headers(init?.headers).get('Authorization')).toBe('Bearer key');
  expect(new Headers(init?.headers).get('Content-Type')).toBe('application/json');
});

it('rejects server URLs with credentials and empty keys', () => {
  expect(() => createApi({ serverUrl: 'https://a:b@chat.example', apiKey: 'k' })).toThrow();
  expect(() => createApi({ serverUrl: 'https://chat.example', apiKey: '' })).toThrow();
});

it('reads the viewer once and reads it again after a failure', async () => {
  let fail = true;
  const request = vi.fn<typeof fetch>(async () =>
    fail
      ? Response.json({ code: 'unavailable', message: 'down' }, { status: 503 })
      : Response.json({ user: { profile: { id: 'bot' } } })
  );
  const api = createApi({ serverUrl: 'https://chat.example', apiKey: 'key', fetch: request });
  await expect(api.viewerId()).rejects.toThrow();
  fail = false;
  await expect(api.viewerId()).resolves.toBe('bot');
  await expect(api.viewerId()).resolves.toBe('bot');
  expect(request).toHaveBeenCalledTimes(2);
});
