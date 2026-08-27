import { describe, expect, it } from "vitest";

import {
  extractSection,
  findTranslation,
  selectPlural,
  validateTranslationOverlay,
} from "./catalog.js";
import { CatalogValidationError } from "./errors.js";
import type { TranslationObject } from "./types.js";

describe("catalog validation", () => {
  it.each([
    [
      "an unknown plural category",
      { common: { member_count: { singular: "one", other: "many" } } },
      "only CLDR plural categories",
    ],
    [
      "a non-string plural branch",
      { common: { member_count: { one: 1, other: "many" } } },
      "only CLDR plural categories",
    ],
    [
      "a non-string leaf",
      { common: { title: null } },
      "must be a string or object",
    ],
  ])("rejects %s", (_name, document, message) => {
    expect(() => extractSection(document, "common")).toThrow(
      CatalogValidationError,
    );
    expect(() => extractSection(document, "common")).toThrow(message);
  });

  it("rejects every translated/base value-kind mismatch defensively", () => {
    expect(() =>
      validateTranslationOverlay(
        { nested: { title: "Base" } },
        { nested: "Translated" },
        "common",
      ),
    ).toThrow("same value kind as the base locale");

    expect(() =>
      validateTranslationOverlay(
        { title: "Base" },
        { title: { nested: "Translated" } },
        "common",
      ),
    ).toThrow("same value kind as the base locale");

    expect(() =>
      validateTranslationOverlay(
        { member_count: { one: "one", other: "many" } },
        {
          member_count: { nested: "Translated" },
        } as unknown as TranslationObject,
        "common",
      ),
    ).toThrow("same value kind as the base locale");
  });

  it("unwraps raw and default-export catalog modules", () => {
    const section = { close: "Close" };
    expect(extractSection({ common: section }, "common")).toEqual(section);
    expect(extractSection({ default: { common: section } }, "common")).toEqual(
      section,
    );
  });

  it("only resolves strings and plural leaves", () => {
    const section = {
      nested: { title: "Title" },
      member_count: { one: "one", other: "many" },
    } satisfies TranslationObject;

    expect(findTranslation(section, ["nested", "title"])).toBe("Title");
    expect(findTranslation(section, ["nested"])).toBeUndefined();
    expect(findTranslation(section, ["missing"])).toBeUndefined();
    expect(findTranslation(undefined, ["missing"])).toBeUndefined();
    expect(findTranslation(section, ["member_count"])).toEqual({
      one: "one",
      other: "many",
    });
  });

  it("falls back to the other plural category", () => {
    expect(selectPlural({ one: "one", other: "many" }, "few")).toBe("many");
  });
});
