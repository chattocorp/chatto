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

## Selectable Record Collections

Selectable or directly actionable records use the same inset-box treatment as
message search results:

- Use `selectable-list` on non-table collections and `selectable-list-item` on
  each navigable, draggable, or directly actionable record.
- Use `ActivityListRow` for newest-first activity records that have one primary
  open action and optional row actions. It owns the shared read-state styling,
  important fill, pending state, and hover-reveal action shell.
- Rows rest transparently on the owning `background` work plane. Hover and
  keyboard focus rise exactly one level to `surface`; do not introduce ruled
  separators or a stronger surface jump.
- Each row owns its rounded shape. The collection owns only the standard
  `p-1` inset and `gap-1`, so selections never merge into a single slab.

## Sidebar Navigation

### User identity cards

Use `UserCard` for member entries, the current-user card, and call participant
headers. It owns identity alignment, name truncation, secondary text, and
action placement. All variants use a 3 rem identity row, the same padding, and
a semibold display name. Pass `username` to show the isolated `@username`
below the name. Use `row` for a shell surface only on hover or keyboard focus,
`card` for a permanent shell surface, and `plain` inside an existing card.
Call cards use the same shell surface without an extra border. Supply avatar,
badge, indicator, action, and optional body snippets. Avatar snippets normally use `UserAvatar`
at size `sm`.

Pass `identityAttributes` for a clickable identity. Put independent controls in
the `actions` snippet so buttons never nest. Pass `menu` with a label, click
callback, and expanded state for the standard three-dot button. Member entries
use this button and leave identity text passive. Use `menu.revealOnHover` to
hide the button until hover or keyboard focus; touch devices keep it visible.
Use `menu.oncontextmenu` for right-click access from the whole card.
The identity row cannot shrink below the shared control height. For the current-user card, omit
the identity button and supply the presence button in `avatar`. Callers own
profile menus, presence lookup, permissions, audio-level sources, and media
lifecycles; the shared component does not read application stores.

Pass `voiceLevel` as a getter returning a normalized 0–1 microphone level to
show voice activity. The internal `VoiceActivity` canvas stays behind the
identity row, so video and screen shares remain clear. Distorted two-dimensional
simplex noise forms three translucent layers of broad fog wisps. Each layer has
its own scale, seed, and drift direction; source-over composition lets them
overlap without becoming an opaque fill.
The renderer enlarges a bounded alpha texture and tints it with the accent colour.
Microphone volume controls opacity and spread, with a quick rise and a gentle
fade. Smoothed volume also controls travel speed: quiet speech drifts and louder
speech moves faster, without position jumps when the level changes. The fog
disappears in silence. Reduced motion fixes the texture
and updates intensity without animation. All layers share a sampling clock and an
animation loop; silent and offscreen cards do not request animation frames.
Do not add a separate speaking border or pulse to the card.

### Sidebar links

Below the `md` breakpoint, the drawer fills the available frame width. The
server gutter keeps its width and the navigation pane fills the remaining
space. Both panes inherit one animation state through `sidebar-drawer`.
Mouse drawers use the thread overlay's 300 px slide and fade. Touch drawers
move the full frame width to follow a finger. Select a destination, use the app-header toggle, or swipe towards
the inline start to close the drawer. Desktop sidebar sizing is unchanged.

Sidebar links use `sidebar-item`. Set `aria-current="page"` on the current
route. The shared primitive then uses a quiet action-coloured fill and an
action-coloured icon. Current links and selected menu rows use flat fills,
without gradients, bevels, or shadows. A selected pane-header icon button
uses the same flat action fill and action-coloured icon. It marks the panel
that the button shows. On/off settings change the icon and label; they do not
use a fill. Apply `sidebar-item-attention`
only to unread content that is not the current route. Unread dots remain neutral. Notification badges
keep their semantic priority colour.

When one route path contains another navigation path, set `aria-current` only
on the most-specific matching item.

The server gutter has a distinct rule. The current server uses a two-pixel
text-coloured ring and a `surface-selected` surround. Keep the server artwork unchanged. Selection
stays separate from the accent-coloured keyboard focus outline. Do not add
a side stripe or cast shadow for navigation selection.

## Choosing A Primitive

