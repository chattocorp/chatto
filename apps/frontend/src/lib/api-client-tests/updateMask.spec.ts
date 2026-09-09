import { describe, expect, it } from 'vitest';
import { updateMask } from '$lib/api-client/updateMask';

describe('updateMask', () => {
  it('selects explicit defaults and null but excludes omitted fields and identifiers', () => {
    const values = {
      roomId: 'r1',
      bio: null,
      shareTimezone: false,
      count: 0,
      text: '',
      names: [],
      absent: undefined
    };
    expect(
      updateMask(values, ['bio', 'shareTimezone', 'count', 'text', 'names', 'absent'])
    ).toEqual({
      paths: ['bio', 'share_timezone', 'count', 'text', 'names']
    });
  });
});
