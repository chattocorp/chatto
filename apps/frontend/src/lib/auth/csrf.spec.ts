// SPDX-License-Identifier: Apache-2.0

import { afterEach, describe, expect, it, vi } from 'vitest';
import { csrfFetch } from './csrf';

describe('csrfFetch', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('repairs an expired bound token once and replays the original request', async () => {
    const browserDocument = { cookie: 'chatto_csrf=old-token' };
    vi.stubGlobal('document', browserDocument);
    vi.stubGlobal('location', new URL('https://chatto.example.test/chat'));
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockImplementationOnce(async (request, init) => {
        expect(request).toBe('https://chatto.example.test/auth/browser/logout');
        expect(new Headers(init?.headers).get('X-CSRF-Token')).toBe('old-token');
        expect(init?.body).toBe('{"value":1}');
        return new Response(JSON.stringify({ error: 'CSRF token missing or invalid' }), {
          status: 403,
          headers: { 'Content-Type': 'application/json' }
        });
      })
      .mockImplementationOnce(async (request) => {
        expect(request).toBe('/auth/browser/csrf');
        browserDocument.cookie = 'chatto_csrf=new-token';
        return new Response(null, { status: 200 });
      })
      .mockImplementationOnce(async (request, init) => {
        expect(request).toBe('https://chatto.example.test/auth/browser/logout');
        expect(new Headers(init?.headers).get('X-CSRF-Token')).toBe('new-token');
        expect(init?.body).toBe('{"value":1}');
        return new Response(null, { status: 204 });
      });
    vi.stubGlobal('fetch', fetchMock);

    const response = await csrfFetch('https://chatto.example.test/auth/browser/logout', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: '{"value":1}'
    });

    expect(response.status).toBe(204);
    expect(fetchMock).toHaveBeenCalledTimes(3);
  });

  it('does not replay an unrelated forbidden response', async () => {
    vi.stubGlobal('document', { cookie: '' });
    vi.stubGlobal('location', new URL('https://chatto.example.test/chat'));
    const response = new Response(JSON.stringify({ error: 'Permission denied' }), {
      status: 403,
      headers: { 'Content-Type': 'application/json' }
    });
    const fetchMock = vi.fn<typeof fetch>().mockResolvedValue(response);
    vi.stubGlobal('fetch', fetchMock);

    expect(await csrfFetch('https://chatto.example.test/protected', { method: 'POST' })).toBe(
      response
    );
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it('does not repair or replay a cross-origin CSRF-shaped response', async () => {
    vi.stubGlobal('document', { cookie: 'chatto_csrf=local-token' });
    vi.stubGlobal('location', new URL('https://chatto.example.test/chat'));
    const response = new Response(JSON.stringify({ error: 'CSRF token missing or invalid' }), {
      status: 403,
      headers: { 'Content-Type': 'application/json' }
    });
    const fetchMock = vi.fn<typeof fetch>().mockResolvedValue(response);
    vi.stubGlobal('fetch', fetchMock);

    expect(await csrfFetch('https://remote.example.test/protected', { method: 'POST' })).toBe(
      response
    );
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });
});