| Need                                      | Use                                                                        | Avoid                                                        |
| ----------------------------------------- | -------------------------------------------------------------------------- | ------------------------------------------------------------ |
| Committed text action or button-like link | `Button` from `$lib/ui/form`                                               | Rebuilding `btn-*` recipes in feature code                   |
| Form field                                | `TextInput`, `TextArea`, `Select`, `Combobox`, `Checkbox`, or `RangeField` | Raw controls unless the interaction is genuinely specialized |
| Short, visible settings choice list        | `ChoiceRow` inside a `radiogroup`                                          | Repeating indicator and selected-state markup                |
| Compact choice from a long or variable list | `Select`                                                                 | Expanding every device or option into a separate row          |
| Compact one-of-many mode                  | `SegmentedControl`                                                         | Separate buttons or independently styled chips               |
| Selectable non-table collection           | `selectable-list` and `selectable-list-item`                               | Feature-local hover recipes                                  |
| Newest-first activity record              | `ActivityListRow` inside `selectable-list`                                 | Repeating row, unread, pending, and action-shell recipes     |
| Modal form                                | `FormDialog`                                                               | A dialog containing an unrelated hand-rolled form footer     |
| Confirmation                              | `ConfirmDialog`                                                            | A custom destructive modal                                   |
| General dialog                            | `Dialog`; `BottomSheet` for touch-specific presentation                    | Fixed-position modal shells                                  |
| Floating menu or tooltip                  | `ContextMenu`, `HelpTooltip`, or `FloatingPopover`                         | Hand-written fixed positioning and z-index                   |
| Context-menu command                      | `MenuItem` inside `MenuSection`                                            | `sidebar-item` or repeated icon and state markup             |
| Standard pane page                        | `PageTitle`, `PaneHeader`, `PaneContent`, and titled `Panel` sections      | Hand-rolled page widths, scrolling, and section cards        |
| Pane title and toolbar                    | `PaneHeader` with `HeaderIconButton` actions                               | Textual primary actions in the pane header                   |
| Inline icon action with standard hit area | `icon-action`                                                              | Repeating hit-area, hover, and pressed classes               |
| Mini icon action directly beside a value  | `mini-icon-action`                                                         | Adding padding, a background fill, or press scaling          |
| Global app-header icon                    | `app-header-icon`                                                          | `icon-action` with compensating margins                      |
| Durable content container                 | `Panel` or `panel-shell`                                                   | Ad hoc card borders, radius, and elevation                   |
| Compact nested row                        | `surface-box`                                                              | A panel nested inside another panel                          |
| Status or scope label                     | `Pill`; `ToggleChip` when independently interactive                        | One-off colored badges                                       |
| Inline contextual notice                  | `Hint`                                                                     | A panel used as an alert                                     |
| Transient feedback                        | `toast`                                                                    | Persistent inline copy that disappears automatically         |
| Empty collection or search result         | `EmptyState`                                                               | Bespoke centered placeholder markup                          |
| Loading content                           | `LoadingFog` sized to the content area                                      | Rows shaped like future content                              |
| Loading image                             | A stable image frame with `LoadingFog` until load, then the existing fallback on error | An image with no reserved size                     |

`Select` uses a native control and plain-text options. The shared
`select-control` utility styles the picker where `appearance: base-select`
is supported. Other browsers keep their platform picker. Long menus scroll
and long option labels wrap within the viewport. The closed control uses
`control-raised`; the picker uses `floating-frame`. Both follow the depth
preference. Option rows keep flat selection and hover fills. Use `bind:value` for local
form values. Use `value` with `onValueChange` when the caller must apply a
change before it becomes the committed selection. The callback owns error
feedback; the control blocks further changes while pending and restores the
caller's value after completion or failure.

`Checkbox` and `ChoiceRow` use the same option-row shape and selected fill.
Use the square check indicator for an independent boolean setting. Use the
circular radio indicator for one choice in a group. Rows use a faint raised
edge. Empty indicators look inset; selected indicators use a reduced lighting
strength with no outer shadow. Disabled controls have no raised finish. Row and indicator colour transitions
use the shared `feedback-quick` utility: instant feedback in both
directions. Sidebar links and menu rows use the same timing. Keep transition
properties on the control; use the motion tokens below to tune feedback speed.

The quick finder uses `command-palette`, which composes the shared menu frame.
It keeps a compact search field and aligned result rows through
`command-palette-result`. The active result uses a flat neutral fill and a quiet
Enter cue. `keycap` supplies the shared keyboard-hint shape.

## Attachment Description Icons

Never use a wheelchair icon for alt text or attachment descriptions. These
controls describe content; a wheelchair symbol does not communicate that action.
Use `icon-[uil--file-edit-alt]` for add and edit description actions in both the
composer and sent messages. Keep the translated action label in `aria-label`
and the tooltip, and hide the decorative icon from assistive technology.

## Joined Action Pills

Use `PillButtonGroup` for related independent actions in one shell. Each native
button uses `pill-button`; `pill-button-success` and `pill-button-danger`
supply semantic fills. Unlike `SegmentedControl`, this group does not represent
one choice. Buttons retain independent labels, disabled states, and keyboard focus.

The group owns the rounded corners. Each segment uses the same `shell-lighting`
as the composer and user card, without a second lighting layer on the group. Flat mode
uses separators; 3D adds narrow gaps, softly rounded segment corners, and faint
individual highlights. These changes follow the animated depth preference.
Do not add `btn` or local bevels to its segments. The default height is
48 px, aligned with the composer and user card. Use `compact` for a 28 px
secondary row with 15 px icons and a smaller corner radius, such as the call controls above the user card
or the composer formatting bar. Use `gaps={false}` for a continuous surface with separators and square inner
corners in every depth mode. The composer and user-card control rows use a 4 px gap above their main surface.
The composer uses this variant for three groups: inline text styles, block
formats, and lists with indentation. Only the groups have gaps between them. Formatting buttons use `aria-pressed` for the
shared active fill. Keep an intrinsic-width group inside a horizontal scroller
when the controls must stay on one row in a narrow pane.

