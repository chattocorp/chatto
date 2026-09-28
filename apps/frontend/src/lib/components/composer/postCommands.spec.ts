import { describe, expect, it } from 'vitest';
import { expandPostCommand } from './postCommands';

describe('expandPostCommand', () => {
  it('appends the shrug to command text', () => {
    expect(expandPostCommand('/shrug I do not know')).toBe('I do not know ¯\\_(ツ)_/¯');
  });

  it('sends only the shrug for a command with no message', () => {
    expect(expandPostCommand('/shrug')).toBe('¯\\_(ツ)_/¯');
    expect(expandPostCommand('/shrug   ')).toBe('¯\\_(ツ)_/¯');
  });

  it('keeps multiline message text before the shrug', () => {
    expect(expandPostCommand('/shrug\nfirst line\nsecond line')).toBe(
      'first line\nsecond line ¯\\_(ツ)_/¯'
    );
  });

  it('does not replace ordinary message text or partial commands', () => {
    for (const body of ['hello /shrug', '/shrugging', '/Shrug hi', '\\/shrug hi']) {
      expect(expandPostCommand(body)).toBe(body);
    }
  });
});
