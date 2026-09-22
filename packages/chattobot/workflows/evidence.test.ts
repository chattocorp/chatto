import { mkdtemp, rm, writeFile, symlink } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterEach, expect, test } from "vitest";
import { verifyFinding, renderFindings, evidenceCollector, type Finding } from "./evidence.ts";
import type { AgentExtensionAPI } from "runling/agents";

const folders: string[] = [];
afterEach(async () => { await Promise.all(folders.splice(0).map(path => rm(path, { recursive: true, force: true }))); });
async function fixture() {
  const root = await mkdtemp(join(tmpdir(), "source-evidence-"));
  folders.push(root);
  await writeFile(join(root, "example.ts"), "first\nconst answer = 42;\nlast\n");
  const finding: Finding = { claim: "The answer is constant.", kind: "observation", evidence: [{ path: "example.ts", startLine: 2, endLine: 2, quote: "const answer = 42;" }] };
  return { root, finding };
}
test("checks exact source lines without claiming reproduction or applied changes", async () => {
  const { root, finding } = await fixture();
  await verifyFinding(root, finding, new AbortController().signal);
  expect(renderFindings([finding])).toContain("example.ts:2-2");
  expect(renderFindings([finding])).toContain("Tests were not run.");
  expect(renderFindings([])).toContain("No checked source evidence");
});
test.each([
  { startLine: 1, endLine: 1 }, { endLine: 10 }, { startLine: 0 },
  { quote: "invented quote" }, { path: "../outside.ts" }, { path: "/etc/passwd" }, { path: "missing.ts" },
])("rejects invalid citations %j", async invalid => {
  const { root, finding } = await fixture();
  Object.assign(finding.evidence[0]!, invalid);
  await expect(verifyFinding(root, finding, new AbortController().signal)).rejects.toThrow();
});
test("rejects symlinks outside the checkout and cancelled reads", async () => {
  const { root, finding } = await fixture();
  const outside = await fixture();
  await symlink(join(outside.root, "example.ts"), join(root, "escape.ts"));
  finding.evidence[0]!.path = "escape.ts";
  await expect(verifyFinding(root, finding, new AbortController().signal)).rejects.toThrow("inside the checkout");
  await expect(verifyFinding(root, finding, AbortSignal.abort())).rejects.toThrow();
});

test("the tool records only accepted citations and bounds report size", async () => {
  const { root, finding } = await fixture();
  const collector = evidenceCollector(root, new AbortController().signal);
  let execute!: (id: string, value: Finding) => Promise<{ isError?: boolean }>;
  const factory = typeof collector.extension === "function" ? collector.extension : collector.extension.factory;
  await factory({ registerTool(tool: { execute: typeof execute }) { execute = tool.execute; } } as unknown as AgentExtensionAPI);
  const invalid = structuredClone(finding);
  invalid.evidence[0]!.startLine = 1;
  expect(await execute("bad", invalid)).toMatchObject({ isError: true });
  expect(collector.findings).toEqual([]);
  await execute("good", finding);
  expect(collector.findings).toEqual([finding]);
  const large = { ...finding, suggestedChange: "x".repeat(4000) };
  for (let i = 0; i < 4; i++) await execute(`more-${i}`, large);
  await expect(execute("overflow", large)).rejects.toThrow("Evidence budget");
  expect(JSON.stringify(collector.findings).length).toBeLessThan(20_000);
});