Below 560 px of composer content width, the editor uses the full inner width.
The formatting toggle sits below it at the start of the surface. Attachment,
timestamp, thread options, and Send stay together at the end. Groups wrap in very narrow
panes. Wider composers keep inline actions. Layout and labels use the named
`composer` container.

In wider panes, a draft that grows beyond one text line also moves the actions
below the editor. Keep this layout until the draft is cleared or sent, or its
destination or edit mode changes. The extra text width can remove a line wrap;
keeping the expanded layout prevents repeated layout changes while typing.

Both composer toolbars use 44 px targets and 20 px icons in narrow touch
windows, including hybrid devices. Wide windows and mouse-only devices keep
28 px controls and 15 px icons. Composer controls opt in through
`mobile-presentation:pill-button-group-touch`. This is a composer-specific
exception to touch target sizing at every viewport width.
The formatting shelf scrolls horizontally when its controls do not fit.
In a wide, single-line composer, centre the editor and touch controls vertically.
The compact desktop controls retain their bottom offset; do not apply that offset
to the taller touch controls.

## Standard Dialogs

Use the standard dialog family for focused tasks:

- Use `ConfirmDialog` when a user must confirm one command.
- Use `FormDialog` when a dialog collects input and submits one form.
- Use `Dialog` for information, custom content, or two or more action paths.

All three components own responsive presentation. Below 768 px, task
dialogs become full-width bottom sheets only when a coarse pointer is available.
Mouse windows retain centred dialogs that fit the viewport. Sheets have a drag handle, `rounded-lg` top
corners, and safe-area spacing. Both layouts keep a `surface` frame around one
inset `background` work plane. `sheet-frame` owns the same 16 px side and bottom
surround for task dialogs and context-menu sheets, including safe-area spacing.
Do not add another outer inset in either component. Task work planes use 12 px
inner padding; desktop retains its existing frame. Feature components must not
select a mobile layout or render separate mobile content.

The header and actions stay visible while long body content scrolls. When a
short viewport or enlarged text leaves too little space, the sheet content
can scroll to keep every control reachable. Sheets follow the visual viewport
when a keyboard opens. Only the handle captures drag gestures. The body keeps
native scrolling. Resizing keeps the dialog, body, and action controls mounted.
Do not add a `Panel`, another work plane, or a footer divider inside this structure.

Dialog trays and floating menu shells use `floating-frame` for a faint top
highlight and lower shading across their padded frame. Content surfaces keep
their solid `background` fill and use `floating-inset` for a soft inner shadow
that makes them look recessed. Menu sections receive this shadow inside their
shared frame, including bottom sheets. The finish adds no backdrop blur or animation and retains
the existing outer shadow. Full-screen mobile media dialogs have no exposed
frame and do not use this finish. Standard panels share the frame highlight
and recessed content treatment. Panel title bands stay transparent so the
frame gradient continues around the content without a seam. The shared
`--frame-*` tokens reduce lower-edge shading and inset shadows in light mode;
dark mode retains the stronger depth treatment. Form input fields
keep their existing finish.

Declare task buttons in `Dialog` snippets: `primaryAction` for the main action,
`secondaryActions` for alternatives, and `dismissAction` for Cancel or Close.
Each snippet contains ordinary buttons; callers do not set responsive classes.
`FormDialog` supplies submit and Cancel. `ConfirmDialog` inherits this behavior.

Mobile actions fill the sheet width, have a minimum 48 px height, and wrap
complete labels, including loading labels. The order is primary, alternatives,
then dismissal. Desktop actions form an end-aligned row in the order dismissal,
alternatives, then primary. DOM and keyboard order follow the visible order.
Desktop labels can expand the dialog beyond its baseline width and truncate
only at the viewport limit on wide screens. Below 768 px, centred dialogs wrap
actions and complete labels to keep them readable. Keep alternative actions in
their intended order.

Use `secondary` for Cancel, `action` for the recommended path, and a semantic
tone such as `danger` only when the action has that meaning. Use an
action-specific icon on the committing action. Do not add an icon to Cancel
by default. The primary slot does not enable Enter activation: use a native
submit button or `Button`'s `defaultAction` where appropriate. Reserve `footer`
and `footerDetails` for specialized viewer controls. The prop types prevent
combining a custom footer with semantic actions. `footerDetails` requires `footer`.

Do not put large, browsable content such as the Server Directory in a dialog.
Use a frame view (see Standard Pane Pages).

