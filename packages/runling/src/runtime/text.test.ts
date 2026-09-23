import { describe, expect, test } from "vitest";
import { containsMalformedToolCall, toSingleLine } from "./text.ts";

describe("toSingleLine", () => {
  test("normalizes whitespace without removing content", () => {
    expect(toSingleLine("  first line\n\nsecond\tline  ")).toBe(
      "first line second line",
    );
  });

  test("does not truncate long responses", () => {
    const response = "x".repeat(500);
    expect(toSingleLine(response)).toBe(response);
  });
});

describe("containsMalformedToolCall", () => {
  test("recognizes a tool call emitted as text", () => {
    expect(
      containsMalformedToolCall('functions.bash:0{"command":"pnpm test"}'),
    ).toBe(true);
  });

  test("ignores regular assistant text", () => {
    expect(containsMalformedToolCall("Changed the requested files.")).toBe(false);
  });
});
