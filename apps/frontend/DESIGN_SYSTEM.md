# Chatto Frontend Design System

This guide is the canonical entry point for implementing visible UI in the
Chatto frontend. Storybook is the visual catalog; this document explains how
to choose and extend its primitives.

Run `mise storybook` from the repository root to browse the catalog.

## Working Order

Before changing visible UI:

1. Inspect the relevant Storybook category and the nearest equivalent product
   surface.
2. Prefer an established Svelte component.
3. If no component fits, prefer a semantic utility from `src/app.css`.
4. Use raw Tailwind utilities for local layout and responsive composition.
5. Add or extend a primitive when the same visual or interaction recipe would
   otherwise be repeated.
6. Verify every applicable state: light and dark theme, narrow and wide layout,
   hover, focus-visible, pressed, disabled, loading, empty, and error.
7. Update the reusable component's Storybook story when its API or appearance
   changes.

This order is a decision aid, not a ban on native elements. Specialized chat,
media, menu, and toolbar controls often need native buttons combined with a
semantic utility because their behavior is not a committed form action.

## Choosing A Primitive

| Need                                      | Use                                                                        | Avoid                                                        |
| ----------------------------------------- | -------------------------------------------------------------------------- | ------------------------------------------------------------ |
| Committed text action or button-like link | `Button` from `$lib/ui/form`                                               | Rebuilding `btn-*` recipes in feature code                   |
| Form field                                | `TextInput`, `TextArea`, `Select`, `Combobox`, `Checkbox`, or `RangeField` | Raw controls unless the interaction is genuinely specialized |
| One-of-many settings choice               | `ChoiceRow` inside a `radiogroup`                                          | Repeating indicator and selected-state markup                |
| Compact one-of-many mode                  | `SegmentedControl`                                                         | Separate buttons or independently styled chips               |
| Modal form                                | `FormDialog`                                                               | A dialog containing an unrelated hand-rolled form footer     |
| Confirmation                              | `ConfirmDialog`                                                            | A custom destructive modal                                   |
| General dialog                            | `Dialog`; `BottomSheet` for touch-specific presentation                    | Fixed-position modal shells                                  |
| Floating menu or tooltip                  | `ContextMenu`, `HelpTooltip`, or `FloatingPopover`                         | Hand-written fixed positioning and z-index                   |
| Standard pane page                        | `PageTitle`, `PaneHeader`, `PaneContent`, and titled `Panel` sections      | Hand-rolled page widths, scrolling, and section cards        |
| Pane title and toolbar                    | `PaneHeader` with `HeaderIconButton` actions                               | Textual primary actions in the pane header                   |
| Inline icon action                        | `icon-action`                                                              | Repeating hit-area, hover, and pressed classes               |
| Global app-header icon                    | `app-header-icon`                                                          | `icon-action` with compensating margins                      |
| Durable content container                 | `Panel` or `panel-shell`                                                   | Ad hoc card borders, radius, and elevation                   |
| Compact nested row                        | `surface-box`                                                              | A panel nested inside another panel                          |
| Status or scope label                     | `Pill`; `ToggleChip` when independently interactive                        | One-off colored badges                                       |
| Inline contextual notice                  | `Hint`                                                                     | A panel used as an alert                                     |
| Transient feedback                        | `toast`                                                                    | Persistent inline copy that disappears automatically         |
| Empty collection or search result         | `EmptyState`                                                               | Bespoke centered placeholder markup                          |
| Loading image                             | `SkeletonImg`                                                              | `<img class="skeleton">`                                     |

## Standard Pane Pages

Use the pane-page composition for primary application pages such as search,
settings, and Server Admin. It gives these pages the same header, scrolling
behaviour, content width, spacing, and panel hierarchy.

```svelte
<PageTitle title={pageTitle} />

<div class="pane-page">
  <PaneHeader title={pageTitle} subtitle={pageSubtitle} />

  <PaneContent>
    <div class="flex flex-col gap-6">
      <Panel title={formTitle}>
        <form><!-- padded form content --></form>
      </Panel>

      <Panel title={resultsTitle} noPadding>
        <!-- edge-to-edge list, table, or result state -->
      </Panel>
    </div>
  </PaneContent>
</div>
```

Follow these defaults:

- `PageTitle` owns the browser title. `PaneHeader` owns the visible page title,
  optional subtitle, back affordance, and icon actions.
- Keep the outer `pane-page` wrapper. This semantic utility lets the pane shrink
  inside the application shell without creating an accidental second page
  scrollbar.
- Let `PaneContent` own scrolling, the `max-w-5xl` content width, and page
  padding. Do not reproduce those constraints in each route.
- Stack peer sections with `flex flex-col gap-6`. Use a tighter gap only for a
  deliberately dense surface, not as a page-by-page styling choice.
- Give peer panels short, descriptive titles. A form panel names the task or
  input group; a list panel names the collection. If a panel title repeats a
  single form field's visible label, keep the field label available to
  assistive technology with the field component's `labelHidden` option.
