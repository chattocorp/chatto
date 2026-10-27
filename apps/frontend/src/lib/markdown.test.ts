import { describe, it, expect } from 'vitest';
import { renderInlineMarkdown, renderMarkdown } from './markdown';
import {
  canHighlightCodeLanguage,
  ensureCodeLanguageLoaded,
  isCodeLanguageLoaded,
  lowlight
} from './codeHighlighting';

function tableSource(columns: number, bodyRows: number): string {
  return [
    Array.from({ length: columns }, (_, index) => `h${index}`).join('|'),
    Array.from({ length: columns }, () => '---').join('|'),
    ...Array.from({ length: bodyRows }, () => '|')
  ].join('\n');
}

describe('renderInlineMarkdown', () => {
  it('keeps formatting and safe links without block markup or line breaks', () => {
    const html = renderInlineMarkdown('**News**\n\n*Today*\r\n[Details](https://example.com)');
    expect(html).toContain('<strong>News</strong>');
    expect(html).toContain('<em>Today</em>');
    expect(html).toContain('href="https://example.com"');
    expect(html).toContain('rel="noopener noreferrer"');
    expect(html).not.toMatch(/<(?:p|br|h[1-6]|ul|ol)\b|[\r\n]/);
  });

  it('escapes source HTML and rejects unsafe links', () => {
    const html = renderInlineMarkdown('<img src=x onerror=alert(1)> [bad](javascript:alert(1))');
    expect(html).toContain('&lt;img');
    expect(html).not.toContain('<img');
    expect(html).not.toContain('href="javascript:');
  });
});

