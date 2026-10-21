# Translatto

Translatto is a small internationalisation library for JavaScript and TypeScript.
It loads JSON catalogs by locale and section, then provides synchronous
translation lookups. It supports locale fallback, interpolation, and plural
messages without a framework dependency or generated translation code.

## Installation

```sh
npm install @chatto/translatto
```

The package provides ESM JavaScript and TypeScript declarations. It has no
runtime dependencies. Use a runtime with ES2022, `Intl.PluralRules`, and
`Intl.NumberFormat` support.

## Quick start

Group your loaders by section, then by locale. Each loader returns a promise
with a catalog that contains that section.

```ts
import { createTranslatto } from '@chatto/translatto';

const translatto = createTranslatto({
  baseLocale: 'en-GB',
  loaders: {
    room: {
      'en-GB': async () => ({
        room: {
          welcome: 'Hello {name}',
          member_count: { one: '{count} member', other: '{count} members' }
        }
      }),
      'de-DE': async () => ({
        room: {
          welcome: 'Hallo {name}',
          member_count: { one: '{count} Mitglied', other: '{count} Mitglieder' }
        }
      })
    }
  }
});

await translatto.setActiveSections(['room']);

translatto.t('room.welcome', { name: 'Ada' }); // 'Hello Ada'
translatto.t('room.member_count', { count: 2 }); // '2 members'

await translatto.setLocale('de-DE');
translatto.t('room.welcome', { name: 'Ada' }); // 'Hallo Ada'
```

The initial locale defaults to `baseLocale`. Set `initialLocale` to start with
another locale from the loader registry. Activate the sections you need before
you read their translations.

## JSON catalogs

Catalogs can be JSON files. For example, `messages/en-GB/room.json`:

```json
{
  "room": {
    "welcome": "Hello {name}",
    "member_count": {
      "one": "{count} member",
      "other": "{count} members"
    },
    "rules_html": "Please read the <strong>room rules</strong>."
  }
}
```

In a bundler that supports JSON imports, use a dynamic import as the loader:

```ts
const loaders = {
  room: {
    'en-GB': () => import('./messages/en-GB/room.json'),
    'de-DE': () => import('./messages/de-DE/room.json')
  }
};
```

Translatto accepts a catalog object or a module with that object as its default
export. Loaders can use another data source; Translatto does not require Vite
or make network requests itself. Each section must have a base-locale loader.

## Interpolation and plurals

Use `{name}` placeholders in messages and pass their values to `t()`. Values
can be strings or numbers.

Keys ending in `_count` or `.count` identify plural messages. Pass a numeric
`count` to select a CLDR plural category for the locale. Each plural message
must define `other`. All branches must use the same placeholders. They can
omit `{count}` when the number selects a form but is not part of the text.

TypeScript infers locale and section names from the loader registry. It also
requires a numeric `count` for literal keys that follow the plural naming
convention. Translation keys are not generated or checked against the catalog
by TypeScript.

## Fallback and validation

The base locale is the final fallback for missing translations. Use
`fallbackLocales` in the options to add intermediate locales, for example
`{ 'de-AT': 'de-DE' }`. Both locales must be in the loader registry.

Translated catalogs can contain only the messages they override. Translatto
rejects unknown keys, changed value types, and placeholders that differ from
the base catalog. If a key is missing from all fallbacks, `t()` returns
`⟦key⟧` by default. Set `onMissingTranslation` to customize that result.

## Loading and locale changes

`setLocale()` loads all active sections before it commits the new locale.
`setActiveSections()` also waits for all required catalogs. Subscribers see
one complete update. If loading or validation fails, the promise rejects and
the previous translations remain available. A later request retries a failed
load. Successful loads and requests in progress are cached.

`preload(locale, sections)` fills the cache without changing the current
translations or notifying subscribers. Sections that were loaded successfully
remain readable after deactivation.

Concurrent locale and section changes combine their requested values. Only
the latest request commits the combined state. If it fails, the next change
starts from the last committed locale and active sections.

For synchronous access to base translations at startup, supply
`initialBaseCatalogs` in the options. Each entry must contain the **complete
base catalog for that section**; it replaces that section's base-locale
loader result. Other sections can load on demand.

## Localized HTML

Keys ending in `_html` or `.html` contain markup and must use `html()`.
Plural HTML keys can end in `_count_html`, `.count_html`, or `.count.html`.

For a loaded catalog with the `room.rules_html` message shown above:

```ts
const markup = translatto.html('room.rules_html');
```

`html()` returns a `LocalizedHtml` string that contains **untrusted markup**.
Translatto HTML-escapes interpolated values, but does not sanitize the
translated template. Sanitize the complete result with a strict allowlist
of elements, attributes, and URLs before you insert it as HTML. `t()` rejects
HTML keys.

## Reactive integrations

`subscribe()` immediately calls the listener with a snapshot of the locale,
active sections, and revision. It calls the listener again after each committed
change. Snapshots are immutable. Use them to update your framework's state.
The returned function removes the listener.

```ts
const unsubscribe = translatto.subscribe((snapshot) => {
  console.log(snapshot.locale);
});

// When the integration is no longer needed:
unsubscribe();
```

## Contributing

See the [maintainer guide](https://github.com/chattocorp/chatto/blob/main/packages/translatto/MAINTAINING.md)
for development commands and release instructions.

## License

Apache-2.0.