- Use the default padded `Panel` for forms, prose, summaries, and grouped
  controls. Use `noPadding` for tables, lists, search results, and other
  edge-to-edge collections; the child owns its row padding and dividers.
- Render loading, error, and empty states inside the panel whose content they
  replace. A single full-page availability state may use one untitled panel
  because there are no peer sections to distinguish.
- Use `fillHeight` on both `PaneContent` and the single primary `Panel` when a
  dense table or editor should consume the remaining pane height. Ordinary
  forms and document-like pages should remain content-sized.
- Do not nest `Panel` components. Use `surface-box` for compact structure
  inside a panel, and place page-level `Hint` notices above the affected panel.

Panel titles are structural navigation, not decorative headings. Do not omit
them merely because the page header already names the overall feature: the
page title answers “where am I?”, while panel titles answer “what is in this
section?”.

## Semantic Color Language

Use semantic tokens instead of Tailwind palette colors for application chrome.
Media overlays may use literal black and white where contrast must be
independent of the active theme.

| Meaning                                    | Canonical token  |
| ------------------------------------------ | ---------------- |
| Recommended action, selection, focus, link | `action`         |
| Neutral emphasized control                 | `neutral-action` |
| Positive state                             | `success`        |
| Caution                                    | `warning`        |
| Destructive or failed state                | `danger`         |
| Form validation failure                    | `error`          |
| Server identity                            | `server`         |

The token name describes intent, not visual intensity. Use `action` for the
recommended path and `neutral-action` for an emphasized control that should not
compete with it. Retired `accent` and `primary` color utilities are rejected by
the design-system guardrail.

Compact filled controls pair each tone with its `on-*` foreground token.
Prominent action, success, warning, and danger buttons use dedicated fills with
white labels. Light-theme action buttons share the friendly action blue; dark
mode uses deeper button fills so white labels remain clear without darkening
the brighter semantic tones used for links, focus rings, and status UI.

Surfaces form a small semantic ladder:

- Light and dark mode are intentionally asymmetric. Do not infer elevation by
  mechanically reversing luminance between themes.
- In light mode, `background` is the pale primary work plane and `surface` is
  the cool gray used for anchored chrome, composers, user cards, dialogs, and
  panel frames and headers. These surfaces read as inset and substantial, not
  as white paper floating above the application.
- In dark mode, progressively lighter surfaces provide separation from the
  dark primary plane.
- `surface-emphasized` separates hover states and nested rows from their
  surrounding surface.
- `surface-strong` provides firmer contrast for compact framed UI.
- `surface-selected` is reserved for persistent selection. Pair it with an
  action-colored indicator when selection must be obvious at a glance.
- In light mode, reserve white for form fields or an explicitly reviewed
  paper-like surface; do not use it as the default fill for persistent
  application chrome.

Panel shells provide a `surface` frame around a `background` work plane. Panel
and table headers also use `surface`, keeping padded forms, row rules, nested
controls, and the outer frame visually distinct. Sticky table cells must match
the body background. This contrast is structural, not an additional surface
level.

`Panel` owns the shared inset geometry for admin content. Titled panels frame a
clipped `panel-inset` work plane with `px-1 pb-1`; omitting top padding prevents
the frame gap from visually adding to the title band's bottom padding. Untitled
edge-to-edge panels use the same rule so the gap does not add to a table header's
top padding. Untitled padded panels retain `p-1`. Custom shells such as draggable
room groups must compose the same structure instead of approximating it.
`DataTable` owns only its scrollable table viewport. Dense matrices may keep an
intrinsic content width inside it; ordinary record tables fill it. Standard
record-table headings use `table-header-cell`; matrix headings remain bespoke
because their vertical labels have different spatial needs.

Panel title bands use `px-6 py-3`. The horizontal inset aligns titles with
`p-5` panel content after accounting for the frame, while keeping the band
compact. Panels in scrolling flex columns must not shrink below their content.
Room-directory group cards use `Panel` as well; reusable product surfaces must
not assemble `panel-shell` directly when the shared component can express them.
Hoverable rows inside panel insets use the quiet `surface/70` treatment and a
`rounded-md` radius; `surface-emphasized` is too strong for transient hover.

Do not infer a new numeric surface level. Choose the nearest semantic role, or
adjust the owning component when the hierarchy itself is wrong.

For text, use `text-text` for normal copy, `text-text-top` for the strongest
heading contrast, and `text-muted` for metadata. Use `link` for inline links.

## Components, Utilities, And Tailwind

Components own behavior, semantics, accessibility, and visual variants.
Semantic utilities own reusable visual recipes for native or highly
specialized elements. Raw Tailwind owns local layout.

Caller-provided classes may control placement and composition, such as width,
margin, flex behavior, or responsive visibility. Do not use `!` overrides to
change a component's color, density, radius, typography, or interaction state.
If a legitimate variant is missing, add it to the component and its story.

