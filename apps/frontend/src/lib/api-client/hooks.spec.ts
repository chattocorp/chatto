import { afterEach, describe, expect, it, vi } from 'vitest';
import { configureApiClientHooks, resolveUserSummaries } from './hooks';

describe('user summary resolution hooks', () => {
  afterEach(() => configureApiClientHooks({}));

  it('leaves cache writes with the resolver instead of priming its result again', async () => {
    const users = [{ id: 'U1', login: 'user', displayName: 'User', deleted: false, avatarUrl: null }];
    const prime = vi.fn();
    const resolve = vi.fn().mockResolvedValue(users);
    const read = vi.fn();
    configureApiClientHooks({ resolveUserSummaries: resolve, onUserSummaries: prime });
    expect(await resolveUserSummaries('server', ['U1'], read, 'cursor')).toBe(users);
    expect(resolve).toHaveBeenCalledWith('server', ['U1'], read, 'cursor');
    expect(read).not.toHaveBeenCalled();
    expect(prime).not.toHaveBeenCalled();
  });

  it('retains direct reads and cache notification without an application resolver', async () => {
    const users: [] = [];
    const read = vi.fn().mockResolvedValue(users);
    const prime = vi.fn();
    configureApiClientHooks({ onUserSummaries: prime });
    await resolveUserSummaries('server', ['U1'], read, 'cursor');
    expect(read).toHaveBeenCalledWith(['U1'], 'cursor');
    expect(prime).toHaveBeenCalledWith('server', users);
  });
});