Use the `sm` baseline size for short confirmations, `md` for ordinary custom
dialogs, and `lg` for dense content such as screen selection. Footer actions
can make each size wider on desktop. Keep the desktop viewport gutter. Do not add a
feature-specific width to a task dialog.

The attachment image viewer, fullscreen video, `QuickSwitcher`, and popovers
remain specialized overlays. The image viewer keeps its zoom controls outside
the pan surface and its details panel closed until the user opens it. Media
viewers keep their full-screen mobile presentation. Context
menus retain input-capability selection between floating menus and sheets.
`Dialog` and `BottomSheet` share the internal `ModalSurface` for native modal
lifecycle, focus restoration, backdrop handling, animation, and handle gestures.

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
        <!-- child-owned collection, table, or result state -->
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
  controls. Use `noPadding` when the child owns its work-plane treatment.
  `DataTable` rows are flush and use dividers. A `selectable-list` keeps its
  standard `p-1` inset and `gap-1` so separately rounded rows have clear boundaries.
- Render loading, error, and empty states inside the panel whose content they
  replace. A single full-page availability state may use one untitled panel
  because there are no peer sections to distinguish.
- Use one `LoadingFog` block for a pending content area. Give it a size that
  keeps the surrounding layout stable. Each block has its own quiet simplex-noise
  motion and an accessible busy state. It fades in briefly and leaves as soon as
  content is ready. Motion pauses when the user requests less motion or the tab
  is hidden; reduced motion also removes the fade.
- Use `fillHeight` on both `PaneContent` and the single primary `Panel` when a
  dense table or editor should consume the remaining pane height. Ordinary
  forms and document-like pages should remain content-sized.
- Do not nest `Panel` components. Use `surface-box` for compact structure
  inside a panel, and place page-level `Hint` notices above the affected panel.

Panel titles are structural navigation, not decorative headings. Do not omit
them merely because the page header already names the overall feature: the
page title answers “where am I?”, while panel titles answer “what is in this
section?”.

### Frame Views

A frame view is a history-backed pane page that fills the app frame. Use
`FrameView` for large, browsable content that the user opens from anywhere,
such as the Server Directory. It covers the Server Gutter, the sidebars, and
the main area; the app header stays available. The covered route content stays
mounted but is inert and invisible, so it keeps its state. On narrow screens,
opening a frame view closes the server drawer. The sidebar button in the app
header can open the drawer above the view.

The view has a `PaneHeader` with a close button and a scrolling `PaneContent`,
so it uses the pane-page rules above. Escape, the close button, and Back close
it. Declare the modal type with `isFrameViewModal` in `$lib/modal`; the root
layout renders it in `FrameViewContainer` instead of `ModalContainer`.

Cards that must adapt to the space they get, not to the device, use a named
container query. `ServerProfileCard` is an icon tile when its nearest
`@container/server-cards` ancestor is narrower than 40rem: a large centred
logo, the name and host, and the actions, without a banner or description.
Callers adapt their own card content with the `server-tile:` variant. The
container belongs to the grid owner, so one grid can show full cards on a
wide page and two tile columns on a phone.

### Settings And Preferences

Server Configuration, server-scoped User Preferences, and App Preferences use
the same page structure. Their different data scopes affect navigation and
state ownership, not the visual treatment of their forms.

- Put settings content in `PaneContent`; do not reproduce its scrolling,
  padding, or width classes on a route-local wrapper.
- Frame every page-level form or independently meaningful control group with a
  titled, padded `Panel`. Keep its validation, status, and actions inside the
  same panel.
- Let the panel span the content column. Apply `max-w-*` to the fields or form
  inside it when a shorter line length improves usability, not to the panel
  shell itself.
- Use `FormSection` only to subdivide a single panel whose controls submit or
  operate as one group. It is not a substitute for the page-level panel frame.
- Stack independent settings panels with the standard `gap-6`, and never nest
  one `Panel` inside another.

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

Focused form fields use the action colour for their border without an additional
glow. Invalid fields follow the same treatment with the error-coloured border.

Compact filled controls pair each tone with its `on-*` foreground token.
Prominent action, success, warning, and danger buttons use dedicated fills with
contrast-safe labels. App Preferences → Appearance offers Blue, Cyan, Teal,
Green, Amber, Orange, Pink, Violet, and Grey. Cyan is the default. The saved
accent applies across this browser's registered servers and is restored before
first paint. It does not require a server request or account sync.

Each palette supplies a deeper shade and a brighter shade. `action` uses the
deeper shade in light mode and the brighter shade in dark mode for links,
focus borders, selection indicators, and compact status UI. Pair these filled
indicators with `on-action`. `button-action` uses the deeper shade in both
themes and pairs with `on-button-action` for white labels. Keep the lighting in
contrast checks. Warning, danger, presence, and server identity colours do not
change with the accent. Do not use the selected accent as the only way to
communicate status.