Do not add a Svelte `<style>` block for ordinary component styling. Scoped CSS
is appropriate for keyframes, pseudo-elements, browser-specific behavior, or
third-party content that cannot be expressed clearly with established
utilities. Before adding one, prefer a named utility or narrowly scoped global
recipe in `src/app.css` when the behavior is reusable. The reviewed exception
list lives in `scripts/check-design-system.mjs`; adding to it requires a comment
in the component explaining why Tailwind or a semantic utility is insufficient.

## Action Hierarchy

- `Button` defaults to `variant="action"` for the recommended action.
- Use `neutral` for neutral emphasis, `secondary` for cancellation or quiet
  alternatives, and `ghost` for low-emphasis commands.
- Use `warning` or `danger` when the action itself carries that meaning.
- Use `danger-secondary` when a destructive action must remain visually quiet
  until hover or focus.
- Use Save buttons only for multi-field forms submitted together, and disable
  them until the form is dirty.
- Binary settings in Server Admin save immediately and confirm through a toast.

`Button` is not the universal representation of every clickable control.
Menus, compact chat hover bars, media overlays, and icon toolbars use their
context-specific primitive.

The supported variants are `action`, `neutral`, `secondary`, `ghost`,
`warning`, `danger`, and `danger-secondary`. Use the variant whose meaning
matches the action.

## Shape, Type, And Motion

- `rounded` and `rounded-md` are the default for compact controls, fields,
  nested rows, pills, and embedded content.
- `rounded-lg` is reserved for menus, dialogs, panels, and major shells.
- `rounded-xl` is exceptional and should communicate a deliberately softer
  product-specific object, such as a server tile—not an ordinary card.
- Nested rounded surfaces should be concentric when their padding is small.
- Base text is the default. Use `text-sm` for secondary copy and `text-xs` for
  metadata, timestamps, and terse labels.
- A compact surface uses one text size throughout. Menus, popovers, controls,
  and nested rows must not mix smaller metadata text with base-sized actions;
  express hierarchy with color, weight, spacing, and icons instead.
- Headings use balanced wrapping; short body copy uses pretty wrapping.
- Updating numeric columns and counters use `tabular-nums`.
- Interactive transitions must name their properties. Never use Tailwind's
  bare `transition` utility or `transition-all`.
- Press feedback uses `active:scale-[0.96]` where it does not interfere with
  drag, resize, or text-selection behavior.
- Respect `prefers-reduced-motion` for non-essential animation.
- Keep interactive hit areas at least 40 by 40 pixels unless a dense desktop
  toolbar has a documented non-overlapping exception.

Chatto deliberately uses browser/platform text rendering. Do not add global
font smoothing. Ordinary controls are solid rather than gradient-filled;
borders define structure, and shadows are reserved for genuinely floating or
raised surfaces. Do not use decorative one-sided accent borders or inset edge
stripes on cards, rows, panels, or selected states. When a boundary is needed,
keep it uniform around the element; communicate selection with fill and the
control's indicator.

## Storybook Contract

Every public reusable component under `src/lib/ui`, `src/lib/ui/form`, and
`src/lib/components/admin` should have a story. Internal helpers may omit one
when their parent component demonstrates the behavior.

Stories should:

- show realistic variants and important states;
- work in both light and dark theme;
- use `asChild` for stories containing markup;
- include narrow-layout examples for responsive primitives;
- keep fixture copy literal and local to the story.

Literal story fixture copy is exempt from application Paraglide catalogs.
Strings added to production components and routes are not exempt and require
British English and German messages, plus US English overrides where wording
differs.

## Regression Coverage

- Storybook is the component-state catalog. Add stories for reusable public
  components and cover meaningful variants, disabled/loading/error states, and
  narrow layouts where applicable.
- `e2e/accessibility.test.ts` scans representative public, chat, settings,
  mobile, admin, and dialog states against WCAG A/AA axe rules. Fix violations
  at their source; do not add blanket exclusions.

## Public Surface

Import public primitives from `$lib/ui`, form primitives from `$lib/ui/form`,
and toast APIs from `$lib/ui/toast`. Direct `.svelte` imports are reserved for
internal helpers and type-only imports that are not re-exported.

When adding a public primitive:

1. Export it from the appropriate index.
2. Add a component-level usage comment for non-obvious behavior.
3. Add or update its Storybook story.
4. Add a browser component test when DOM behavior, context, focus, or Svelte
   runtime behavior could regress.
5. Run the Svelte autofixer, relevant tests, and the Storybook build.

## Exceptions

Raw palette colors, arbitrary dimensions, fixed positioning, and component
style blocks are expected in a few specialized areas: media overlays, rich-text
editing, viewport/safe-area chrome, and content whose geometry comes from
external media. Keep those exceptions local and document why the semantic
system does not apply.