describe('renderMarkdown', () => {
  it.each(['svg', 'mermaid', 'ansi'])(
    'uses only the rendered modifier for %s',
    async (language) => {
      const source = language === 'mermaid' ? 'flowchart LR\nA-->B' : 'source';
      for (const modifier of ['rendered', 'RENDERED']) {
        const html = await renderMarkdown(
          '```' + language + ' ' + modifier + '\n' + source + '\n```'
        );
        expect(html).toContain('data-code-panel="source" hidden');
        expect(html).toContain(`data-artifact="${language}"`);
      }
      for (const modifier of ['render', 'rendered extra']) {
        const html = await renderMarkdown(
          '```' + language + ' ' + modifier + '\n' + source + '\n```'
        );
        expect(html).not.toContain('data-artifact=');
        expect(html).not.toContain('data-code-panel="source" hidden');
      }
    }
  );

  describe('Mermaid previews', () => {
    it('recognises only the complete language hint and escapes fallback source', async () => {
      const source = 'flowchart LR\nA["<img src=x onerror=alert(1)>"] --> B\n';
      const html = await renderMarkdown('```Mermaid\n' + source + '```');
      expect(html).toContain('data-artifact="mermaid"');
      expect(html).toContain('data-copy-source="flowchart LR');
      expect(html).not.toContain('<img');
      for (const info of ['mermaid extra', 'mermaidish', 'text']) {
        expect(await renderMarkdown('```' + info + '\n' + source + '```')).not.toContain(
          'data-artifact="mermaid"'
        );
      }
    });

    it('bounds previews and leaves source configuration as code', async () => {
      const fence = '```mermaid\nflowchart LR\nA-->B\n```';
      expect(
        (await renderMarkdown(Array(5).fill(fence).join('\n\n'))).match(/ disabled/g)
      ).toHaveLength(1);
      expect(await renderMarkdown('```mermaid\n' + 'x'.repeat(10_001) + '\n```')).toContain(
        ' disabled'
      );
      expect(
        await renderMarkdown('```mermaid\n%%{init: {}}%%\nflowchart LR\nA-->B\n```')
      ).toContain(' disabled');
      expect(
        await renderMarkdown(
          fence
            .split('\n')
            .map((line) => '> ' + line)
            .join('\n')
        )
      ).toContain('data-artifact="mermaid"');
    });
  });
  describe('SVG previews', () => {
    const svg = '<svg xmlns="http://www.w3.org/2000/svg"><text>Hi &amp; bye</text></svg>\n';

    it('uses the rendered hint only to select the initial tab and keeps source escaped', async () => {
      for (const info of ['svg', 'xml', 'svg renderer', 'svg rendered extra']) {
        expect(await renderMarkdown('```' + info + '\n' + svg + '```')).not.toContain('<img');
      }
      const html = await renderMarkdown('```svg rendered\n' + svg + '```');
      expect(html).toContain('data-artifact="svg"');
      expect(html).not.toContain('<img');
      expect(html).not.toContain('<details>');
      expect(html).toContain('data-code-panel="source" hidden');
      expect(await renderMarkdown('```svg\n' + svg + '```')).toContain(
        'data-code-panel="render" hidden'
      );
      expect(html).not.toContain('<svg');
      expect(html).toContain('data-copy-source="&lt;svg');
    });

    it('supports nested fences and limits preview count and source length', async () => {
      const fence = '```svg rendered\n' + svg + '```';
      expect(
        await renderMarkdown(
          fence
            .split('\n')
            .map((line) => '> ' + line)
            .join('\n')
        )
      ).toContain('data-artifact="svg"');
      expect(
        (await renderMarkdown(Array(5).fill(fence).join('\n\n'))).match(
          /data-code-panel="source" hidden/g
        )
      ).toHaveLength(4);
      expect(await renderMarkdown('```svg rendered\n' + 'x'.repeat(100_001) + '\n```')).toContain(
        ' disabled'
      );
    });
  });
  describe('footnotes', () => {
    it('numbers notes in reference order and returns to each repeated reference', async () => {
      const html = await renderMarkdown(
        'First[^later], second[^earlier], and first again[^later].\n\n' +
          '[^earlier]: **Second** note.\n[^later]: First note.'
      );

      expect(html).toContain('<sup class="footnote-ref"><a href="#fn1" id="fnref1">[1]</a>');
      expect(html).toContain('href="#fn2" id="fnref2">[2]</a>');
      expect(html).toContain('href="#fn1" id="fnref1:1">[1:1]</a>');
      expect(html).toContain('<section class="footnotes">');
      expect(html).toContain('<li id="fn1" class="footnote-item" tabindex="-1"><p>First note.');
      expect(html).toContain(
        '<li id="fn2" class="footnote-item" tabindex="-1"><p><strong>Second</strong>'
      );
      expect(html).toContain('href="#fnref1" class="footnote-backref"');
      expect(html).toContain('href="#fnref1:1" class="footnote-backref"');
    });

    it('renders paragraphs, lists, links, and code inside a note', async () => {
      const html = await renderMarkdown(
        'Details[^note].\n\n[^note]: First paragraph with `code`.\n\n' +
          '    Second paragraph with [a link](https://example.com).\n\n' +
          '    - One item\n    - Another item'
      );

      expect(html).toContain('<p>First paragraph with <code>code</code>.</p>');
      expect(html).toContain('<p>Second paragraph with <a href="https://example.com"');
      expect(html).toContain('<ul>');
      expect(html).toContain('One item');
      expect(html).toContain('rel="noopener noreferrer"');
    });

    it('leaves undefined references and footnote syntax in code literal', async () => {
      const html = await renderMarkdown(
        'Unknown[^missing] and `[^known]`.\n\n```text\n[^known]\n```\n\n[^known]: Unused note.'
      );

      expect(html).toContain('Unknown[^missing]');
      expect(html).toContain('<code>[^known]</code>');
      expect(html).toContain('</span>[^known]</span>');
      expect(html).not.toContain('class="footnotes"');
      expect(html).not.toContain('Unused note.');
    });

    it('escapes note HTML and keeps unsafe destinations inactive', async () => {
      const html = await renderMarkdown(
        'Text[^<svg/onload=alert(1)>].\n\n' +
          '[^<svg/onload=alert(1)>]: <img src=x onerror=alert(1)> [bad](javascript:alert(1))'
      );

      expect(html).toContain('class="footnote-ref"');
      expect(html).toContain('&lt;img');
      expect(html).not.toContain('<img');
      expect(html).not.toContain('<svg');
      expect(html).not.toContain('href="javascript:');
      expect(html).not.toContain('id="<svg');
    });

    it('does not share note definitions or numbering between renders', async () => {
      const [first, second] = await Promise.all([
        renderMarkdown('Text[^note].\n\n[^note]: First document.'),
        renderMarkdown('Text[^note].')
      ]);
      const third = await renderMarkdown('Text[^note].\n\n[^note]: Third document.');

      expect(first).toContain('First document.');
      expect(second).toContain('Text[^note].');
      expect(second).not.toContain('class="footnotes"');
      expect(third).toContain('href="#fn1"');
      expect(third).not.toContain('First document.');
    });

    it('keeps inline-only rendering free of footnote markup', () => {
      const html = renderInlineMarkdown('Text[^note] and ^[inline note].');

      expect(html).toBe('Text[^note] and ^[inline note].');
    });
  });

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

    it('does not expand adversarial short rows into an oversized table', async () => {
      const body = tableSource(256, 256);

      const html = await renderMarkdown(body);

      expect(html).not.toContain('<table>');
      expect(html).not.toContain('<td>');
      expect(html.length).toBeLessThan(20_000);
    });

    it('accepts the column limit and rejects one additional column', async () => {
      const atLimit = await renderMarkdown(tableSource(64, 1));
      const overLimit = await renderMarkdown(tableSource(65, 1));

      expect(atLimit).toContain('<table>');
      expect(overLimit).not.toContain('<table>');
    });

    it('accepts the row limit and rejects one additional row', async () => {
      // The header counts as one of the 256 rows.
      const atLimit = await renderMarkdown(tableSource(2, 255));
      const overLimit = await renderMarkdown(tableSource(2, 256));

      expect(atLimit).toContain('<table>');
      expect(overLimit).not.toContain('<table>');
    });

    it('accepts the per-table cell limit and rejects one additional row of cells', async () => {
      // 64 columns × 64 rows, including the header, is exactly 4,096 cells.
      const atLimit = await renderMarkdown(tableSource(64, 63));
      const overLimit = await renderMarkdown(tableSource(64, 64));

      expect(atLimit).toContain('<table>');
      expect(overLimit).not.toContain('<table>');
    });

    it('limits cumulative table cells across one message', async () => {
      const maximumTable = tableSource(64, 63);
      const html = await renderMarkdown(
        `${maximumTable}\n\n${maximumTable}\n\n| Extra | Table |\n| --- | --- |\n| x | y |`
      );

      expect(html.match(/<table>/g)).toHaveLength(2);
      expect(html).toContain('| Extra | Table |');
    });

    it('applies table expansion limits inside blockquotes', async () => {
      const lines = tableSource(128, 128).split('\n');
      const html = await renderMarkdown(lines.map((line) => `> ${line}`).join('\n'));

      expect(html).not.toContain('<table>');
      expect(html).not.toContain('<td>');
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
      expect(html).toContain('</span>&amp;nbsp;</span>');
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

    it('renders `__foo:__` as underline with trailing colon', async () => {
      const html = await renderMarkdown('__foo:__');
      expect(html).toContain('<u>foo:</u>');
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

    it('groups inline content in tight list items', async () => {
      const html = await renderMarkdown(
        '1. Connect to `/api/realtime` using `chatto.realtime.v1`.'
      );

      expect(html).toContain(
        '<li><span class="list-item-content">Connect to <code>/api/realtime</code> using <code>chatto.realtime.v1</code>.</span>'
      );
    });
  });

  describe('code blocks', () => {
    it.each(['```js\nconst x = 1;\n```', '    indented code'])(
      'makes the code scroller keyboard focusable: %s',
      async (source) => {
        expect(await renderMarkdown(source)).toMatch(/<code[^>]* tabindex="0">/);
      }
    );

    it('renders fenced code blocks with lowlight classes', async () => {
      const html = await renderMarkdown('```js\nconst x = 1;\n```');
      expect(html).toContain('<pre class="hljs markdown-code" data-language="js"');
      expect(html).toContain('language-js');
      expect(html).toContain('hljs-keyword');
      expect(html).toContain('<span class="markdown-code-language">js</span>');
      expect(html).toContain('data-markdown-copy');
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

    it('keeps the original fenced code as the escaped copy payload', async () => {
      const html = await renderMarkdown('```text\n\t<a title="x">&</a>\n```');

      expect(html).toContain('data-copy-source="\t&lt;a title=&quot;x&quot;&gt;&amp;&lt;/a&gt;\n"');
      expect(html).toContain('&lt;a title=&quot;x&quot;&gt;&amp;&lt;/a&gt;');
      expect(html).not.toContain('<a title="x">');
    });

    it('adds a copy button to indented blocks but not inline code', async () => {
      const html = await renderMarkdown('    first\n    second\n\n`inline`');

      expect(html).toContain('data-copy-source="first\nsecond\n"');
      expect(html).toContain('<span class="markdown-code-language">text</span>');
      expect(html.match(/data-markdown-copy/g)).toHaveLength(1);
      expect(html).toContain('<code>inline</code>');
    });
  });
});

describe('automatic links', () => {
  it.each([
    'path/www.example.com',
    '@www.example.com',
    'agents.md',
    'readme.md',
    'foo.py',
    'example.com',
    'app.example.io',
    '//example.com',
    'user@example.com'
  ])('keeps %s as plain text', async (text) => {
    expect(await renderMarkdown(text)).not.toContain('<a ');
    expect(renderInlineMarkdown(text)).not.toContain('<a ');
  });

  it.each([
    'https://example.com',
    'http://example.com',
    'mailto:user@example.com',
    'www.example.com',
    'WWW.example.dev/path?q=1'
  ])('links %s', async (text) => {
    expect(await renderMarkdown(text)).toContain('<a ');
    expect(renderInlineMarkdown(text)).toContain('<a ');
  });

  it('keeps explicit Markdown links and safe angle autolinks', async () => {
    const html = await renderMarkdown(
      '[agents.md](https://example.com/agents.md) <https://example.com>'
    );
    expect(html).toContain('href="https://example.com/agents.md"');
    expect(html).toContain('href="https://example.com"');
  });
});

// Exercise the policy through real Markdown parsing, including explicit links.
describe('message protocol allow list', () => {
  it.each(['/', '/chat/chat.chatto.run/R8rLgzhPkYja7Gs', '/chat/example.com/room?q=1#message'])(
    'keeps explicit links to %s',
    async (href) => {
      const source = `[link](${href})`;
      expect(await renderMarkdown(source)).toContain(`href="${href}"`);
      expect(renderInlineMarkdown(source)).toContain(`href="${href}"`);
      expect(await renderMarkdown(href)).not.toContain('<a ');
    }
  );

  it.each([
    '//example.com/path',
    'ftp://example.com/file',
    'ftps://example.com/file',
    'ssh://example.com',
    'tel:+123456789',
    'sms:+123456789',
    'vscode://file/test',
    'file:///etc/passwd',
    'data:text/html,test',
    'javascript:alert(1)',
    'blob:https://example.com/id'
  ])('keeps unsupported %s unlinked', async (url) => {
    for (const source of [url, `<${url}>`, `[label](${url})`]) {
      expect(await renderMarkdown(source)).not.toContain('<a ');
      expect(renderInlineMarkdown(source)).not.toContain('<a ');
    }
  });
  it('keeps allowed links and formatted unsupported labels in mixed text', async () => {
    const html = await renderMarkdown(
      '[**file**](ftp://example.com/file) [web](HTTPS://example.com) [mail](mailto:foo@example.com?subject=Hello)'
    );
    expect(html).toContain('<strong>file</strong>');
    expect(html).not.toContain('href="#"');
    expect(html.match(/<a /g)).toHaveLength(2);
    expect(html).toContain('href="HTTPS://example.com"');
    expect(html).toContain('href="mailto:foo@example.com?subject=Hello"');
  });
});
