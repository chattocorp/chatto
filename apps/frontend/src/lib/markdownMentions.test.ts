/**
 * Mention tokenization in the message renderer. The shared cases in
 * `testdata/mentions/extraction.json` also run against the server's mention
 * extraction, so both implementations must agree on every case.
 */
import { describe, it, expect } from 'vitest';
import sharedCases from '../../../../testdata/mentions/extraction.json';
import { extractMarkdownMentions, renderInlineMarkdown, renderMarkdown } from './markdown';

/** Returns the mention candidate handles in rendered HTML, in document order. */
function renderedCandidates(html: string): string[] {
  return [...html.matchAll(/<span data-mention-handle="([^"]*)">@\1<\/span>/g)].map(
    (match) => match[1]
  );
}

describe('shared mention extraction cases', () => {
  it.each(sharedCases.cases)('$name', async ({ body, mentions }) => {
    expect(extractMarkdownMentions(body)).toEqual(mentions);

    // The renderer marks the same handles (before deduplication).
    const rendered = [
      ...new Set(renderedCandidates(await renderMarkdown(body, { mentions: true })))
    ];
    expect(rendered).toEqual(mentions);
  });
});

describe('mention rendering', () => {
  it('marks a mention as an unresolved candidate', async () => {
    expect(await renderMarkdown('Hi @alice!', { mentions: true })).toBe(
      '<p>Hi <span data-mention-handle="alice">@alice</span>!</p>\n'
    );
  });

  it('keeps a handle in a linkified URL as literal link text', async () => {
    const url = 'https://social.5f9.de/@jollyorc/117331230238178837';
    const html = await renderMarkdown(`Zum Thema ${url}`, { mentions: true });
    expect(html).toContain(`>${url}</a>`);
    expect(html).not.toContain('data-mention-handle');
  });

  it('keeps a handle in link text as literal text', async () => {
    const html = await renderMarkdown('[@alice](https://example.com)', { mentions: true });
    expect(html).toContain('>@alice</a>');
    expect(html).not.toContain('data-mention-handle');
  });

  it('does not mark mentions in code or blockquotes', async () => {
    const html = await renderMarkdown('`@alice`\n\n> @bob\n\n```\n@carol\n```', { mentions: true });
    expect(html).not.toContain('data-mention-handle');
    expect(html).toContain('<code>@alice</code>');
    expect(html).toContain('@bob');
  });

  it('marks every occurrence of a repeated mention', async () => {
    expect(
      renderedCandidates(await renderMarkdown('@alice and @alice', { mentions: true }))
    ).toEqual(['alice', 'alice']);
  });

  it('keeps mentions as plain text unless message mentions are requested', async () => {
    expect(await renderMarkdown('Ask @alice')).toBe('<p>Ask @alice</p>\n');
    expect(renderInlineMarkdown('Ask @alice')).toBe('Ask @alice');
  });
});
