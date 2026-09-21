import { describe, expect, test } from "vitest";
import { parseModelReference } from "./model.ts";

describe("parseModelReference", () => {
  test("preserves slashes within the model ID", () => {
    expect(parseModelReference("openrouter/z-ai/glm-5.3-flash")).toEqual({
      provider: "openrouter",
      id: "z-ai/glm-5.3-flash",
    });
  });

  test("rejects malformed references", () => {
    expect(() => parseModelReference("glm-5.3-flash")).toThrow(
      'expected "provider/model-id"',
    );
    expect(() => parseModelReference("openrouter/")).toThrow(
      'expected "provider/model-id"',
    );
  });
});
