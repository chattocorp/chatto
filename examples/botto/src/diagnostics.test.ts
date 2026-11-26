/** External messages and arbitrary error codes must never enter startup logs. */
import { expect, test } from 'vitest';
import { announcementError, announcementStep, startupStep } from './diagnostics.ts';

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

test('announcement diagnostics retain the inner stage and only an approved error-code hint', async () => {
  const privateError = Object.assign(
    new Error('Bearer secret-token https://private.example/?key=secret'),
    { code: 'ECONNREFUSED' }
  );
  const result = await announcementStep('model turn', () =>
    announcementStep('thread delivery', () => {
      throw privateError;
    })
  ).catch((error) => error);
  expect(result.message).toBe('Announcement thread delivery failed. Connection refused.');
  expect(result.cause).toBeUndefined();
  expect(announcementError('workflow', result)).toBe(result);
  expect(result.stack).not.toContain('secret-token');
});

test('announcement diagnostics hide arbitrary messages and codes, including model setup errors', async () => {
  await expect(
    announcementStep('model setup', () => {
      throw { code: 'private token', message: 'private message' };
    })
  ).rejects.toThrow('Announcement model setup failed.');
});
