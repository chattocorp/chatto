import { describe, expect, test } from "vitest";
import { concat } from "./utils.ts";

describe("concat", () => {
  test("joins its arguments with newlines", () => {
    expect(concat("first", "second", "third")).toBe(
      "first\nsecond\nthird",
    );
  });

  test("preserves explicit blank lines", () => {
    expect(concat("first", "", "second")).toBe("first\n\nsecond");
  });

  test("flattens array arguments", () => {
    expect(concat("first", ["second", ["third", "fourth"]])).toBe(
      "first\nsecond\nthird\nfourth",
    );
  });

  test("returns an empty string without arguments", () => {
    expect(concat()).toBe("");
  });
});
