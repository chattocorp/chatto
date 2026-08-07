import { describe, expect, it } from "vitest";

import {
  asLocalizedHtml,
  isCountTranslationKey,
  isHtmlTranslationKey,
} from "./conventions.js";

describe("translation key conventions", () => {
  it.each([
    "common.member_count",
    "common.member.count",
    "common.member_count_html",
    "common.member.count_html",
    "common.member.count.html",
  ])("recognises the count suffix in %s", (key) => {
    expect(isCountTranslationKey(key)).toBe(true);
  });

  it.each(["common.discount", "common.counter", "common.html_counted"])(
    "does not mistake %s for a count key",
    (key) => {
      expect(isCountTranslationKey(key)).toBe(false);
    },
  );

  it.each(["common.warning_html", "common.warning.html"])(
    "recognises the HTML suffix in %s",
    (key) => {
      expect(isHtmlTranslationKey(key)).toBe(true);
    },
  );

  it("brands HTML without changing its runtime value", () => {
    expect(asLocalizedHtml("<strong>Safe boundary</strong>")).toBe(
      "<strong>Safe boundary</strong>",
    );
  });
});
