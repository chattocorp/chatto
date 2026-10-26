import { describe, expect, it } from 'vitest';
import { MAX_SOURCE_LENGTH, renderAnsiArt } from './ansiArt';
import { renderMarkdown } from './markdown';

const ESC = '\x1b';
const fence = (source: string) => '```ansi\n' + source + '\n```';
const screenText = (html: string) =>
  html
    .replace(/<[^>]*>/g, '')
    .split('\n')
    .map((line) => line.trimEnd());

describe('ANSI art', () => {
  it('preserves carriage returns through Markdown parsing and copy source', async () => {
    const source = 'old\rnew\r\nnext';
    const html = await renderMarkdown(fence(source));
    expect(html).toContain(`data-copy-source="${source.replaceAll('\r', '&#13;')}\n"`);
    const code = html.match(/<code[^>]*>([\s\S]*?)<\/code>/)![1];
    expect(screenText(code)).toEqual(['new', 'next']);
  });
  it('renders palette, indexed and true colors, including painted spaces', async () => {
    const html = await renderAnsiArt(
      `${ESC}[31mRED${ESC}[38;5;46mGREEN${ESC}[38;2;1;2;3mRGB${ESC}[44m  `
    );
    expect(html).toContain('color:#aa0000');
    expect(html).toContain('color:rgb(0,255,0)');
    expect(html).toContain('color:#010203');
    expect(html).toContain('background-color:#0000aa');
  });

  it('applies cursor movement, overwriting, erase and saved cursor positions', async () => {
    const html = await renderAnsiArt(
      `wrong${ESC}[1GOK${ESC}[K${ESC}[3;4H${ESC}7X${ESC}[1;1H${ESC}8Y`
    );
    expect(screenText(html)).toEqual(['OK', '', '   Y']);
  });

  it.each([
    ['palette', `${ESC}[2;31;44m`, '#aa0000', '#0000aa'],
    ['true color', `${ESC}[2;38;2;20;40;60;48;2;100;120;140m`, '#14283c', '#64788c'],
    ['inverse', `${ESC}[2;7;31;44m`, '#0000aa', '#aa0000'],
    ['invisible', `${ESC}[2;8;31;44m`, '#0000aa', '#0000aa']
  ])('dims only the foreground for %s cells', async (_name, sequence, foreground, background) => {
    const html = await renderAnsiArt(sequence + 'X');
    expect(html).toContain(`color:color-mix(in srgb, ${foreground} 50%, ${background})`);
    expect(html).toContain(`background-color:${background}`);
    expect(html).not.toContain('opacity');
  });

  it('preserves block characters, tabs and wide Unicode cells', async () => {
    const html = await renderAnsiArt('█▀▄\t界!\nnext');
    expect(screenText(html)).toEqual(['█▀▄     界!', 'next']);
  });

  it('escapes HTML and does not expose OSC links, titles or clipboard payloads', async () => {
    const html = await renderAnsiArt(
      `${ESC}]8;;https://example.com\x07<img>${ESC}]8;;\x07${ESC}]52;c;secret\x07${ESC}]2;title\x07`
    );
    expect(html).toContain('&lt;img&gt;');
    expect(html).not.toMatch(/<img|href|example\.com|secret|title/);
  });

  it('bounds cursor movement and ignores window resizing', async () => {
    const html = await renderAnsiArt(`${ESC}[8;99999;99999t${ESC}[99999;99999HX`);
    const lines = screenText(html);
    expect(lines).toHaveLength(100);
    expect(lines[99]).toBe(' '.repeat(79) + 'X');
    expect(html.length).toBeLessThan(20_000);
    await expect(renderAnsiArt('x'.repeat(MAX_SOURCE_LENGTH + 1))).rejects.toThrow('limit');
  });

  it('isolates colors and cursor state between concurrent messages', async () => {
    const [red, plain] = await Promise.all([
      renderMarkdown(fence(`${ESC}[31mred`)),
      renderMarkdown(fence('plain'))
    ]);
    expect(red).toContain('color:#aa0000');
    expect(plain).not.toContain('color:#aa0000');
    expect(plain).toContain('plain');
  });

  it('renders only ansi fences, including nested fences, and keeps copy source', async () => {
    const html = await renderMarkdown('> ' + fence(`${ESC}[32mART`).replaceAll('\n', '\n> '));
    expect(html).toContain('markdown-ansi');
    expect(html).toContain('color:#00aa00');
    expect(html).toContain(`data-copy-source="${ESC}[32mART\n"`);
    const plain = await renderMarkdown('```text\n' + ESC + '[32mART\n```');
    expect(plain).not.toContain('markdown-ansi');
  });

  it('limits artwork per message and falls back to source for oversized blocks', async () => {
    const html = await renderMarkdown(Array.from({ length: 5 }, () => fence('art')).join('\n\n'));
    expect(html.match(/markdown-ansi/g)).toHaveLength(4);
    expect(html.match(/data-copy-source=/g)).toHaveLength(5);
    expect(await renderMarkdown(fence('x'.repeat(MAX_SOURCE_LENGTH + 1)))).not.toContain(
      'markdown-ansi'
    );
  });
});
