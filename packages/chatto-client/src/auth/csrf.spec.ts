import { afterEach, describe, expect, it, vi } from 'vitest';
import { csrfFetch, csrfHeaders, csrfToken, withCSRFHeaders } from './csrf.js';

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

describe('CSRF token', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('reads the token cookie, also with an encoded value', () => {
    vi.stubGlobal('document', { cookie: 'a=1; chatto_csrf=tok%3Den=; b=2' });
    expect(csrfToken()).toBe('tok=en=');
    expect(csrfHeaders()).toEqual({ 'X-CSRF-Token': 'tok=en=' });
    expect(withCSRFHeaders({ Accept: 'text/plain' }).get('X-CSRF-Token')).toBe('tok=en=');
  });

  it('sends no token without the cookie or outside a browser', () => {
    vi.stubGlobal('document', { cookie: 'a=1' });
    expect(csrfToken()).toBeNull();
    expect(csrfHeaders()).toEqual({});
    expect(withCSRFHeaders().has('X-CSRF-Token')).toBe(false);
    vi.stubGlobal('document', undefined);
    expect(csrfToken()).toBeNull();
  });
});

describe('csrfFetch repair limits', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  const rejected = () =>
    new Response(JSON.stringify({ error: 'CSRF token missing or invalid' }), {
      status: 403,
      headers: { 'Content-Type': 'application/json' }
    });

  it('returns the rejection when the token repair fails', async () => {
    vi.stubGlobal('document', { cookie: '' });
    vi.stubGlobal('location', new URL('https://chatto.example.test/chat'));
    const first = rejected();
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(first)
      .mockResolvedValueOnce(new Response(null, { status: 500 }));
    vi.stubGlobal('fetch', fetchMock);
    await expect(csrfFetch('/auth/browser/logout', { method: 'POST' })).resolves.toBe(first);
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it('replays a Request input with a fresh copy', async () => {
    vi.stubGlobal('document', { cookie: 'chatto_csrf=t' });
    vi.stubGlobal('location', new URL('https://chatto.example.test/chat'));
    const sent: Request[] = [];
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockImplementationOnce(async (input) => {
        sent.push(input as Request);
        return rejected();
      })
      .mockResolvedValueOnce(new Response(null, { status: 200 }))
      .mockImplementationOnce(async (input) => {
        sent.push(input as Request);
        return new Response('{}', { status: 200 });
      });
    vi.stubGlobal('fetch', fetchMock);
    const request = new Request('https://chatto.example.test/auth/browser/logout', {
      method: 'POST',
      body: '{}'
    });
    await expect(csrfFetch(request)).resolves.toMatchObject({ status: 200 });
    expect(sent[1]).not.toBe(sent[0]);
    expect(sent[1]?.url).toBe(request.url);
  });

  it('does not repair outside a page or for an unreadable URL', async () => {
    vi.stubGlobal('document', { cookie: '' });
    const fetchMock = vi.fn<typeof fetch>().mockImplementation(async () => rejected());
    vi.stubGlobal('fetch', fetchMock);
    vi.stubGlobal('location', undefined);
    await csrfFetch('/auth/browser/logout');
    vi.stubGlobal('location', new URL('https://chatto.example.test/chat'));
    await csrfFetch('http://[invalid');
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });
});
