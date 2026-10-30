# Authling design system

Authling uses templ, Tailwind CSS, and native HTML. Fonts and icons are served
from Authling's own origin. Locally bundled htmx enhances ordinary navigation;
templ still renders each page. This system applies to authentication, consent,
account, and admin pages.

## Foundations

Use IBM Plex Sans for interface text. Use monospace only for identifiers and
verification codes. All interface text uses 17px, including headings, inputs,
buttons, hints, badges, and metadata. Use weight, spacing, and secondary text
color for hierarchy instead of different font sizes. Normal line height is 24px.
Input text at 17px also avoids focus zoom on mobile browsers.

Use a 4px spacing grid. Forms have 16px between fields and 24px before the first
field. Related text has 4–8px between lines or groups. Buttons and inputs are at
least 44px high, including compact buttons. Control corners are 8px; record and
table corners are 12px. Cards and panels use `--ui-raised`, `--ui-raised-border`,
and `--ui-raised-shadow` for separation from the canvas. Inputs and nested records
do not have elevation shadows. The page canvas
has a subtle radial gradient from `--ui-canvas-glow` at the top center to
`--ui-canvas`.

Shared colors live in `web/src/app.css`. Light and dark values are defined once;
components use semantic Tailwind names. Do not add per-page neutral shades.
Dark mode uses blue-gray surfaces and soft gray text. Primary actions use a
muted blue-gray fill instead of a bright white block.

| Token | Use |
| --- | --- |
| `canvas` | Page background |
| `surface` | Input and record background |
| `subtle` | Hover areas, badges, and table headers |
| `ink`, `muted` | Primary and secondary text |
| `line`, `control-line` | Content separators and control boundaries |
| `action`, `action-hover`, `on-action` | Primary actions and their text |
| `focus` | Keyboard outlines and input rings |
| `danger`, `success`, `warning` | Status text, paired with matching `*-surface` |

Color is supplementary. Notices explain results in words. Errors and warnings
use alert semantics; success and neutral feedback use status semantics. Disabled
controls remain native disabled controls. Do not remove keyboard focus outlines.

## Layouts and components

Sign-in and registration use a `focus-card` with a 32rem maximum width.
`focus-main` centers the card
horizontally and vertically in the space below the site header. On short screens,
auto margins collapse and the document scrolls; the card's top stays accessible.

Account, recovery, and administration pages use a 64rem content container.
The site header aligns with this container. Administrator navigation appears
in the header on the account overview and administration pages.
The account workspace has four URL-backed sections: Profile (`/account`),
Security (`/account/security`), Connected apps (`/account/apps`), and Sessions
(`/account/sessions`). Use ordinary links with `aria-current="page"` for the
active section. The four equal-width tabs form the top edge of the content
sheet. They remain in one row on small screens, where Connected apps is
displayed as Apps. The accessible name remains Connected apps.
Account tasks retain this navigation and use full-width panels. Fields and
explanatory text have a narrower maximum width for readability.
Each functional section
uses a `panel` that fills the container width. The user directory keeps its
heading, table, and pagination in one panel. Panels use the same surface and
border tokens as focused cards. Standalone card and panel titles use a full-width
title bar with a bottom divider. In the account sheet, the tab strip supplies
the shared frame and section titles sit inside the content. Descriptions remain
in the padded content body.
Sessions and connected apps remain records inside
their panels. Consent keeps its narrow, compact layout for authorization popups.

Use `ui.Heading` once per page and `ui.SectionHeading` for account task panels.
Use `settings-heading` and `settings-description`
for sections, and `form-stack` for normal forms. Consent may use a compact action
row. Allow scrolling for long content, expanded disclosures, or increased text
size; never hide overflow to make a page fit.

The [component guide](../internal/web/ui/README.md) describes the templ API.
Page templates live beside their feature handlers in `internal/web/`.
`layout.templ` owns the document and page shell. Keep feature content in its
own template and shared markup in `ui/`.
Use components when markup carries shared semantics: labels and hints, notices,
or table captions. Use CSS primitives for native buttons, links, and disclosures.
A wrapper around every HTML element is not required.

Buttons use `button` plus `button-primary`, `button-secondary`, or `button-danger`.
Add `button-compact` for record actions and `w-full` where the layout needs it.
Use links for navigation and buttons for actions. Set button types explicitly.
Use `text-link` for inline navigation.

Use `ui.Field` for text, email, and password inputs. Supply native validation,
autocomplete, and required attributes at the call site. Never return passwords
as input values. Put hints in the hint argument to connect `aria-describedby`.
Use `aria-invalid` only when the server knows which field is invalid; login
failures must not disclose whether an address exists. `ui.CodeField` keeps one
native input behind six decorative cells.

Use native `details` with `disclosure` for optional information, or `table-details`
for small table identifiers. Tables retain native row and column headers; only
the table region scrolls horizontally on narrow screens.

## Preview and verification

The HTML head includes a small light/dark canvas style before external assets.
Its exact CSP hash permits this style without permitting arbitrary inline CSS.
Keep its fallback colors in `internal/web/assets.go` aligned with the canvas tokens.

Ordinary links use htmx to replace the body while keeping the document alive.
Ordinary sign-in and profile editing also use htmx, including validation errors
and account redirects. Their submit buttons are disabled while a request is
pending. A failed network request preserves input and displays a message; it
does not automatically repeat the submission.
OAuth sign-in, other forms, consent pages, and authentication links with
query parameters keep native navigation. Titles, browser history, and keyboard
focus follow the destination.
History pages are fetched again instead of stored in htmx's browser cache.
Without JavaScript, all links and forms use ordinary browser navigation.
Page changes have no animation. htmx swaps content directly; CSS View
Transitions are not used.
The initial canvas style remains active without the external stylesheet.

Start the development Authling server and open `/dev/ui`. The preview is
registered only in binaries built with the `bootstrap` tag. Static examples read
no account data and have no save actions. Production binaries return 404 for this
path. The page follows the browser's light/dark preference.

Inspect normal, invalid, disabled, read-only, and pending states. Check keyboard
focus, empty tables, long content, first/last-page navigation, and narrow layouts.
Then check real pages. Test forms without
JavaScript and retain native validation and autofill. Check long names and email
addresses. Standard consent must fit at 520 × 500px; forms must remain usable at
320px width. Arbitrary long content and zoomed text may need scrolling.

Run `mise codegen-templates`, `mise build-web`, and `mise test-e2e` from
`authling/`. Check both build modes with `mise test` when the preview route changes.
