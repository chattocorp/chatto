import { describe, expect, it } from 'vitest';
import {
  customStatusTemplateText,
  formatCustomStatusText,
  getCustomStatusTemplate
} from './customStatusTemplates';

describe('custom status templates', () => {
  it('uses reserved text tokens for templates', () => {
    expect(customStatusTemplateText('out_for_lunch')).toBe('chatto:status:out_for_lunch');
    expect(customStatusTemplateText('vacation')).toBe('chatto:status:vacation');
    expect(customStatusTemplateText('sick')).toBe('chatto:status:sick');
  });

  it('recognizes a template only when emoji and token match', () => {
    expect(
      getCustomStatusTemplate({
        emoji: '🍽️',
        text: 'chatto:status:out_for_lunch',
        expiresAt: null
      })?.id
    ).toBe('out_for_lunch');

    expect(
      getCustomStatusTemplate({
        emoji: '🌴',
        text: 'chatto:status:out_for_lunch',
        expiresAt: null
      })
    ).toBeUndefined();
  });

  it('formats template tokens and leaves custom text untouched', () => {
    expect(formatCustomStatusText('chatto:status:vacation')).toBe('Vacation');
    expect(formatCustomStatusText('In focus mode')).toBe('In focus mode');
  });
});
