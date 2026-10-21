# `@chatto/translatto`

`@chatto/translatto` is a small, framework-neutral internationalisation runtime for
JSON catalogs. It loads catalogs by locale and section, performs synchronous
lookups after loading, supports locale fallback and CLDR plurals, and does not
compile or generate translation modules.

The package is licensed under Apache-2.0.

## Rename migration

Replace the `@chatto/lingua` dependency and imports with `@chatto/translatto`.
Replace `createLingua`, `Lingua`, `LinguaError`, `LinguaOptions`, and
`LinguaSnapshot` with `createTranslatto`, `Translatto`, `TranslattoError`,
`TranslattoOptions`, and `TranslattoSnapshot`, respectively.

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
Plural HTML messages can use `_count_html`, `.count_html`, or `.count.html`.

Plural branches may use `{count}`, but do not have to display it. This supports
grammar-only choices such as “joined the room” versus “joined the room” while
still selecting the correct form for languages that distinguish them.

## Usage

Provide ordinary dynamic imports. Translatto does not depend on Vite; any loader
that returns a JSON-like document works.

```ts
import { createTranslatto } from '@chatto/translatto';

const translatto = createTranslatto({
  baseLocale: 'en-GB',
  initialLocale: 'de-DE',
  fallbackLocales: {
    'de-AT': 'de-DE'
  },
  initialBaseCatalogs: {
    room: { room: { title: 'Room' } }
  },
  loaders: {
    room: {
      'en-GB': () => import('./messages/en-GB/room.json'),
      'de-DE': () => import('./messages/de-DE/room.json'),
      'de-AT': () => import('./messages/de-AT/room.json')
    }
  }
});

await translatto.setActiveSections(['room']);

translatto.t('room.title');
translatto.t('room.member_count', { count: 12 });
```

Static translation keys are intentionally not generated or catalog-typed.
TypeScript does infer locale and section names from the loader registry. It
also requires `{ count: number }` when a literal key follows the plural naming
convention.

`initialBaseCatalogs` is optional. Applications can use it to make their base
fallback synchronously available before the first asynchronous section load.
Include only sections needed at startup. Other base sections can use lazy
loaders, just like translated sections.

`setLocale()` loads every active section before publishing the new locale, so
subscribers never observe a partially loaded language. Loaded sections and
in-flight requests are cached. `fallbackLocales` can define a chain of
regional overlays, such as `de-AT` falling back to `de-DE`; the base locale is
always the final fallback. When a translated section loads, Translatto also rejects
unknown keys, changed value kinds, and placeholders that differ from the base
catalog.

`preload()` only fills the cache; it does not change lookup results or notify
subscribers. A successful `setActiveSections()` or `setLocale()` makes all
required catalogs available together. A failed load or validation leaves the
published translations unchanged. A later request retries the failed catalog.

Active sections determine what a locale change must load. Previously published
sections remain readable after deactivation, and their downloads remain cached.
Concurrent locale and section setters combine their requested values. The latest
request publishes the combined state; a superseded request can finish without
publishing. Failed requests reject their promises. If the latest request fails,
the next setter starts from the last committed locale and active sections.

## Localized HTML

`html()` returns `LocalizedHtml`, which represents **untrusted** localized
markup:

```ts
const markup = translatto.html('room.rules_html');
```

Translatto HTML-escapes interpolated values, but it deliberately does not sanitize
the translated template. The rendering application must sanitize the complete
result against a strict element, attribute, and URL allowlist before inserting
it as HTML. Ordinary `t()` calls reject HTML-designated keys.

## Reactive integrations

`subscribe()` immediately receives the current locale, active sections, and
revision, then receives another immutable snapshot after every committed
transition. Framework adapters can use this to invalidate their own reactive
state without adding a framework dependency to Translatto.
