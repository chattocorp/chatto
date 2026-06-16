import { describe, it, expect } from 'vitest';
import { renderMarkdown } from './markdown';
import {
  canHighlightCodeLanguage,
  ensureCodeLanguageLoaded,
  isCodeLanguageLoaded,
  lowlight
} from './codeHighlighting';

describe('renderMarkdown', () => {
  describe('literal backslashes', () => {
    it('preserves the backslash in the shrug kaomoji', async () => {
      const html = await renderMarkdown('¯\\_(ツ)_/¯');
      expect(html).toContain('¯\\_(ツ)_/¯');
      expect(html).not.toContain('<em>');
    });

    it('preserves backslashes in Windows-style paths', async () => {
      const html = await renderMarkdown('C:\\Users\\foo');
      expect(html).toContain('C:\\Users\\foo');
    });
  });

  describe('emphasis at word boundaries', () => {
    it('renders `*italic*` as italic', async () => {
      const html = await renderMarkdown('*italic*');
      expect(html).toContain('<em>italic</em>');
    });

    it('renders `_italic_` as italic', async () => {
      const html = await renderMarkdown('_italic_');
      expect(html).toContain('<em>italic</em>');
    });

    it('renders `**bold**` as bold', async () => {
      const html = await renderMarkdown('**bold**');
      expect(html).toContain('<strong>bold</strong>');
    });

    it('renders italic surrounded by other text', async () => {
      const html = await renderMarkdown('hello *world* foo');
      expect(html).toContain('<em>world</em>');
    });

    it('renders `_...moo_` as italic with leading punctuation', async () => {
      const html = await renderMarkdown('_...moo_');
      expect(html).toContain('<em>...moo</em>');
    });

    it('renders `*...moo*` as italic with leading punctuation', async () => {
      const html = await renderMarkdown('*...moo*');
      expect(html).toContain('<em>...moo</em>');
    });

    it('renders `**...bold**` as bold with leading punctuation', async () => {
      const html = await renderMarkdown('**...bold**');
      expect(html).toContain('<strong>...bold</strong>');
    });

    it('renders `**foo:**` as bold with trailing colon', async () => {
      const html = await renderMarkdown('**foo:**');
      expect(html).toContain('<strong>foo:</strong>');
    });

    it('renders `__foo:__` as bold with trailing colon', async () => {
      const html = await renderMarkdown('__foo:__');
      expect(html).toContain('<strong>foo:</strong>');
    });

    it('renders `*foo:*` as italic with trailing colon', async () => {
      const html = await renderMarkdown('*foo:*');
      expect(html).toContain('<em>foo:</em>');
    });

    it('renders `_foo:_` as italic with trailing colon', async () => {
      const html = await renderMarkdown('_foo:_');
      expect(html).toContain('<em>foo:</em>');
    });

    it('renders `**foo:** bar` as bold followed by text', async () => {
      const html = await renderMarkdown('**foo:** bar');
      expect(html).toContain('<strong>foo:</strong>');
    });

    it('renders `**foo:** bar` inside a list item as bold', async () => {
      const html = await renderMarkdown('- **foo:** bar');
      expect(html).toContain('<strong>foo:</strong>');
    });

    it('renders both halves of `**foo:** bar **baz**` as bold', async () => {
      const html = await renderMarkdown('**foo:** bar **baz**');
      expect(html).toContain('<strong>foo:</strong>');
      expect(html).toContain('<strong>baz</strong>');
    });

    it('renders both halves of `**foo:** **bar**` as bold', async () => {
      const html = await renderMarkdown('**foo:** **bar**');
      expect(html).toContain('<strong>foo:</strong>');
      expect(html).toContain('<strong>bar</strong>');
    });
  });

  describe('emphasis suppressed when not at word boundaries', () => {
    it('does not italicize underscores between punctuation', async () => {
      const html = await renderMarkdown('_(ツ)_/¯');
      expect(html).toContain('_(ツ)_/¯');
      expect(html).not.toContain('<em>');
    });

    it('does not italicize asterisks between punctuation', async () => {
      const html = await renderMarkdown('*(ツ)*');
      expect(html).toContain('*(ツ)*');
      expect(html).not.toContain('<em>');
    });

    it('does not break snake_case identifiers', async () => {
      const html = await renderMarkdown('snake_case_name');
      expect(html).toContain('snake_case_name');
      expect(html).not.toContain('<em>');
    });

    it('does not italicize intraword asterisks', async () => {
      const html = await renderMarkdown('foo*bar*baz');
      expect(html).toContain('foo*bar*baz');
      expect(html).not.toContain('<em>');
    });
  });

  describe('code spans', () => {
    it('renders inline code', async () => {
      const html = await renderMarkdown('`code`');
      expect(html).toContain('<code>code</code>');
    });

    it('preserves literal markdown chars inside code spans', async () => {
      const html = await renderMarkdown('`*not bold*`');
      expect(html).toContain('<code>*not bold*</code>');
    });
  });

  describe('code blocks', () => {
    it('renders fenced code blocks with lowlight classes', async () => {
      const html = await renderMarkdown('```js\nconst x = 1;\n```');
      expect(html).toContain('<pre class="hljs" data-language="js">');
      expect(html).toContain('language-js');
      expect(html).toContain('hljs-keyword');
    });

    it('does not render the fence delimiter newline as a blank code line', async () => {
      const html = await renderMarkdown('```js\nconst x = 1;\n```');
      expect(html.match(/class="line"/g)).toHaveLength(1);
    });

    it('does not add whitespace rows between rendered code lines', async () => {
      const html = await renderMarkdown('```js\nconst x = 1;\nconst y = 2;\n```');
      expect(html).not.toContain('</span>\n<span class="line">');
    });

    it('loads alias languages on demand while preserving the original label', async () => {
      const html = await renderMarkdown('```toml\nname = "chatto"\n```');
      expect(html).toContain('data-language="toml"');
      expect(html).toContain('language-toml');
      expect(html).toContain('hljs-attr');
    });

    it('derives aliases from Highlight.js supported language metadata', () => {
      expect(canHighlightCodeLanguage('pas')).toBe(true);
      expect(canHighlightCodeLanguage('notalanguage')).toBe(false);
    });

    it('registers aliases on the shared lowlight instance', async () => {
      await ensureCodeLanguageLoaded('js');
      expect(lowlight.registered('js')).toBe(true);
    });

    it('loads bundled languages lazily when they appear in a fence', async () => {
      expect(isCodeLanguageLoaded('1c')).toBe(false);

      const html = await renderMarkdown('```1c\nПроцедура Тест()\nКонецПроцедуры\n```');

      expect(isCodeLanguageLoaded('1c')).toBe(true);
      expect(html).toContain('data-language="1c"');
      expect(html).toContain('language-1c');
    });

    it('preserves unsupported language labels while rendering plain code', async () => {
      const html = await renderMarkdown('```notalanguage\nname = "chatto"\n```');
      expect(html).toContain('data-language="notalanguage"');
      expect(html).toContain('language-notalanguage');
      expect(html).toContain('name = &quot;chatto&quot;');
    });
  });
});
