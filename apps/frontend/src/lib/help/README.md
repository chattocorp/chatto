# Help authoring

Help has four guides: general usage (`conversations`), formatting, bots, and
phone installation. Keep non-obvious everyday behaviour in the general guide;
do not add separate articles for each setting or repeat basic chat instructions.

The articles are English review drafts. Other locales fall back to English until
the reviewed articles are translated. Navigation labels remain localised. The
earlier translated prototype articles have been removed so readers do not receive
different instructions when they change language.

Write article content in `content/<locale>/<topic>.mdx`. MDsveX compiles these
files into Svelte components during the frontend build. The `.mdx` extension
uses Markdown and Svelte syntax, not React JSX.

Use ordinary Markdown for paragraphs, headings, lists, links, and code blocks.
The build generates section anchors and a table of contents from top-level `##`
and `###` headings. Quoted headings, code blocks, and embedded component headings
do not appear in that list. Keep heading names stable when possible: changing a
heading changes its section link. Duplicate names get a numeric suffix.

Start article sections at `##`; the route supplies the page title. Import an
interactive example in a `<script>` block and place its component where the
reader needs it:

```mdx
<script>import FormattingExample from '$lib/help/FormattingExample.svelte';</script>

## Try message formatting

Change the text and compare it with the preview.

<FormattingExample />
```

Keep example logic in Svelte components. Examples must work without an active
server and must not send messages or change account settings. Article files are
trusted application source; do not compile user-submitted Markdown as MDsveX.

Keep titles, card summaries, and navigation labels in `messages/<locale>/help.json`
so the overview does not download article bodies. Each article and language is a
separate dynamic import. The compiler does not ship to the browser. Regional
locales use the same inheritance as the UI catalogs: US English uses British
English, and Austrian and Swiss German use German unless an article overrides it.

Run the frontend check and production build after editing documents. The build
compiles all articles, including languages that you did not open in the browser.
Review the rendered article and exercise its examples. A successful build does
not establish that the instructions are accurate or useful.
