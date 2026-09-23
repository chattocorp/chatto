import { describe, expect, test } from "vitest";
import { randomId } from "./id.ts";

describe("randomId", () => {
  test("returns two lowercase words and a four-digit number", () => {
    expect(randomId()).toMatch(/^[a-z]+-[a-z]+-\d{4}$/);
  });
});
