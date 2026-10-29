/** External messages and arbitrary error codes must never enter startup logs. */
import { expect, test } from 'vitest';
import { startupStep } from './diagnostics.ts';

test('uses a fixed hint from a nested error without exposing private content', async () => {
  const error = new Error('https://secret@example.com/private', {
    cause: Object.assign(new Error('private payload'), { code: 'ECONNREFUSED' })
  });
  await expect(
    startupStep('Cannot connect.', async () => {
      throw error;
    })
  ).rejects.toThrow('Cannot connect. Connection refused.');
});

test('drops arbitrary error codes and messages', async () => {
  try {
    await startupStep('Cannot connect.', async () => {
      throw { code: 'secret token', message: 'private payload' };
    });
  } catch (error) {
    expect(error).toBeInstanceOf(Error);
    expect((error as Error).message).toBe('Cannot connect.');
  }
});
