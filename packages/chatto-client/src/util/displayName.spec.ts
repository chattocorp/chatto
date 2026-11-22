import { describe, expect, it } from 'vitest';
import {
  DISPLAY_NAME_MAX_LENGTH,
  displayNameValidationError,
  presentableDisplayName
} from './displayName.js';

const NANDI = '\u{1FB15}\u{1FB22}▌\u{1FB1C}\u{1FB0E}\u{1FB13}';

describe('displayNameValidationError', () => {
  it.each(['Alice', '𝐍𝐀𝐍𝐃𝐈', '(╯°□°)╯︵ ┻━┻', '⣿ Braille', '👩‍💻', 'می‌روم'])('accepts %j', (name) =>
    expect(displayNameValidationError(name)).toBeUndefined()
  );

  it.each(['A█B', NANDI, '\u{1CCD6}', 'A‮B', 'A\nB', '⠀', '́'])('rejects %j', (name) =>
    expect(displayNameValidationError(name)).toBe('invalid')
  );

  it('reports empty and long names', () => {
    expect(displayNameValidationError('')).toBe('empty');
    expect(displayNameValidationError('a'.repeat(DISPLAY_NAME_MAX_LENGTH + 1))).toBe('too_long');
  });
});

describe('presentableDisplayName', () => {
  it('keeps valid names and trims them', () => {
    expect(presentableDisplayName(' Alice ', 'alice')).toBe('Alice');
  });

  it('shows the login for a name that the rules do not permit', () => {
    expect(presentableDisplayName(NANDI, 'nandi')).toBe('nandi');
  });

  it('keeps an empty name or a name without a login', () => {
    expect(presentableDisplayName('', 'nandi')).toBe('');
    expect(presentableDisplayName(NANDI, '')).toBe(NANDI);
  });
});
