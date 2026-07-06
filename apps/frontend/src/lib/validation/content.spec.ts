import { describe, test, expect } from 'vitest';
import { hasVisibleContent } from './content';

describe('hasVisibleContent', () => {
  describe('invisible content returns false', () => {
    test('empty string', () => {
      expect(hasVisibleContent('')).toBe(false);
    });

    test('space only', () => {
      expect(hasVisibleContent(' ')).toBe(false);
    });

    test('multiple spaces', () => {
      expect(hasVisibleContent('   ')).toBe(false);
    });

    test('tab only', () => {
      expect(hasVisibleContent('\t')).toBe(false);
    });

    test('newline only', () => {
      expect(hasVisibleContent('\n')).toBe(false);
    });

    test('mixed whitespace', () => {
      expect(hasVisibleContent(' \t\n\r ')).toBe(false);
    });

    test('zero-width space only', () => {
      expect(hasVisibleContent('\u200B')).toBe(false);
    });

    test('multiple zero-width spaces', () => {
      expect(hasVisibleContent('\u200B\u200B\u200B')).toBe(false);
    });

    test('zero-width joiner only', () => {
      expect(hasVisibleContent('\u200D')).toBe(false);
    });

    test('zero-width non-joiner only', () => {
      expect(hasVisibleContent('\u200C')).toBe(false);
    });

    test('mixed zero-width chars', () => {
      expect(hasVisibleContent('\u200B\u200C\u200D')).toBe(false);
    });

    test('word joiner only', () => {
      expect(hasVisibleContent('\u2060')).toBe(false);
    });

    test('BOM only', () => {
      expect(hasVisibleContent('\uFEFF')).toBe(false);
    });

    test('soft hyphen only', () => {
      expect(hasVisibleContent('\u00AD')).toBe(false);
    });

    test('LTR mark only', () => {
      expect(hasVisibleContent('\u200E')).toBe(false);
    });

    test('RTL mark only', () => {
      expect(hasVisibleContent('\u200F')).toBe(false);
    });

    test('mixed invisible chars', () => {
      expect(hasVisibleContent('\u200B \u200C\t\u200D\n\u2060')).toBe(false);
    });

    test('whitespace and invisible chars', () => {
      expect(hasVisibleContent('  \u200B  \u200C  ')).toBe(false);
    });
  });

  describe('visible content returns true', () => {
    test('single letter', () => {
      expect(hasVisibleContent('a')).toBe(true);
    });

    test('word', () => {
      expect(hasVisibleContent('hello')).toBe(true);
    });

    test('sentence', () => {
      expect(hasVisibleContent('Hello, world!')).toBe(true);
    });

    test('digits', () => {
      expect(hasVisibleContent('12345')).toBe(true);
    });

    test('emoji only', () => {
      expect(hasVisibleContent('😀')).toBe(true);
    });

    test('multiple emoji', () => {
      expect(hasVisibleContent('🎉🎊🎈')).toBe(true);
    });

    test('punctuation', () => {
      expect(hasVisibleContent('!!!')).toBe(true);
    });

    test('text with leading space', () => {
      expect(hasVisibleContent(' hello')).toBe(true);
    });

    test('text with trailing space', () => {
      expect(hasVisibleContent('hello ')).toBe(true);
    });

    test('text with invisible chars mixed', () => {
      expect(hasVisibleContent('\u200Bhello\u200B')).toBe(true);
    });

    test('emoji with invisible chars', () => {
      expect(hasVisibleContent('\u200B😀\u200B')).toBe(true);
    });

    test('Japanese characters', () => {
      expect(hasVisibleContent('田中')).toBe(true);
    });

    test('Chinese characters', () => {
      expect(hasVisibleContent('你好')).toBe(true);
    });

    test('Arabic text', () => {
      expect(hasVisibleContent('مرحبا')).toBe(true);
    });

    test('Hebrew text', () => {
      expect(hasVisibleContent('שלום')).toBe(true);
    });

    test('Cyrillic text', () => {
      expect(hasVisibleContent('Привет')).toBe(true);
    });
  });
});