Filled buttons use the same quiet finish as the composer: a faint top highlight
and soft rim over the semantic fill, without a cast shadow. Pressed buttons
change fill. Ghost buttons stay flat. Disabled buttons have no lighting or raised edge.
`ToggleChip` uses the same `btn` foundation through `toggle-chip`, including
compact admin actions. It uses quiet `shell-lighting` without an outer shadow
so dense action rows stay subtle. Neutral chips use `input-border` to keep
a visible boundary on grey surfaces. Its labelled form uses the standard button radius;
its square form uses the compact icon radius.
Coloured fills retain a matched tonal border. Secondary buttons use a quiet
`surface-emphasized` fill and transparent border space. Filled buttons and Select fields
share the `control-raised` finish. Header icons stay flat. Do not add
local gloss, blur, transparency, or extra shadows.

Compact standalone composer actions and participant-card actions use
`CompactActionButton`. Their backgrounds are transparent at rest and show
the shared bevel on hover or keyboard focus. Disabled controls remain flat.

All message attachment actions pair `attachment-action-button` with `btn-secondary`
or `btn-danger-secondary` for deletion. The attachment utility sets the square
40 px geometry. One shared renderer arranges these controls in media overlays,
beside audio players, or beside ordinary filenames. Audio and file-card actions stay
visible in a horizontal row. Audio cards wrap actions below the player when space
is limited. Both use `attachment-card` for equal 12 px padding and a 12 px content
gap, with 40 px player and action controls. Action rows add no outer margin.
Image and video action groups appear on desktop hover or keyboard focus. The standard button
tone supplies the fill, border, focus, pressed state, and shared depth finish.
These controls follow the Flat, Kinda 3D, and Very 3D preference.
See `UI/Attachment actions` in Storybook.
`attachment-video-frame` keeps the video canvas at least 12 rem high so the
vertical action stack leaves space for playback controls on narrow screens.

### Shared Depth Utilities

`surface-raised` owns the gradient and lit-edge recipe. `surface-lowered` owns
inset shadows. Both live in `src/app.css` and set depth only: they do not set
colour, radius, layout, outer elevation, or interaction states. Use one depth
primitive per surface. Semantic utilities set the strength and add the rest of
the component treatment. Feature components should use those semantic utilities
instead of adding local gradients or arbitrary inset shadows.

| Utility | Use |
| --- | --- |
| `surface-raised` | Base raised finish. Semantic utilities set `--lighting-*` strength. |
| `surface-lowered` | Base recessed finish. Semantic utilities set `--lowered-shadow` and `--lowered-edge`. |
| `control-raised` | Buttons and Select fields; shared shell lighting with disabled-state handling and no cast shadow. Semantic utilities supply fill changes for press feedback. |
| `option-depth` | Quiet checkbox and radio rows; removes depth when disabled. |
| `control-well` | Empty checkbox and radio indicators. |
| `selection-indicator` | Soft lighting on selected checkbox and radio indicators, without a drop shadow. |
| `shell-surface` | User card and call participant cards; soft rim with no button elevation or pressed finish. |
| `shell-action` | Standalone native buttons in the bottom row, such as Start call. Uses the same surface and 48 px minimum height as the composer and user card, with hover, focus, pressed, and disabled states. |
| `chat-input-surface` | Composer and sidebar search fields; the same quiet raised `shell-surface` finish as the user card. |
| `shell-lighting` | Shared quiet finish for shell surfaces and raised controls. Also lights server gutter artwork without changing the image or intercepting clicks. |
| `floating-frame` | Lit panel, dialog, and menu frames. |
| `floating-inset` | Recessed content inside those frames. |
| `sheet-frame` | Touch dialog and menu frames; uses shared depth, shell radius, and safe-area spacing. |
| `app-frame-shell` / `app-frame-inset` | Desktop app frame with a flat fill and no depth highlight on its outer edge. Light mode raises the content area with a light upper/left edge, dark lower/right edge, and soft outer shadow. Dark mode keeps the recessed content edge and inset shadow. Higher contrast settings strengthen the edge in both themes. No full-window gradient. The inset overlay passes pointer input through to the panes. Mobile stays edge-to-edge. |
| `accent-swatch` | Palette samples with their own colour gradient and shared lit edges. |

The shared `--shell-*` theme tokens soften shell bevels in light mode with a
cleaner top highlight, less lower shading, and a small edge blur. Dark mode
keeps its sharper, low-light finish. Composer, user card, pill segments, buttons,
and Select fields all use the same recipe. Buttons
have no cast shadow or inset pressed effect. Secondary buttons keep transparent
border space; Select fields retain their field boundary. Ghost buttons stay flat.
Disabled and loading controls, including button-like links, have no decorative
lighting. See `Form/Button` → `Shared quiet depth` for a comparison in all three
depth modes.

Keep the `--lighting-*` and `--lowered-*` parameters inside semantic utilities.
Each depth primitive resets its parameters so a nested control does not inherit the
surrounding frame's strength. Colour, radius, and outer elevation remain the
responsibility of the semantic utility. Existing components apply their own
finish; callers must not stack a second finish on them.

