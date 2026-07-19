import { describe, it, expect } from 'vitest';
import { renderMarkdown } from './markdown';
import {
  canHighlightCodeLanguage,
  ensureCodeLanguageLoaded,
  isCodeLanguageLoaded,
  lowlight
} from './codeHighlighting';

describe('renderMarkdown', () => {
  describe('GFM tables', () => {
    it('renders a pipe table with semantic sections', async () => {
      const html = await renderMarkdown('| Name | Role |\n| --- | --- |\n| Ada | Admin |');

      expect(html).toContain('<div class="table-scroll" tabindex="0"><table>');
      expect(html).toContain('<thead>');
      expect(html).toContain('<th>Name</th>');
      expect(html).toContain('<tbody>');
      expect(html).toContain('<td>Ada</td>');
      expect(html).toContain('</table></div>');
    });

    it('renders tables without outer pipes', async () => {
      const html = await renderMarkdown('Name | Role\n--- | ---\nAda | Admin');

      expect(html).toContain('<table>');
      expect(html).toContain('<td>Ada</td>');
    });

    it('honours GFM column alignment', async () => {
      const html = await renderMarkdown(
        '| Left | Centre | Right |\n| :--- | :---: | ---: |\n| a | b | c |'
      );

      expect(html).toContain('<th style="text-align:left">Left</th>');
      expect(html).toContain('<th style="text-align:center">Centre</th>');
      expect(html).toContain('<th style="text-align:right">Right</th>');
      expect(html).toContain('<td style="text-align:center">b</td>');
    });

    it('renders inline Markdown and escaped pipes inside cells', async () => {
      const html = await renderMarkdown(
        '| Value | Notes |\n| --- | --- |\n| **bold** | one \\| two |\n| `a\\|b` | [link](https://example.com) |'
      );

      expect(html).toContain('<strong>bold</strong>');
      expect(html).toContain('<td>one | two</td>');
      expect(html).toContain('<code>a|b</code>');
      expect(html).toContain('href="https://example.com"');
    });

    it('leaves table-like text literal without a valid delimiter row', async () => {
      const html = await renderMarkdown('| Name | Role |\n| Ada | Admin |');

      expect(html).not.toContain('<table>');
    });
  });

  describe('invisible spacing', () => {
    it('collapses lines made from encoded non-breaking spaces', async () => {
      const html = await renderMarkdown(`before\n${'&nbsp;\n'.repeat(500)}after`);

      expect(html).toContain('before');
      expect(html).toContain('after');
      expect(html).not.toContain('&nbsp;');
      expect(html.match(/<br>/g)).toHaveLength(1);
    });

    it('preserves word separation when normalizing non-breaking spaces', async () => {
      const html = await renderMarkdown('bread&nbsp;and&#160;butter');

      expect(html).toContain('<p>bread and butter</p>');
    });

    it('removes paragraphs made only from encoded non-breaking spaces', async () => {
      const html = await renderMarkdown(`before\n\n${'&nbsp;\n\n'.repeat(500)}after`);

      expect(html.match(/<p>/g)).toHaveLength(2);
      expect(html).toContain('<p>before</p>');
      expect(html).toContain('<p>after</p>');
    });

    it('preserves non-breaking-space entity source in code', async () => {
      const html = await renderMarkdown('`&nbsp;`\n\n```text\n&nbsp;\n```');

      expect(html).toContain('<code>&amp;nbsp;</code>');
      expect(html).toContain('<span class="line">&amp;nbsp;</span>');
    });

    it('preserves entity spellings Markdown-it does not decode', async () => {
      const html = await renderMarkdown('&NBSP;\n&#00000160;');

      expect(html).toContain('&amp;NBSP;');
      expect(html).toContain('&amp;#00000160;');
    });
  });

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

    it('renders bold text when the closing marker is followed by text', async () => {
      const html = await renderMarkdown('fsdfsd **fsdf**fdsf');
      expect(html).toContain('<strong>fsdf</strong>fdsf');
    });

    it('renders bold text embedded within surrounding text', async () => {
      const html = await renderMarkdown('fsdfsd**fsdf**fdsf');
      expect(html).toContain('fsdfsd<strong>fsdf</strong>fdsf');
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
