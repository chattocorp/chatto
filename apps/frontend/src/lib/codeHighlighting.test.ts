import { describe, expect, it } from 'vitest';
import {
  CODE_LANGUAGE_OPTIONS,
  detectCodeLanguage,
  ensureCodeLanguageLoaded,
  lowlight
} from './codeHighlighting';

describe('Mermaid syntax highlighting', () => {
  it('offers Mermaid and highlights diagram tokens without changing source', async () => {
    expect(CODE_LANGUAGE_OPTIONS).toContainEqual({ value: 'mermaid', label: 'MERMAID' });
    await ensureCodeLanguageLoaded('mermaid');
    const source =
      'flowchart TD\n  %% Launch checks\n  accTitle: Launch\n  subgraph Cargo\n    A[Scan cargo] --> B{Ready?}\n  end';
    const tree = lowlight.highlight('mermaid', source);
    const text = (nodes: typeof tree.children): string =>
      nodes
        .map((node) =>
          node.type === 'text' ? node.value : node.type === 'element' ? text(node.children) : ''
        )
        .join('');
    const hasToken = (nodes: typeof tree.children, token: string, type: string): boolean =>
      nodes.some(
        (node) =>
          node.type === 'element' &&
          ((Array.isArray(node.properties.className) &&
            node.properties.className.includes(`hljs-${type}`) &&
            text(node.children) === token) ||
            hasToken(node.children, token, type))
      );
    for (const [token, type] of [
      ['flowchart', 'keyword'],
      ['TD', 'built_in'],
      ['%% Launch checks', 'comment'],
      ['accTitle', 'attribute'],
      ['subgraph', 'keyword'],
      ['[Scan cargo]', 'string'],
      ['-->', 'operator'],
      ['{Ready?}', 'string'],
      ['end', 'keyword']
    ]) {
      expect(hasToken(tree.children, token, type)).toBe(true);
    }
    expect(text(tree.children)).toBe(source);
  });
});

describe('code language guesses', () => {
  it('uses the same candidate set after unrelated languages load', async () => {
    const code =
      '19:38:55 [worker-3438 / task-3] Choose Chatto participation\n19:38:56 [worker-3438 / task-4] Decision: ignore · probabilities: {"engage":0.19}';
    const language = await detectCodeLanguage(code);
    expect(language).not.toBeNull();
    await ensureCodeLanguageLoaded('1c');
    await ensureCodeLanguageLoaded('mermaid');
    expect(await detectCodeLanguage(code)).toBe(language);
    expect(lowlight.highlight(language!, code).children.length).toBeGreaterThan(0);
  });

  it('leaves content without a match plain', async () => {
    expect(await detectCodeLanguage('')).toBeNull();
    expect(await detectCodeLanguage('   ')).toBeNull();
    expect(await detectCodeLanguage('xyzzy')).toBeNull();
  });
});