Appearance exposes `data-depth="flat|3d|very-3d"` on the document root. The
registered `--depth-strength` and `--depth-width` numbers scale decorative
lighting, inset shadows, and bevel width. Kinda 3D uses 0.75 strength and 1 width. Flat uses
zero strength; Very 3D uses 1.75 strength and 1.5 width. Changes interpolate over
220 ms, with no transition under reduced motion. The preference is restored
before the first paint. Keep boundaries, focus rings, status colours, and
floating-menu elevation independent of this setting. Accent swatches become
solid in Flat, retain their gradient in 3D, and add sheen in Very 3D. New depth recipes must
use these shared parameters instead of fixed decorative shadow opacity.

`shimmer-hover` overlays one broad, soft highlight that crosses the surface in
450 ms. It runs once per hover and respects reduced motion. Nested gutter
logos defer to the outer shimmer so the effect does not stack.

Global header controls use `app-header-icon`. Hover highlights only the icon
or text, with no background, border, or lighting effect. Keep the 44 px hit
area and the visible keyboard focus outline. The colour changes with
`feedback-quick`. Text actions such as the version number use
`app-header-text-action` for the same treatment at content width.

Form inputs and `SegmentedControl` share the `control-frame` utility. It owns
their rounded corners, one-pixel border, and subtle inset shadow. Inputs and
segmented tracks read as recessed surfaces. `segmented-track` softens the track
fill and border, with space around every option. `segmented-selection` uses quiet
shared shell lighting to lift the selected pill above its track. Buttons do
not use `control-frame`.

Page searches use the standard bordered input treatment. Use `TextInput` or
the `bordered` appearance of `ChatSearchInput` when a clear action is needed.
The composer surface remains specific to chat rails.

Surfaces form a small semantic ladder:

### Surface Escalation Rule — Mandatory

> [!IMPORTANT]
> **NEVER INCREASE A NESTED ELEMENT BY MORE THAN ONE SURFACE LEVEL.** A child on
> `background` may use `surface`; a child on `surface` may use
> `surface-emphasized`; a child on `surface-emphasized` may use
> `surface-strong`. Never jump directly from `background` to
> `surface-emphasized` or from `surface` to `surface-strong`. If one level does
> not provide enough separation, add an appropriate border or revise the
> surrounding composition instead of skipping a level.

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
`DataTable` owns only its scrollable table viewport and keeps a radius when used
standalone or inside padded content. Inside `Panel noPadding`, the panel owns the
single outer radius and clipping boundary: the table viewport becomes square so
preceding controls or notices meet its header without an inset corner. Do not
add feature-local radius overrides for this composition. Dense matrices may keep
an intrinsic content width inside the viewport; ordinary record tables fill it.
Standard record-table headings use `table-header-cell`; matrix headings remain
bespoke because their vertical labels have different spatial needs.

When a matrix container is narrower than 640px, each row label occupies a
separate line above its cells. Labels wrap and stay visible during horizontal
scrolling, as do category labels. Column headings and cells share one native
scroll container so columns stay aligned. Wider matrices keep the sticky label
column. Cells retain explicit associations with their row and column headings
in both layouts.

`noPadding` does not require every child to be flush. `DataTable` uses a
continuous grid, so its header and rows meet the work-plane edge. A
`selectable-list` uses independent rounded rows, so it keeps the shared `p-1`
inset and `gap-1` inside that same work plane. Do not add local padding utilities to
either primitive to make it resemble the other.

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

The **Contrast** slider in Appearance's UI Style panel runs from 0% to 100%.
At 50%, the semantic palette keeps its original colours. Lower values soften
text and surface separation; higher values strengthen them. Keep text,
backgrounds, surfaces, and borders on semantic tokens so they respond together.
Accent, status, focus, and depth treatments stay independent of this control.
At 0%, headings, body text, and muted text become deliberately softer in both
themes. Action colours keep their separate contrast.
At 100%, light uses black text on a white background with dark boundaries.
Dark uses white text on a black background with light boundaries. The app frame
and recessed panel edges use the same clear boundary. The prominent
range field gains a visible boundary as contrast increases.
The control fills the UI Style panel width and uses the prominent `RangeField`
variant, with a larger track, thumb, and pointer target. Other range settings
keep the standard size.

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
- Treat each settings panel as one action scope. Its recommended committing
  action uses `action`, whether it submits a form or starts an immediate upload.
  A page can contain one recommended action in each independent panel.
- Use sentence case for action labels, such as "Save changes" and "Upload logo".
- Use `danger-secondary` for a quiet destructive action beside a recommended
  action. Do not apply a local danger text colour to `secondary` or `ghost`.

`Button` is not the universal representation of every clickable control.
Menus, compact chat hover bars, media overlays, and icon toolbars use their
context-specific primitive.

