// @vitest-environment node
import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  customStatusTemplateText,
  defaultTemplateExpiry,
  getCustomStatusTemplate,
  getCustomStatusTemplateByToken
} from './customStatusTemplates.js';

afterEach(() => vi.useRealTimers());

describe('custom status templates', () => {
  it('uses reserved text tokens for templates', () => {
    expect(customStatusTemplateText('out_for_lunch')).toBe('chatto:status:out_for_lunch');
    expect(customStatusTemplateText('vacation')).toBe('chatto:status:vacation');
    expect(customStatusTemplateText('sick')).toBe('chatto:status:sick');
  });

  it('recognizes a template only when emoji and token match', () => {
    expect(
      getCustomStatusTemplate({ emoji: '🍽️', text: 'chatto:status:out_for_lunch', expiresAt: null })
        ?.id
    ).toBe('out_for_lunch');
    expect(
      getCustomStatusTemplate({ emoji: '🌴', text: 'chatto:status:out_for_lunch', expiresAt: null })
    ).toBeUndefined();
    expect(getCustomStatusTemplate(null)).toBeUndefined();
    expect(getCustomStatusTemplateByToken('In focus mode')).toBeUndefined();
  });

  it('suggests an expiry only for templates that define one', () => {
    vi.useFakeTimers({ toFake: ['Date'] });
    vi.setSystemTime(new Date('2026-01-01T12:00:00Z'));
    expect(defaultTemplateExpiry('out_for_lunch')?.toISOString()).toBe('2026-01-01T13:00:00.000Z');
    expect(defaultTemplateExpiry('vacation')).toBeNull();
  });
});
