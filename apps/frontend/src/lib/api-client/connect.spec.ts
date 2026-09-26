// SPDX-License-Identifier: Apache-2.0

import { Code, ConnectError, createContextValues } from '@connectrpc/connect';
import { RoomService } from '@chatto/api-types/api/v1/rooms_connect';
import { describe, expect, it, vi } from 'vitest';
import {
  authenticationRequiredInterceptor,
  bearerRenewalInterceptor,
  createChattoClient,
  dataGenerationInterceptor,
  minimumCursorHeaders,
  privateRequestInterceptor,
  skipAuthenticationRequired,
  StaleResponseError
} from './connect';
import { configureApiClientHooks } from './hooks';

describe('saved-view private request boundary', () => {
  it('allows viewer verification and gates other private calls', async () => {
    const beforeRequest = vi.fn(async () => {});
    const next = vi.fn(async () => ({ message: {} }));
    const invoke = privateRequestInterceptor(beforeRequest)(next as never);
    const signal = new AbortController().signal;

    await invoke({ service: { typeName: 'chatto.api.v1.ViewerService' },
      method: { name: 'GetViewer' }, signal } as never);
    expect(beforeRequest).not.toHaveBeenCalled();

    await invoke({ service: { typeName: 'chatto.api.v1.RoomService' },
      method: { name: 'ListMembers' }, signal } as never);
    expect(beforeRequest).toHaveBeenCalledOnce();
    expect(beforeRequest).toHaveBeenCalledWith('ListMembers', signal);
  });

  it('records the data generation after viewer verification releases a private read', async () => {
    let generation = 0;
    let release!: () => void;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    const beforeRequest = vi.fn(async () => gate);
    const fetch = vi.fn(async () =>
      new Response(new Uint8Array(), {
        status: 200,
        headers: { 'Content-Type': 'application/proto' }
      })
    );
    vi.stubGlobal('fetch', fetch);
    try {
      const client = createChattoClient(RoomService, {
        baseUrl: 'http://localhost:1234/api/connect',
        bearerToken: null,
        dataGeneration: () => generation,
        beforePrivateRequest: beforeRequest
      });
      const result = client.listMembers({ roomId: 'room' });
      await vi.waitFor(() => expect(beforeRequest).toHaveBeenCalledOnce());
      generation++;
      release();
      await expect(result).resolves.toBeDefined();
      expect(fetch).toHaveBeenCalledOnce();
    } finally {
      vi.unstubAllGlobals();
    }
  });
});

describe('private data response boundary', () => {
  it('discards a delayed read while allowing another connection to finish', async () => {
    let generation = 0;
    let finish!: (value: unknown) => void;
    const next = vi.fn(
      () =>
        new Promise((resolve) => {
          finish = resolve;
        })
    );
    const request = { method: { name: 'GetMember' } };
    const result = dataGenerationInterceptor(() => generation)(next as never)(request as never);
    const rejected = expect(result).rejects.toMatchObject({ mutationSucceeded: false });
    generation++;
    finish({ message: { secret: 'old' } });
    await rejected;
    const response = { message: { current: true } };
    await expect(
      dataGenerationInterceptor(() => 0)(vi.fn().mockResolvedValue(response))(request as never)
    ).resolves.toBe(response);
  });

  it('reports obsolete mutation success without exposing its response body', async () => {
    let generation = 0;
    const next = vi.fn(async () => {
      generation++;
      return { message: { secret: 'old' } };
    });
    await expect(
      dataGenerationInterceptor(() => generation)(next as never)({
        method: { name: 'CreateRole' }
      } as never)
    ).rejects.toEqual(new StaleResponseError(true));
  });
});

describe('bearerRenewalInterceptor', () => {
  it('rotates and retries one unauthenticated unary request with the new token', async () => {
    const renewBearerToken = vi
      .fn<(force: boolean) => Promise<string | null>>()
      .mockResolvedValueOnce('access-1')
      .mockResolvedValueOnce('access-2');
    const response = { stream: false };
    const next = vi
      .fn()
      .mockRejectedValueOnce(new ConnectError('expired', Code.Unauthenticated))
      .mockResolvedValueOnce(response);
    const request = { stream: false, header: new Headers({ Authorization: 'Bearer stale' }) };
    const invoke = bearerRenewalInterceptor({ serverId: 'remote', renewBearerToken });

    await expect(invoke(next as never)(request as never)).resolves.toBe(response);
    expect(renewBearerToken.mock.calls).toEqual([[false], [true]]);
    expect(next).toHaveBeenCalledTimes(2);
    expect(request.header.get('Authorization')).toBe('Bearer access-2');
  });

  it('does not retry streaming requests after authentication fails', async () => {
    const renewBearerToken = vi.fn(async () => 'access-1');
    const error = new ConnectError('expired', Code.Unauthenticated);
    const next = vi.fn().mockRejectedValue(error);
    const request = { stream: true, header: new Headers() };
    const invoke = bearerRenewalInterceptor({ serverId: 'remote', renewBearerToken });

    await expect(invoke(next as never)(request as never)).rejects.toBe(error);
    expect(renewBearerToken).toHaveBeenCalledOnce();
    expect(renewBearerToken).toHaveBeenCalledWith(false);
    expect(next).toHaveBeenCalledOnce();
  });

  it('keeps a renewed bearer session after an API retry is rejected', async () => {
    const onAuthenticationRequired = vi.fn();
    configureApiClientHooks({ onAuthenticationRequired });
    try {
      const renewBearerToken = vi
        .fn<(force: boolean) => Promise<string | null>>()
        .mockResolvedValueOnce('access-1')
        .mockResolvedValueOnce('access-2');
      const error = new ConnectError('authentication required', Code.Unauthenticated);
      const next = vi.fn().mockRejectedValue(error);
      const request = { stream: false, header: new Headers() };
      const config = { serverId: 'remote', renewBearerToken };

      await expect(bearerRenewalInterceptor(config)(next as never)(request as never))
        .rejects.toBe(error);
      expect(renewBearerToken.mock.calls).toEqual([[false], [true]]);
      expect(request.header.get('Authorization')).toBe('Bearer access-2');
      expect(next).toHaveBeenCalledTimes(2);
      await expect(
        authenticationRequiredInterceptor(config)(() => Promise.reject(error))({
          contextValues: createContextValues()
        } as never)
      ).rejects.toBe(error);
      expect(onAuthenticationRequired).not.toHaveBeenCalled();
    } finally {
      configureApiClientHooks({});
    }
  });
});

