import { describe, expect, it } from 'vitest';
import { detectCodeLanguage, ensureCodeLanguageLoaded, lowlight } from './codeHighlighting';

describe('code language guesses', () => {
  it('uses the same candidate set after unrelated languages load', async () => {
    const code =
      '19:38:55 [worker-3438 / task-3] Choose Chatto participation\n19:38:56 [worker-3438 / task-4] Decision: ignore · probabilities: {"engage":0.19}';
    const language = await detectCodeLanguage(code);
    expect(language).not.toBeNull();
    await ensureCodeLanguageLoaded('1c');
    expect(await detectCodeLanguage(code)).toBe(language);
    expect(lowlight.highlight(language!, code).children.length).toBeGreaterThan(0);
  });

  it('leaves content without a match plain', async () => {
    expect(await detectCodeLanguage('')).toBeNull();
    expect(await detectCodeLanguage('   ')).toBeNull();
    expect(await detectCodeLanguage('xyzzy')).toBeNull();
  });
});
