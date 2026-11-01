import { describe, expect, it } from 'vitest';
import { formatArtifactFence, parseArtifactFence } from './artifactFence';

describe('artifact fence presentation', () => {
  it.each(['svg', 'mermaid', 'ansi'] as const)(
    'round trips %s presentation and normalises case',
    (language) => {
      for (const visual of [false, true]) {
        const info = formatArtifactFence(language, visual);
        expect(parseArtifactFence(`  ${info.toUpperCase()}  `)).toEqual({
          language,
          presentation: visual ? 'visual' : 'code'
        });
      }
    }
  );
  it('rejects partial hints and removes the hint when changing to ordinary code', () => {
    for (const info of ['svg render', 'mermaid rendered extra', 'ansi-art', 'text rendered']) {
      expect(parseArtifactFence(info)).toBeNull();
    }
    expect(formatArtifactFence('typescript', true)).toBe('typescript');
  });
});
