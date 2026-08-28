// SPDX-License-Identifier: Apache-2.0

import { Code, ConnectError } from '@connectrpc/connect';
import { describe, expect, it, vi } from 'vitest';
import { bearerRenewalInterceptor } from './connect';

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
