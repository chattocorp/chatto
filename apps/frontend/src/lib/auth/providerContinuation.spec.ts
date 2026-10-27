import { describe, expect, it, vi } from 'vitest';
import { isProviderContinuation } from './providerContinuation';

vi.mock('$app/paths', () => ({ resolve: (path: string) => path }));

describe('provider continuation navigation', () => {
  it.each([
    ['/sso/confirm?token=flow&existing=1', true],
    ['/servers/callback?mode=provider&state=transaction', true],
    ['/servers/callback?mode=popup&state=transaction', false],
    ['/sso/confirm?token=flow', false],
    ['/sso/confirm?existing=1', false],
    ['/chat/-/settings/account', false],
    ['//other.example/sso/confirm?token=flow&existing=1', false],
    ['/\\other.example/sso/confirm?token=flow&existing=1', false]
  ])('recognizes only explicit local provider continuations: %s', (path, expected) => {
    expect(isProviderContinuation(path)).toBe(expected);
  });
});
