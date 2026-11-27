import { describe, expect, it, vi } from 'vitest';

vi.mock('$app/navigation', () => ({ goto: vi.fn() }));
vi.mock('$app/paths', () => ({ resolve: (path: string) => path }));

const { messagesFromQuery } = await import('./searchUserMessages');

describe('messagesFromQuery', () => {
  it('uses a bare login when the parser accepts it', () => {
    expect(messagesFromQuery('alice')).toBe('from:alice');
    expect(messagesFromQuery('bob.smith-2_x')).toBe('from:bob.smith-2_x');
  });

  it('quotes a login with other characters', () => {
    expect(messagesFromQuery('ali ce')).toBe('from:"ali ce"');
  });
});
