import { describe, expect, it } from 'vitest';
import { existsSync, readFileSync } from 'node:fs';
import {
  applicableHighlights,
  unreadHighlights,
  highlightCollections,
  type ReleaseHighlight
} from './catalog';
import { selectableLocales, fallbackLocales } from '$lib/i18n/locales';

const entries: ReleaseHighlight[] = [
  { id: 'old', version: '0.5.0', titleKey: '' },
  { id: 'current', version: '0.6.0', titleKey: '' },
  { id: 'server', version: '0.6.0', serverVersion: '0.6.0', titleKey: '' },
  { id: 'future', version: '0.7.0', titleKey: '' }
];

describe('release highlight eligibility', () => {
  it('keeps unavailable client and server behavior out of release claims', () => {
    expect(applicableHighlights('users', '0.6.0', '0.5.0', entries).map((e) => e.id)).toEqual([
      'current',
      'old'
    ]);
    expect(applicableHighlights('users', '0.6.0', 'custom', entries).map((e) => e.id)).toEqual([
      'current',
      'old'
    ]);
    expect(applicableHighlights('users', 'unknown', '0.6.0', entries)).toEqual([]);
  });
  it('compares prereleases and custom builds with the shared compatibility policy', () => {
    expect(
      applicableHighlights('users', '0.6.0-beta.1', '0.6.0', entries).map((e) => e.id)
    ).toEqual(['old']);
    expect(
      applicableHighlights('users', '0.6.0-custom', '0.6.0', entries).map((e) => e.id)
    ).toEqual(['current', 'server', 'old']);
  });
  it('announces only the latest applicable release on the first visit', () => {
    const available = applicableHighlights('users', '0.7.0', '0.7.0', entries);
    expect(unreadHighlights('users', available).map((e) => e.id)).toEqual(['future']);
  });
  it('retains skipped releases after an established baseline and keeps surfaces independent', () => {
    const available = applicableHighlights('users', '0.7.0', '0.7.0', entries);
    const state = {
      usersSinceVersion: '0.5.0',
      adminsSinceVersion: '0.7.0',
      dismissedIds: ['users:current', 'admins:future']
    };
    expect(unreadHighlights('users', available, state).map((e) => e.id)).toEqual([
      'future',
      'server',
      'old'
    ]);
    expect(unreadHighlights('admins', available, state)).toEqual([]);
  });
  it('preserves dismissal across text corrections and produces no unread items for empty collections', () => {
    expect(
      unreadHighlights('users', [{ ...entries[0], titleKey: 'corrected' }], {
        dismissedIds: ['users:old']
      })
    ).toEqual([]);
    expect(unreadHighlights('users', [], { dismissedIds: [] })).toEqual([]);
  });
});

describe('translated bundled highlights', () => {
  for (const locale of selectableLocales) {
    it(`${locale} has complete translated articles`, () => {
      for (const surface of ['users', 'admins'] as const) {
        for (const entry of highlightCollections[surface]) {
          const path = (language: string) =>
            new URL(`./content/${language}/${surface}/${entry.id}.mdx`, import.meta.url);
          const own = path(locale);
          const parent = fallbackLocales[locale as keyof typeof fallbackLocales];
          const actual = existsSync(own) ? own : path(parent ?? locale);
          expect(existsSync(actual)).toBe(true);
          const text = readFileSync(actual, 'utf8');
          const english = readFileSync(path('en-GB'), 'utf8');
          expect(text.match(/topic: '[^']+'/g)).toEqual(english.match(/topic: '[^']+'/g));
          if (!locale.startsWith('en-')) expect(text).not.toBe(english);
        }
      }
    });
  }
});
