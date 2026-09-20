import { describe, expect, test } from "vitest";
import { displayPath, displayText } from "./paths.ts";

describe("displayPath", () => {
  const cwd = "/tmp/project";

  test("strips the cwd prefix", () => {
    expect(displayPath("/tmp/project/src/agent.ts", cwd)).toBe("src/agent.ts");
  });

  test("returns . for the cwd itself", () => {
    expect(displayPath("/tmp/project", cwd)).toBe(".");
  });

  test("leaves paths outside cwd untouched", () => {
    expect(displayPath("/tmp/other/file.ts", cwd)).toBe("/tmp/other/file.ts");
  });

  test("does not strip a mere prefix without a separator", () => {
    expect(displayPath("/tmp/project-x/file.ts", cwd)).toBe(
      "/tmp/project-x/file.ts",
    );
  });

  test("keeps paths unchanged without an explicit base", () => {
    expect(displayPath(`${process.cwd()}/src/paths.ts`)).toBe(`${process.cwd()}/src/paths.ts`);
  });
});

describe("displayText", () => {
  const cwd = "/tmp/project";

  test("removes embedded absolute cwd paths", () => {
    expect(displayText("pnpm test /tmp/project/src/agent.ts", cwd)).toBe(
      "pnpm test src/agent.ts",
    );
  });

  test("leaves text without cwd references untouched", () => {
    expect(displayText("pnpm test src/agent.ts", cwd)).toBe(
      "pnpm test src/agent.ts",
    );
  });

  test("handles multiple occurrences", () => {
    expect(
      displayText("cat /tmp/project/a.ts /tmp/project/b.ts", cwd),
    ).toBe("cat a.ts b.ts");
  });

  test("keeps paths unchanged without an explicit base", () => {
    expect(displayText(`cat ${process.cwd()}/index.ts`)).toBe(`cat ${process.cwd()}/index.ts`);
  });
});
