/** Check synthetic importer output against Chatto's Markdown engine and link policy. */
import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { createRequire } from 'node:module';
import { fileURLToPath } from 'node:url';
import { configureMarkdownLinks } from '../apps/frontend/src/lib/markdownLinks.ts';

const root = fileURLToPath(new URL('../', import.meta.url));
const requireFrontend = createRequire(new URL('../apps/frontend/package.json', import.meta.url));
const MarkdownIt = requireFrontend('markdown-it');
const markdown = new MarkdownIt({ html: false, linkify: true, breaks: true });
markdown.disable(['lheading', 'hr', 'reference', 'image', 'html_inline', 'escape']);
configureMarkdownLinks(markdown);
// Match Chatto's double-underscore rendering rule; this harness checks parsing,
// not the frontend's full renderer or mention presentation.
for (const type of ['strong_open', 'strong_close']) {
  markdown.renderer.rules[type] = (tokens, index, options, _env, self) => {
    if (tokens[index].markup === '__') tokens[index].tag = 'u';
    return self.renderToken(tokens, index, options);
  };
}

// No export is read. Python generates only these synthetic conversion cases.
const generated = spawnSync(
  'python3',
  [
    '-B',
    '-c',
    `import sys,json
sys.path.insert(0,'tools')
from slack_apply import escape,code_span,rich_link,attachment_summary
from test_slack_rich_text import render,section,text
print(json.dumps({
 'attachment':attachment_summary({'title':'Example','title_link':'https://www.example.org/a(b)?x=1&y=2'},False),
 'shared':attachment_summary({'is_msg_unfurl':True,'author_name':'Example','text':'Quoted question?','fallback':'Duplicated question','original_url':'https://example.slack.com/archives/C1/p123'},False),
 'underline':render(section(text('moo',underline=True))),
 'underlineBold':render(section(text('moo',underline=True,bold=True))),
 'underlineItalic':render(section(text('moo',underline=True,italic=True))),
 'underlineCode':render(section(text('moo',underline=True,code=True))),
 'underlineLink':render(section({'type':'link','url':'https://example.org','text':'moo','style':{'underline':True}})),
 'literal':escape('*literal* &amp; <img> [label](url)'),
 'code':code_span('a\\u0060b'),
 'link':rich_link('https://example.org/a(b)?q=x y','[label]'),
 'phone':rich_link('tel:+123','Call'),
 'entityLink':rich_link('https://example.org/?x=&copy;&y=2','Query'),
 'list':render({'type':'rich_text_list','style':'ordered','offset':99,'elements':[section(text('parent'))]},
               {'type':'rich_text_list','style':'bullet','indent':1,'elements':[section(text('child'))]}),
 'quote':render({'type':'rich_text_quote','elements':[text('one\\n\\ntwo')]}),
 'fence':render({'type':'rich_text_preformatted','elements':[text('a\\n\\u0060\\u0060\\u0060\\nb')]})
}))`
  ],
  { cwd: root, encoding: 'utf8' }
);
assert.equal(generated.status, 0, generated.stderr);
const cases = JSON.parse(generated.stdout);
assert.match(
  markdown.render(cases.attachment),
  /href="https:\/\/www.example.org\/a\(b\)\?x=1&amp;y=2"/
);
const shared = markdown.render(cases.shared);
assert.match(shared, /<blockquote>/);
assert.equal((shared.match(/Quoted question/g) || []).length, 1);
assert.doesNotMatch(shared, /Duplicated question|layout and embedded media/);
assert.equal(markdown.renderInline(cases.underline), '<u>moo</u>');
assert.equal(markdown.renderInline(cases.underlineBold), '<u><strong>moo</strong></u>');
assert.equal(markdown.renderInline(cases.underlineItalic), '<em><u>moo</u></em>');
assert.equal(markdown.renderInline(cases.underlineCode), '<u><code>moo</code></u>');
assert.match(markdown.renderInline(cases.underlineLink), /^<u><a .*">moo<\/a><\/u>$/);
const literal = markdown.render(cases.literal);
assert.match(literal, /\*literal\* &amp;amp; &lt;img&gt; \[label\]\(url\)/);
assert.doesNotMatch(literal, /<em>|<img|<a /);
assert.match(markdown.render(cases.code), /<code>a`b<\/code>/);
assert.match(markdown.render(cases.link), /href="https:\/\/example.org\/a\(b\)\?q=x%20y"/);
assert.match(markdown.render(cases.link), />\[label\]<\/a>/);
assert.doesNotMatch(markdown.render(cases.phone), /<a /);
assert.match(
  markdown.render(cases.entityLink),
  /href="https:\/\/example.org\/\?x=&amp;copy;&amp;y=2"/
);
assert.match(markdown.render(cases.list), /<ol start="100">\s*<li>parent\s*<ul>/);
assert.match(markdown.render(cases.quote), /<blockquote>/);
assert.match(markdown.render(cases.fence), /<pre><code>a\n```\nb\n<\/code><\/pre>/);
console.log('Slack Markdown rendering checks passed (synthetic data only).');
