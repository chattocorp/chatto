import { describe, expect, it } from 'vitest';
import { showsServerSignedOut } from './serverSignedOut';

describe('showsServerSignedOut', () => {
  const remote = { isOrigin: false, hasDisplayableView: false };

  it('shows the view for a remote server without a session', () => {
    expect(showsServerSignedOut({ token: null, reauthRequiredAt: null }, remote)).toBe(true);
    expect(
      showsServerSignedOut(
        { token: null, reauthRequiredAt: null },
        { ...remote, hasDisplayableView: true }
      )
    ).toBe(true);
  });

  it('shows the view for a rejected session only while no chat data is loaded', () => {
    const server = { token: 'rejected', reauthRequiredAt: 123 };
    expect(showsServerSignedOut(server, remote)).toBe(true);
    expect(showsServerSignedOut(server, { ...remote, hasDisplayableView: true })).toBe(false);
  });

  it('keeps a usable session and the origin out of the view', () => {
    expect(showsServerSignedOut({ token: 'valid', reauthRequiredAt: null }, remote)).toBe(false);
    expect(
      showsServerSignedOut(
        { token: null, reauthRequiredAt: 123 },
        { isOrigin: true, hasDisplayableView: false }
      )
    ).toBe(false);
  });
});
