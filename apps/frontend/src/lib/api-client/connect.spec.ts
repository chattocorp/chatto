// SPDX-License-Identifier: Apache-2.0

import { Code, ConnectError } from '@connectrpc/connect';
import { describe, expect, it, vi } from 'vitest';
import {
  bearerRenewalInterceptor,
  dataGenerationInterceptor,
  privateRequestInterceptor,
  StaleResponseError
} from './connect';

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
});
