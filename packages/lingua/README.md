# `@chatto/lingua`

`@chatto/lingua` is a small, framework-neutral internationalisation runtime for
JSON catalogs. It loads catalogs by locale and section, performs synchronous
lookups after loading, supports locale fallback and CLDR plurals, and does not
compile or generate translation modules.

The package is licensed under Apache-2.0.

## Catalogs

Each JSON file contains one top-level section:

```json
{
  "room": {
    "title": "Room",
    "member_count": {
      "one": "{count} member",
      "other": "{count} members"
    },
    "rules_html": "Please read the <strong>room rules</strong>."
  }
}
```

Keys ending in `_count` or `.count` are plural messages. Their values use CLDR
plural categories and must define `other`. Keys ending in `_html` or `.html`
contain markup and can only be read through `html()`.

Plural branches may use `{count}`, but do not have to display it. This supports
grammar-only choices such as “joined the room” versus “joined the room” while
still selecting the correct form for languages that distinguish them.

## Usage

Provide ordinary dynamic imports. Lingua does not depend on Vite; any loader
that returns a JSON-like document works.

```ts
import { createLingua } from "@chatto/lingua";

const lingua = createLingua({
  baseLocale: "en-GB",
  initialLocale: "de-DE",
  fallbackLocales: {
    "de-AT": "de-DE",
  },
  initialBaseCatalogs: {
    room: { room: { title: "Room" } },
  },
  loaders: {
    room: {
      "en-GB": () => import("./messages/en-GB/room.json"),
      "de-DE": () => import("./messages/de-DE/room.json"),
      "de-AT": () => import("./messages/de-AT/room.json"),
    },
  },
});

await lingua.setActiveSections(["room"]);

lingua.t("room.title");
lingua.t("room.member_count", { count: 12 });
```

Static translation keys are intentionally not generated or catalog-typed.
TypeScript does infer locale and section names from the loader registry. It
also requires `{ count: number }` when a literal key follows the plural naming
convention.

`initialBaseCatalogs` is optional. Applications can use it to make their base
fallback synchronously available before the first asynchronous section load.

`setLocale()` loads every active section before publishing the new locale, so
subscribers never observe a partially loaded language. Loaded sections and
in-flight requests are cached. `fallbackLocales` can define a chain of
regional overlays, such as `de-AT` falling back to `de-DE`; the base locale is
always the final fallback. When a translated section loads, Lingua also rejects
unknown keys, changed value kinds, and placeholders that differ from the base
catalog.

## Localized HTML

`html()` returns `LocalizedHtml`, which represents **untrusted** localized
markup:

```ts
const markup = lingua.html("room.rules_html");
```

Lingua HTML-escapes interpolated values, but it deliberately does not sanitize
the translated template. The rendering application must sanitize the complete
result against a strict element, attribute, and URL allowlist before inserting
it as HTML. Ordinary `t()` calls reject HTML-designated keys.

## Reactive integrations

`subscribe()` immediately receives the current locale, active sections, and
revision, then receives another immutable snapshot after every committed
transition. Framework adapters can use this to invalidate their own reactive
state without adding a framework dependency to Lingua.
