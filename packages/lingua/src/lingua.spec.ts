import { describe, expect, it, vi } from "vitest";

import {
  CatalogValidationError,
  InterpolationError,
  LinguaError,
  TranslationKindError,
  createLingua,
} from "./index.js";

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, reject, resolve };
}

function common(values: Record<string, unknown>) {
  return { default: { common: values } };
}

describe("Lingua", () => {
  it("loads active sections, caches them, and falls back by message", async () => {
    const loadBase = vi.fn(async () =>
      common({
        close: "Close",
        welcome: "Welcome, {name}!",
        base_only: "Base only",
      }),
    );
    const loadGerman = vi.fn(async () => common({ close: "Schließen" }));
    const lingua = createLingua({
      baseLocale: "en-GB",
      initialLocale: "de-DE",
      loaders: { common: { "en-GB": loadBase, "de-DE": loadGerman } },
    });

    await lingua.setActiveSections(["common"]);

    expect(lingua.t("common.close")).toBe("Schließen");
    expect(lingua.t("common.welcome", { name: "Ada" })).toBe("Welcome, Ada!");
    expect(lingua.t("common.base_only")).toBe("Base only");
    expect(loadBase).toHaveBeenCalledOnce();
    expect(loadGerman).toHaveBeenCalledOnce();

    await lingua.preload("de-DE", ["common"]);
    expect(loadBase).toHaveBeenCalledOnce();
    expect(loadGerman).toHaveBeenCalledOnce();
  });

  it("layers regional overlays through their configured fallback chain", async () => {
    const lingua = createLingua({
      baseLocale: "en-GB",
      initialLocale: "de-AT",
      fallbackLocales: { "de-AT": "de-DE" },
      loaders: {
        common: {
          "en-GB": async () =>
            common({
              close: "Close",
              parent_only: "Base parent",
              base_only: "Base only",
            }),
          "de-DE": async () => common({ parent_only: "Nur im Elternkatalog" }),
          "de-AT": async () => common({ close: "Schließen" }),
        },
      },
    });

    await lingua.setActiveSections(["common"]);

    expect(lingua.t("common.close")).toBe("Schließen");
    expect(lingua.t("common.parent_only")).toBe("Nur im Elternkatalog");
    expect(lingua.t("common.base_only")).toBe("Base only");
  });

  it("validates translated keys and placeholders against the base locale", async () => {
    const unknownKey = createLingua({
      baseLocale: "en-GB",
      initialLocale: "de-DE",
      loaders: {
        common: {
          "en-GB": async () => common({ welcome: "Welcome, {name}!" }),
          "de-DE": async () => common({ typo: "Unbekannt" }),
        },
      },
    });
    await expect(unknownKey.setActiveSections(["common"])).rejects.toThrow(
      "does not exist in the base locale",
    );

    const changedPlaceholder = createLingua({
      baseLocale: "en-GB",
      initialLocale: "de-DE",
      loaders: {
        common: {
          "en-GB": async () => common({ welcome: "Welcome, {name}!" }),
          "de-DE": async () => common({ welcome: "Willkommen, {username}!" }),
        },
      },
    });
    await expect(
      changedPlaceholder.setActiveSections(["common"]),
    ).rejects.toThrow("must preserve the base locale placeholders");
  });

  it("uses locale plural rules and locale-aware count formatting", async () => {
    const lingua = createLingua({
      baseLocale: "en-GB",
      initialLocale: "pl",
      loaders: {
        common: {
          "en-GB": async () =>
            common({
              member_count: { one: "{count} member", other: "{count} members" },
            }),
          pl: async () =>
            common({
              member_count: {
                one: "{count} członek",
                few: "{count} członków",
                many: "{count} członków",
                other: "{count} członka",
              },
            }),
        },
      },
    });
    await lingua.setActiveSections(["common"]);

    expect(lingua.t("common.member_count", { count: 1 })).toBe("1 członek");
    expect(lingua.t("common.member_count", { count: 2 })).toBe("2 członków");
    expect(lingua.t("common.member_count", { count: 5 })).toBe("5 członków");
    expect(lingua.t("common.member_count", { count: 1.5 })).toBe("1,5 członka");
  });

  it("keeps locale changes atomic and ignores a stale transition", async () => {
    const german = deferred<ReturnType<typeof common>>();
    const lingua = createLingua({
      baseLocale: "en-GB",
      loaders: {
        common: {
          "en-GB": async () => common({ close: "Close" }),
          "de-DE": () => german.promise,
          fr: async () => common({ close: "Fermer" }),
        },
      },
    });
    await lingua.setActiveSections(["common"]);

    const germanTransition = lingua.setLocale("de-DE");
    const frenchTransition = lingua.setLocale("fr");
    await frenchTransition;

    expect(lingua.snapshot.locale).toBe("fr");
    expect(lingua.t("common.close")).toBe("Fermer");

    german.resolve(common({ close: "Schließen" }));
    await germanTransition;
    expect(lingua.snapshot.locale).toBe("fr");
    expect(lingua.t("common.close")).toBe("Fermer");
  });

  it("does not publish a section transition that fails to load", async () => {
    const lingua = createLingua({
      baseLocale: "en-GB",
      loaders: {
        common: { "en-GB": async () => common({ close: "Close" }) },
        room: {
          "en-GB": async () => {
            throw new Error("network failure");
          },
        },
      },
    });
    await lingua.setActiveSections(["common"]);
    const before = lingua.snapshot;

    await expect(lingua.setActiveSections(["common", "room"])).rejects.toThrow(
      "network failure",
    );
    expect(lingua.snapshot).toEqual(before);
    expect(lingua.t("room.title")).toBe("⟦room.title⟧");
  });

  it("deduplicates concurrent loader requests and retries failures", async () => {
    const first = deferred<ReturnType<typeof common>>();
    const loader = vi
      .fn<() => Promise<ReturnType<typeof common>>>()
      .mockImplementationOnce(() => first.promise)
      .mockResolvedValue(common({ close: "Close" }));
    const lingua = createLingua({
      baseLocale: "en-GB",
      loaders: { common: { "en-GB": loader } },
    });

    const firstPreload = lingua.preload("en-GB", ["common"]);
    const concurrentPreload = lingua.preload("en-GB", ["common"]);
    first.reject(new Error("temporary failure"));
    await expect(firstPreload).rejects.toThrow("temporary failure");
    await expect(concurrentPreload).rejects.toThrow("temporary failure");

    await lingua.preload("en-GB", ["common"]);
    expect(loader).toHaveBeenCalledTimes(2);
  });

  it("notifies subscribers only after committed transitions", async () => {
    const lingua = createLingua({
      baseLocale: "en-GB",
      loaders: { common: { "en-GB": async () => common({ close: "Close" }) } },
    });
    const listener = vi.fn();
    const unsubscribe = lingua.subscribe(listener);

    await lingua.preload("en-GB", ["common"]);
    expect(listener).toHaveBeenCalledOnce();

    await lingua.setActiveSections(["common"]);
    expect(listener).toHaveBeenCalledTimes(2);
    expect(listener.mock.lastCall?.[0]).toEqual({
      locale: "en-GB",
      activeSections: ["common"],
      revision: 1,
    });

    unsubscribe();
    await lingua.setActiveSections([]);
    expect(listener).toHaveBeenCalledTimes(2);
  });

  it("keeps HTML on its explicit path and escapes interpolated values", async () => {
    const lingua = createLingua({
      baseLocale: "en-GB",
      loaders: {
        common: {
          "en-GB": async () =>
            common({
              warning_html: "<strong>{message}</strong>",
              legal: { html: "<em>{message}</em>" },
              member_count_html: {
                one: "<strong>{count}</strong> member",
                other: "<strong>{count}</strong> members",
              },
            }),
        },
      },
    });
    await lingua.setActiveSections(["common"]);

    expect(lingua.html("common.warning_html", { message: `&<>"'` })).toBe(
      "<strong>&amp;&lt;&gt;&quot;&#39;</strong>",
    );
    expect(lingua.html("common.legal.html", { message: "Safe" })).toBe(
      "<em>Safe</em>",
    );
    expect(lingua.html("common.member_count_html", { count: 2 })).toBe(
      "<strong>2</strong> members",
    );
    expect(() =>
      (lingua.t as (key: string) => string)("common.warning_html"),
    ).toThrow(TranslationKindError);
    expect(() =>
      (lingua.html as (key: string) => string)("common.legal"),
    ).toThrow(TranslationKindError);
  });

  it("reports missing translations and interpolation values clearly", async () => {
    const onMissingTranslation = vi.fn(
      ({ key }: { key: string }) => `missing:${key}`,
    );
    const lingua = createLingua({
      baseLocale: "en-GB",
      loaders: {
        common: {
          "en-GB": async () => common({ welcome: "Welcome, {name}!" }),
        },
      },
      onMissingTranslation,
    });
    await lingua.setActiveSections(["common"]);

    expect(lingua.t("common.unknown")).toBe("missing:common.unknown");
    expect(onMissingTranslation).toHaveBeenCalledWith({
      key: "common.unknown",
      locale: "en-GB",
    });
    expect(() => lingua.t("common.welcome")).toThrow(InterpolationError);
    expect(lingua.t("unknown.title")).toBe("missing:unknown.title");
  });

  it.each([
    [
      "string plural",
      common({ member_count: "{count} members" }),
      "must be an object",
    ],
    [
      "missing other",
      common({ member_count: { one: "{count} member" } }),
      'must define an "other" value',
    ],
    [
      "plural without suffix",
      common({ members: { one: "{count} member", other: "{count} members" } }),
      "must use a key ending",
    ],
    [
      "plural placeholder disagreement",
      common({
        member_count: {
          one: "{count} member for {name}",
          other: "{count} members",
        },
      }),
      "must use the same placeholders in every category",
    ],
  ])("rejects an invalid catalog: %s", async (_name, document, message) => {
    const lingua = createLingua({
      baseLocale: "en-GB",
      loaders: { common: { "en-GB": async () => document } },
    });

    await expect(lingua.setActiveSections(["common"])).rejects.toThrow(
      CatalogValidationError,
    );
    await expect(lingua.setActiveSections(["common"])).rejects.toThrow(message);
  });

  it("validates base locales and section documents", async () => {
    expect(() =>
      createLingua({
        baseLocale: "de-DE",
        loaders: {
          common: { "en-GB": async () => common({ close: "Close" }) },
        },
      }),
    ).toThrow(LinguaError);

    const lingua = createLingua({
      baseLocale: "en-GB",
      loaders: { common: { "en-GB": async () => ({ other: {} }) } },
    });
    await expect(lingua.setActiveSections(["common"])).rejects.toThrow(
      'must contain a top-level "common" object',
    );

    expect(() =>
      createLingua({
        baseLocale: "not_a_locale",
        loaders: {
          common: { not_a_locale: async () => common({ close: "Close" }) },
        },
      }),
    ).toThrow("Invalid locale identifier");
  });

  it("rejects invalid constructor configuration", () => {
    expect(() =>
      createLingua({
        baseLocale: "en-GB",
        initialLocale: "fr" as never,
        loaders: {
          common: { "en-GB": async () => common({ close: "Close" }) },
        },
      }),
    ).toThrow('Unknown initial locale "fr"');

    expect(() =>
      createLingua({
        baseLocale: "en-GB",
        loaders: {
          common: { "en-GB": async () => common({ close: "Close" }) },
          room: { "de-DE": async () => ({ room: { title: "Raum" } }) },
        },
      }),
    ).toThrow('Section "room" does not define the base locale "en-GB"');

    expect(() =>
      createLingua({
        baseLocale: "en-GB",
        initialBaseCatalogs: { room: common({ title: "Unknown" }) } as never,
        loaders: {
          common: { "en-GB": async () => common({ close: "Close" }) },
        },
      }),
    ).toThrow('Unknown initial catalog section "room"');

    expect(() =>
      createLingua({
        baseLocale: "en-GB",
        fallbackLocales: { fr: "en-GB" } as never,
        loaders: { common: { "en-GB": async () => common({ close: "Close" }) } },
      }),
    ).toThrow('Unknown fallback locale "fr"');

    expect(() =>
      createLingua({
        baseLocale: "en-GB",
        fallbackLocales: { "de-DE": "fr" } as never,
        loaders: {
          common: {
            "en-GB": async () => common({ close: "Close" }),
            "de-DE": async () => common({ close: "Schließen" }),
          },
        },
      }),
    ).toThrow('Unknown fallback target "fr"');

    expect(() =>
      createLingua({
        baseLocale: "en-GB",
        fallbackLocales: { "de-DE": "de-DE" },
        loaders: {
          common: {
            "en-GB": async () => common({ close: "Close" }),
            "de-DE": async () => common({ close: "Schließen" }),
          },
        },
      }),
    ).toThrow('cannot fall back to itself');

    expect(() =>
      createLingua({
        baseLocale: "en-GB",
        fallbackLocales: { "de-DE": "de-AT", "de-AT": "de-DE" },
        loaders: {
          common: {
            "en-GB": async () => common({ close: "Close" }),
            "de-DE": async () => common({ close: "Schließen" }),
            "de-AT": async () => common({ close: "Schließen" }),
          },
        },
      }),
    ).toThrow('Locale fallback cycle includes "de-DE"');
  });

  it("rejects runtime misuse from untyped callers", async () => {
    const lingua = createLingua({
      baseLocale: "en-GB",
      loaders: {
        common: {
          "en-GB": async () =>
            common({
              close: "Close",
              member_count: { one: "{count} member", other: "{count} members" },
              nested: { title: "Title" },
            }),
        },
      },
    });
    await lingua.setActiveSections(["common"]);

    expect(() => (lingua.t as (key: string) => string)("common")).toThrow(
      "must include a section and message path",
    );
    expect((lingua.t as (key: string) => string)("common.nested")).toBe(
      "⟦common.nested⟧",
    );
    expect(() =>
      (lingua.t as (key: string, values?: { count?: number }) => string)(
        "common.member_count",
      ),
    ).toThrow("requires a finite count");
    expect(() =>
      (lingua.t as (key: string, values: { count: number }) => string)(
        "common.member_count",
        { count: Number.POSITIVE_INFINITY },
      ),
    ).toThrow("requires a finite count");
    await expect(
      (lingua.setLocale as (locale: string) => Promise<void>)("fr"),
    ).rejects.toThrow('Unknown locale "fr"');
    await expect(
      (
        lingua.setActiveSections as (
          sections: readonly string[],
        ) => Promise<void>
      )(["room"]),
    ).rejects.toThrow('Unknown translation section "room"');
  });

  it("falls back when a known locale has no loader for an active section", async () => {
    const loadRoom = vi.fn(async () => ({ room: { title: "Room" } }));
    const lingua = createLingua({
      baseLocale: "en-GB",
      initialLocale: "de-DE",
      loaders: {
        common: {
          "en-GB": async () => common({ close: "Close" }),
          "de-DE": async () => common({ close: "Schließen" }),
        },
        room: { "en-GB": loadRoom },
      },
    });

    await lingua.setActiveSections(["common", "room", "room"]);
    expect(lingua.snapshot.activeSections).toEqual(["common", "room"]);
    expect(lingua.t("room.title")).toBe("Room");
    expect(loadRoom).toHaveBeenCalledOnce();
  });

  it("rejects catalogs mutated after validation", async () => {
    const values: Record<string, unknown> = {
      close: "Close",
      member_count: { one: "{count} member", other: "{count} members" },
    };
    const lingua = createLingua({
      baseLocale: "en-GB",
      loaders: { common: { "en-GB": async () => common(values) } },
    });
    await lingua.setActiveSections(["common"]);

    values.member_count = "mutated into text";
    expect(() =>
      (lingua.t as (key: string, values: { count: number }) => string)(
        "common.member_count",
        { count: 2 },
      ),
    ).toThrow(
      'Plural translation "common.member_count" is not a plural object',
    );

    values.close = { one: "mutated", other: "mutated" };
    expect(() => lingua.t("common.close")).toThrow(
      'Translation "common.close" is unexpectedly plural',
    );
  });

  it("rejects a loader registry mutated after construction", async () => {
    const loaders = {
      common: { "en-GB": async () => common({ close: "Close" }) },
    };
    const lingua = createLingua({ baseLocale: "en-GB", loaders });
    delete (loaders.common as Partial<typeof loaders.common>)["en-GB"];

    await expect(lingua.preload("en-GB", ["common"])).rejects.toThrow(
      'No loader for section "common" and locale "en-GB"',
    );
  });

  it("makes eager base catalogs available before asynchronous section activation", () => {
    const base = common({ close: "Close" });
    const loader = vi.fn(async () => base);
    const lingua = createLingua({
      baseLocale: "en-GB",
      initialBaseCatalogs: { common: base },
      loaders: { common: { "en-GB": loader } },
    });

    expect(lingua.t("common.close")).toBe("Close");
    expect(loader).not.toHaveBeenCalled();
  });
});