Context-menu commands use `MenuItem` inside sibling `MenuSection` components.
`ContextMenu` supplies the compact or touch-safe density. `MenuItem` supplies
button and link semantics, optional leading and trailing content, disabled and
selected states, and danger tone. It also aligns an icon with the first line of
a wrapped label. Use `sidebar-item` only for sidebar navigation.

Let the `ContextMenu` gap separate sibling sections. Do not draw a hairline
divider inside a section because it conflicts with the standard surface-gap
separator.

The supported variants are `action`, `neutral`, `secondary`, `ghost`,
`warning`, `danger`, and `danger-secondary`. Use the variant whose meaning
matches the action.

## Shape, Type, And Motion

Hover, focus, and action-reveal feedback use these tokens from `src/app.css`:

| Token | Default | Purpose |
| --- | --- | --- |
| `--motion-duration-feedback` | `0ms` | Instant item highlights and action reveals in both directions |
| `--motion-easing-feedback` | `ease-out` | Timing curve in both directions |
| `--motion-duration-overlay-enter` | `100ms` | Modal and floating context-menu entrance |
| `--motion-easing-overlay-enter` | `ease-out` | Surface entrance curve |

Use `feedback-quick` with explicit transition properties, for example
`transition-opacity feedback-quick`. The utility reads both tokens and disables
transitions under reduced motion. Menu highlights, member-card menu reveals,
row actions, and icon feedback share this timing. Do not add local numeric
durations for these interactions. Keep disabled/pending opacity at 150 ms;
pane movement and toolbar entrance animations have separate timing.

Centred dialogs and floating context menus fade in and zoom from 95% to full size.
Floating context menus close immediately; centred dialogs use a 100 ms exit.
Sheets slide in and out with `--motion-duration-pane`.
Reduced motion skips surface animations. Floating placement uses the full
layout size so the entrance zoom cannot move a menu beyond the viewport.
Touch context menus keep their bottom-sheet presentation.

- Labelled buttons use `rounded-xl` at every size, matching chat input surfaces.
  Filled icon-only buttons keep `rounded-md` corners.
- `rounded` and `rounded-md` are the default for other compact controls, fields,
  nested rows, pills, and embedded content.
- Quiet close, clear, and header icon controls use `rounded-lg` with a faint
  neutral hover tint and a stronger pressed tint. Keep their keyboard-focus
  outline. Selected toolbar controls retain their persistent fill; disabled
  controls do not gain a hover fill.
- `rounded-lg` also applies to menus, dialogs, panels, and major shells.
- Outside labelled buttons and chat input surfaces, `rounded-xl` is reserved
  for softer product-specific objects, such as server tiles.
- Nested rounded surfaces should be concentric when their padding is small.
- Base text is the default. Use `text-sm` for secondary copy and `text-xs` for
  metadata, timestamps, and terse labels. When any touch pointer is available
  (`any-pointer: coarse`), these sizes are 17, 15, and 13 px at the browser
  default, including on hybrid devices. Mouse-only devices keep 16, 14, and
  12 px at every viewport width. These text tokens do not change spacing or
  heading sizes.
- A compact surface uses one text size throughout. Menus, popovers, controls,
  and nested rows must not mix smaller metadata text with base-sized actions;
  express hierarchy with color, weight, spacing, and icons instead.
- Headings use balanced wrapping; short body copy uses pretty wrapping.
- Updating numeric columns and counters use `tabular-nums`.
- Interactive transitions must name their properties. Never use Tailwind's
  bare `transition` utility or `transition-all`.
- Press feedback uses `active:scale-[0.96]` where it does not interfere with
  drag, resize, or text-selection behavior.
- Shared buttons fade disabled/pending opacity over 150 ms in both directions.
  Pill buttons use instant hover colour feedback. Reduced motion skips opacity fades.
- Respect `prefers-reduced-motion` for non-essential animation.
- Wrap conditional compact toolbars in `FadeScale` for a shared 180 ms
  fade and 96–100% zoom with exponential ease-out entry and cubic ease-in-out
  exit, anchored at the bottom start
  corner. Reduced motion skips the transition. The composer formatting bar
  and current-user call toolbar use this component.
- Use `WipeReveal` for the start-call button and active call controls. Pass the
  connected state as `active` for a 320 ms left-to-right wipe with
  a feathered diagonal edge and cubic ease-in-out. Mounting or hiding the
  sidebar does not animate the controls. Reduced motion skips the wipe.
- Keep interactive hit areas at least 40 by 40 pixels unless a dense desktop
  toolbar has a documented non-overlapping exception. `mini-icon-action` is
  the narrow exception for a subordinate icon placed directly beside the text
  or value it acts on; do not use it for standalone or toolbar actions.

## Input And Responsive Layout

Input capability selects control behavior. Available space selects content layout.
Use `touch-input` for targets and actions that must work whenever
`any-pointer: coarse` matches, including hybrid devices. `compact-input` applies
only when no coarse pointer is available. `hover-actions` permits mouse hover
actions on hybrid devices, but essential controls must also remain accessible
through touch and keyboard input.