describe('authenticationRequiredInterceptor', () => {
  const unauthenticated = new ConnectError('session expired', Code.Unauthenticated);

  function unaryRequest(contextValues = createContextValues()) {
    return { contextValues } as never;
  }

  async function reject(
    config: Parameters<typeof authenticationRequiredInterceptor>[0],
    error: unknown,
    request = unaryRequest()
  ) {
    const invoke = authenticationRequiredInterceptor(config)(() => Promise.reject(error));
    await expect(invoke(request)).rejects.toBe(error);
  }

  function withHook(run: (hook: ReturnType<typeof vi.fn>) => Promise<void>) {
    const onAuthenticationRequired = vi.fn();
    configureApiClientHooks({ onAuthenticationRequired });
    return run(onAuthenticationRequired).finally(() => configureApiClientHooks({}));
  }

  it('requests sign-in when a session that cannot renew is rejected', () =>
    withHook(async (hook) => {
      await reject({ serverId: 'origin' }, unauthenticated);
      expect(hook).toHaveBeenCalledExactlyOnceWith('origin');
    }));

  it('leaves renewable sessions to the bearer renewal flow', () =>
    withHook(async (hook) => {
      await reject({ serverId: 'remote', renewBearerToken: async () => 'token' }, unauthenticated);
      expect(hook).not.toHaveBeenCalled();
    }));

  it('ignores calls without a server and errors other than Unauthenticated', () =>
    withHook(async (hook) => {
      await reject({}, unauthenticated);
      await reject({ serverId: 'origin' }, new ConnectError('denied', Code.PermissionDenied));
      await reject({ serverId: 'origin' }, new Error('network'));
      expect(hook).not.toHaveBeenCalled();
    }));

  it('lets a caller that owns the decision opt out', () =>
    withHook(async (hook) => {
      await reject(
        { serverId: 'origin' },
        unauthenticated,
        unaryRequest(skipAuthenticationRequired().contextValues)
      );
      expect(hook).not.toHaveBeenCalled();
    }));

  it('applies to every client created from a server connection', () =>
    withHook(async (hook) => {
      const fetch = vi.fn(async () =>
        Response.json({ code: 'unauthenticated', message: 'session expired' }, { status: 401 })
      );
      vi.stubGlobal('fetch', fetch);
      try {
        const client = createChattoClient(RoomService, {
          serverId: 'origin',
          baseUrl: 'http://localhost:1234/api/connect',
          bearerToken: null
        });
        await expect(client.listMembers({ roomId: 'room' })).rejects.toMatchObject({
          code: Code.Unauthenticated
        });
        expect(hook).toHaveBeenCalledExactlyOnceWith('origin');
      } finally {
        vi.unstubAllGlobals();
      }
    }));
});

describe('createChattoTransport request headers', () => {
  async function sentHeaders(
    config: Omit<Parameters<typeof createChattoClient>[1], 'baseUrl'>,
    minimumCursor?: string
  ): Promise<Headers> {
    const fetch = vi.fn(
      async (_input: RequestInfo | URL, _init?: RequestInit) =>
        new Response(new Uint8Array(), {
          status: 200,
          headers: { 'Content-Type': 'application/proto' }
        })
    );
    vi.stubGlobal('fetch', fetch);
    try {
      const client = createChattoClient(RoomService, {
        baseUrl: 'http://localhost:1234/api/connect',
        ...config
      });
      await client.listMembers(
        { roomId: 'room' },
        { headers: minimumCursorHeaders(minimumCursor) }
      );
      return new Headers(fetch.mock.calls[0][1]?.headers);
    } finally {
      vi.unstubAllGlobals();
    }
  }

  it('sends the renewed bearer token together with the realtime cursor', async () => {
    const headers = await sentHeaders(
      { serverId: 'remote', bearerToken: 'stale', renewBearerToken: async () => 'access-1' },
      'cursor-7'
    );
    expect(headers.get('Authorization')).toBe('Bearer access-1');
    expect(headers.get('Chatto-Realtime-Minimum-Cursor')).toBe('cursor-7');
  });

  it('sends a fixed bearer token without a renewal function', async () => {
    const headers = await sentHeaders({ bearerToken: 'fixed-token' });
    expect(headers.get('Authorization')).toBe('Bearer fixed-token');
    expect(headers.has('Chatto-Realtime-Minimum-Cursor')).toBe(false);
  });

  it('sends no bearer token for a cookie session', async () => {
    const headers = await sentHeaders({ serverId: 'origin', bearerToken: null });
    expect(headers.has('Authorization')).toBe(false);
  });
});
