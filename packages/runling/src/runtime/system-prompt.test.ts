import { describe, expect, test } from "vitest";
import { formatAgentInstructions } from "./system-prompt.ts";

describe("formatAgentInstructions", () => {
  test("formats userland instructions as a system-prompt section", () => {
    expect(
      formatAgentInstructions([
        "Write tests for new features.",
        "Keep changes focused.",
      ]),
    ).toBe(
      [
        "Additional instructions for this run:",
        "- Write tests for new features.",
        "- Keep changes focused.",
      ].join("\n"),
    );
  });

  test("omits the section when no instructions are provided", () => {
    expect(formatAgentInstructions([])).toBeUndefined();
  });
});