Use `mobile-presentation` for the app frame and task sheets. It requires both a
viewport below 768 px and a coarse pointer. `desktop-presentation` is its inverse:
narrow mouse windows and all wide windows retain the padded frame, rounded inset,
and compact header. The header and drawer offsets share `--app-header-height`.
Safe-area spacing stays separate from that height.
In narrow mouse windows, drawers and their backdrop stay inside the rounded
work plane, including during animation. Measure that plane for swipe distances.
Narrow touch windows keep viewport-based drawers and device safe-area offsets.

Use Svelte's `MediaQuery` with `NARROW_TOUCH_QUERY` from `inputMediaQueries.ts`
when a component needs a reactive presentation decision. Keep this query and
the CSS variants in sync. Do not select a layout from a device name or the last
pointer event. Touch-safe menus can remain floating when hover is available;
touch input still gets larger command targets and emoji targets at every width.
Keep the existing primary-pointer checks for keyboard and autofocus defaults.
Mouse-primary hybrid devices retain mouse keyboard behavior; use touch capability,
not the primary-pointer preference, to select target sizes and frame presentation.

Keep width or container queries for pane columns, navigation collapse, resize
handles for docked panes, toolbar overflow, compact labels, header wrapping,
autocomplete width, media viewers, and form/card grids. These rules prevent
content from exceeding its available space. Timeline row spacing and rounded
highlights follow presentation instead of width alone.

Chatto deliberately uses browser/platform text rendering. Do not add global
font smoothing. Keep the browser's default root text size on mobile as well as
desktop. In narrow touch-capable windows, room and thread timelines add `px-1`
padding around their rows to give avatars and message content more space at
the screen edges.

On iOS, the Home Screen app uses the `default` status-bar style so the system
places web content below the status bar. Do not use `black-translucent`, which
allows the status bar to cover web content. Keep the root layout's safe-area
padding for browser and device layouts that expose these insets.
The `html` and `body` backgrounds use the frame's `surface` colour so system
bars that sample the page match the app frame. The early theme script and
display preferences keep the root background and `theme-color` hint in sync.
The app header is sticky with an opaque `surface` background so WebKit can
extend its colour into the top system bar. A page background alone does not
meet WebKit's fixed-or-sticky header condition for this colour extension.
On mobile, a fixed one-pixel `surface` at the bottom edge provides the same
colour source for Safari's bottom toolbar. It does not receive pointer input
or take layout space, so it cannot block controls or reduce the composer area.

Set `collapseActions` on `PaneHeader` to collapse its `actions` snippet behind
a three-dot button when the pane is narrower than 32 rem. Use `actionsLabel`
for a context-specific accessible label. The button expands the actions inside the pane
header; the room title truncates as needed. A second press or Escape collapses
the actions. Wider panes keep the actions beside the title.
Use the optional `collapsedActions` snippet for important actions that must
remain visible while collapsed. Room headers use this option for active calls.
When a call is active, the collapsed header keeps the call button visible
beside the three-dot button. It uses the same active-call colour and pulse as
the expanded toolbar.

The app header and room header hide in narrow touch-capable windows while the shared
viewport detector reports an open software keyboard. They return when it
closes. `PaneHeader` opts in with `hideOnKeyboard`; thread and settings headers
stay visible. The `keyboard-hide-mobile` utility removes the complete header
from the layout without an animation. Input focus alone does not hide it.

Controls use solid semantic fills, with quiet shell lighting on filled buttons;
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

Literal story fixture copy is exempt from application translation catalogs.
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

## Initial Page Reveal

The app layout reveals the first page with a short, staggered fade and scale.
It runs once per full page load. Route changes and later content updates do not
start it again. Reduced motion skips the reveal; keyboard or pointer input stops it.

Shared pane and authentication components mark sections with `data-page-reveal`.
Use this attribute to split a custom page into smaller reveal sections. The
layout animates separate sections, never a parent and its children together.
Keep content visible in its base styles. Do not add a second entrance animation
to a page.

Before the initial route is ready, the HTML shell shows soft, blurry neutral
blobs that drift across the viewport.
Its critical styles are inline so it does not wait for application assets. It
uses the background and emphasized-surface theme tokens, with matching light
and dark fallbacks before CSS loads, and stays static with reduced motion. A
faint, centred CHATTO wordmark uses a fixed system font to prevent size changes
when application fonts load. When the app layout mounts, the shell fades and
scales out while the page sections appear below it. The app removes the shell
when this transition ends. Keyboard or pointer input ends all startup
animations and shows the complete page immediately. Automatic form focus does
not stop the reveal. Reduced motion removes the shell without a transition.

Server banners fill the sidebar width with square corners and a bottom
`border-border` separator. They form part of the app grid and use no bevel,
gloss overlay, or outer shadow. Display the complete image at its original aspect ratio, with no height cap.
Server operators control the banner shape through the uploaded image.
