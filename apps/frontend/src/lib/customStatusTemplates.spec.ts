import { describe, expect, it } from 'vitest';
import { CUSTOM_STATUS_TEMPLATES, formatCustomStatusText } from './customStatusTemplates';

describe('custom status template labels', () => {
  it('formats template tokens and leaves custom text untouched', () => {
    expect(formatCustomStatusText('chatto:status:vacation')).toBe('Holiday');
    expect(formatCustomStatusText('In focus mode')).toBe('In focus mode');
  });

  it('labels every client template', () => {
    expect(CUSTOM_STATUS_TEMPLATES.map((template) => template.label())).toEqual([
      'Out for lunch',
      'Holiday',
      'Sick'
    ]);
  });
});
