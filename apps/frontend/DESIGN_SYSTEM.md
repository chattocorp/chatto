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

| Need                                      | Use                                                          | Avoid                                                        |
| ----------------------------------------- | ------------------------------------------------------------ | ------------------------------------------------------------ |
| Committed text action or button-like link | `Button` from `$lib/ui/form`                                 | Rebuilding `btn-*` recipes in feature code                   |
| Form field                                | `TextInput`, `TextArea`, `Select`, `Combobox`, or `Checkbox` | Raw controls unless the interaction is genuinely specialized |
| One-of-many settings choice               | `ChoiceRow` inside a `radiogroup`                            | Repeating indicator and selected-state markup                |
| Modal form                                | `FormDialog`                                                 | A dialog containing an unrelated hand-rolled form footer     |
| Confirmation                              | `ConfirmDialog`                                              | A custom destructive modal                                   |
| General dialog                            | `Dialog`; `BottomSheet` for touch-specific presentation      | Fixed-position modal shells                                  |
| Floating menu or tooltip                  | `ContextMenu`, `HelpTooltip`, or `FloatingPopover`           | Hand-written fixed positioning and z-index                   |
| Pane title and toolbar                    | `PaneHeader` with `HeaderIconButton` actions                 | Textual primary actions in the pane header                   |
| Inline icon action                        | `icon-action`                                                | Repeating hit-area, hover, and pressed classes               |
| Global app-header icon                    | `app-header-icon`                                            | `icon-action` with compensating margins                      |
| Durable content container                 | `Panel` or `panel-shell`                                     | Ad hoc card borders, radius, and elevation                   |
| Compact nested row                        | `surface-box`                                                | A panel nested inside another panel                          |
| Status or scope label                     | `Pill`; `ToggleChip` when interactive                        | One-off colored badges                                       |
| Inline contextual notice                  | `Hint`                                                       | A panel used as an alert                                     |
| Transient feedback                        | `toast`                                                      | Persistent inline copy that disappears automatically         |
| Empty collection or search result         | `EmptyState`                                                 | Bespoke centered placeholder markup                          |
| Loading image                             | `SkeletonImg`                                                | `<img class="skeleton">`                                     |

## Semantic Color Language

Use semantic tokens instead of Tailwind palette colors for application chrome.
Media overlays may use literal black and white where contrast must be
independent of the active theme.

| Meaning                                    | Canonical token  | Compatibility name      |
| ------------------------------------------ | ---------------- | ----------------------- |
| Recommended action, selection, focus, link | `action`         | `accent`                |
| Neutral emphasized control                 | `neutral-action` | `primary`               |
| Positive state                             | `success`        | —                       |
| Caution                                    | `warning`        | —                       |
| Destructive or failed state                | `danger`         | `error` for form errors |
| Server identity                            | `server`         | —                       |

`primary` historically means a muted neutral in Chatto. It does **not** mean
the recommended action. New code should use `action` or `neutral-action` so the
intent is obvious. The compatibility names remain available while existing
call sites migrate.

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
`warning`, `danger`, and `danger-secondary`. Historical `primary` and `accent`
button names are not supported; use the variant whose meaning matches the
action.

## Shape, Type, And Motion

- `rounded` and `rounded-md` are for compact controls, pills, and embedded
  content.
- `rounded-lg` is the default for fields, menus, dialogs, and substantial
  controls.
- `rounded-xl` is reserved for panels and major shells.
- Nested rounded surfaces should be concentric when their padding is small.
- Base text is the default. Use `text-sm` for secondary copy and `text-xs` for
  metadata, timestamps, and terse labels.
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
font smoothing. Chatto also uses borders for structure and quiet gradients or
shadows for selected elevated surfaces; do not replace one with the other as a
blanket rule.

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
English and German messages.

## Regression Coverage

- Storybook is the component-state catalog. Add stories for reusable public
  components and cover meaningful variants, disabled/loading/error states, and
  narrow layouts where applicable.
- `DesignSystem.visual.svelte.spec.ts` protects the shared primitive language in
  light/dark and true desktop/mobile viewports. Its committed baselines are
  platform-specific because browser font rasterization differs across operating
  systems. Update them only after visually reviewing the intended change.
- `e2e/accessibility.test.ts` scans representative public, chat, settings,
  mobile, admin, and dialog states against WCAG A/AA axe rules. Fix violations
  at their source; do not add blanket exclusions.
- Run `pnpm run test:visual --update` from `apps/frontend` to intentionally
  refresh visual baselines, then run it again without `--update` to prove the
  snapshots are stable.

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
