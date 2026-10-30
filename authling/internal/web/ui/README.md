# UI primitives

This package owns small, server-rendered templ components that do not depend on
Authling accounts or services. Shared styles live in `web/src/app.css` under
`@layer components`. Keep one-off layout utilities in the calling page.

The [design system](../../../docs/design-system.md) defines tokens, type, spacing,
layouts, and verification. Preview components at `/dev/ui` in a `bootstrap` build.

- `Heading` supplies the page title; children supply its introduction.
- `SectionHeading` supplies a panel title below the page heading.
- `Field` pairs a label, native input, and optional accessible hint. IDs must be
  unique. Pass native attributes for type, name, autocomplete, and validation.
  Do not override its ID, class, or hint relationship in the attributes map.
- `CodeField` supplies one native six-digit input with decorative cells.
- `Notice` supplies status or alert semantics for `Neutral`, `Success`, `Warning`,
  or `Danger` feedback. `Error` omits itself when its message is empty.

- `DataTable` supplies the caption, column headers, and a keyboard-accessible
  horizontal scroll region. The caller supplies native table rows and cells.
  Use a row header with `scope="row"` for the row's identifying value.
- `TableEmpty` supplies a message across all columns when no rows exist.
- `Badge` displays a short status. It is not an interactive control.
- `Pagination` displays available first-page and next-page links. The calling
  handler owns cursor validation and constructs the URLs.

Keep data loading, access checks, column values, and pagination state in the
feature handler and template. Do not add a generic column schema or JavaScript
table engine to render ordinary HTML. Add sorting or filters through server
query parameters when a feature needs them. These components work without
JavaScript and use the existing light/dark palette and local font.

Regenerate renderers with `mise codegen-templates` and rebuild CSS with
`mise build-web`, from `authling/`, after changing templates or styles.
