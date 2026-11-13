import { describe, expect, it } from 'vitest';
import { readGutterEnabled } from './frontendConfig';

function documentWith(head: string): Document {
  return new DOMParser().parseFromString(`<html><head>${head}</head></html>`, 'text/html');
}

describe('readGutterEnabled', () => {
  it('enables the gutter without a server setting', () => {
    expect(readGutterEnabled(undefined)).toBe(true);
    expect(readGutterEnabled(documentWith(''))).toBe(true);
  });

  it('follows the setting of the serving server', () => {
    expect(readGutterEnabled(documentWith('<meta name="chatto-gutter" content="false">'))).toBe(
      false
    );
    expect(readGutterEnabled(documentWith('<meta name="chatto-gutter" content="true">'))).toBe(
      true
    );
  });
});
