import { readFile, realpath, stat } from "node:fs/promises";
import { isAbsolute, relative, resolve } from "node:path";
import { Type, type Static } from "runling";
import { defineAgentExtension } from "runling/agents";

const citationSchema = Type.Object({
  path: Type.String({ minLength: 1, maxLength: 500 }),
  startLine: Type.Integer({ minimum: 1 }),
  endLine: Type.Integer({ minimum: 1 }),
  quote: Type.String({ minLength: 1, maxLength: 8000 }),
});

/** Claims remain model-authored. The host verifies only the cited source excerpt. */
export const findingSchema = Type.Object({
  claim: Type.String({ minLength: 1, maxLength: 3000 }),
  kind: Type.String({ enum: ["observation", "hypothesis"], description: "Use observation for what the quoted code directly shows, or hypothesis for an inference. These are the only allowed values." }),
  evidence: Type.Array(citationSchema, { minItems: 1, maxItems: 5 }),
  suggestedChange: Type.Optional(Type.String({ maxLength: 4000 })),
  limitations: Type.Optional(Type.Array(Type.String({ minLength: 1, maxLength: 1000 }), { maxItems: 5 })),
});
export type Finding = Static<typeof findingSchema>;

/** Validate citations within the retained checkout; reject traversal, external symlinks,
 * oversized files, invalid ranges, and quotes that do not match the specified lines.
 * This does not prove that a claim follows from the excerpt or reproduce a bug.
 */
export async function verifyFinding(root: string, finding: Finding, signal: AbortSignal): Promise<void> {
  if (!finding.claim.trim() || !["observation", "hypothesis"].includes(finding.kind) || !finding.evidence.length || finding.evidence.length > 5) throw new Error("Invalid finding");
  const base = await realpath(root);
  for (const citation of finding.evidence) {
    signal.throwIfAborted();
    if (isAbsolute(citation.path) || citation.path.split(/[\\/]/).includes("..")) throw new Error("Citation must be inside the checkout");
    const path = await realpath(resolve(base, citation.path));
    const local = relative(base, path);
    if (!local || local.startsWith("..") || isAbsolute(local)) throw new Error("Citation must be inside the checkout");
    const info = await stat(path);
    if (!info.isFile() || info.size > 1_000_000) throw new Error("Citation file is not a small text file");
    const lines = (await readFile(path, { encoding: "utf8", signal })).replace(/\r\n/g, "\n").split("\n");
    const { startLine, endLine } = citation;
    if (!Number.isSafeInteger(startLine) || !Number.isSafeInteger(endLine) || startLine < 1 || endLine < startLine || endLine > lines.length || endLine - startLine >= 100) throw new Error("Invalid citation line range");
    if (!citation.quote.trim() || lines.slice(startLine - 1, endLine).join("\n").trim() !== citation.quote.replace(/\r\n/g, "\n").trim()) throw new Error("Citation quote does not match these lines; read the file and correct the range");
  }
}

/** Per-investigation tool. Only verified citations enter the report returned to the owner. */
export function evidenceCollector(root: string, signal: AbortSignal, onFinding?: (finding: Finding) => Promise<void>) {
  const findings: Finding[] = [];
  const extension = defineAgentExtension(pi => {
    pi.registerTool({ name: "recordFinding", label: "Record source evidence",
      description: "Record a finding with exact source quotes and line ranges. Set kind to observation or hypothesis (never fact). The host checks citations, not reasoning. Correct rejected citations before reporting the outcome.",
      parameters: findingSchema,
      async execute(_id, finding, toolSignal) {
        if (findings.length >= 12) throw new Error("Finding limit reached");
        // Reports include both structured findings and rendered text. Keep both
        // below the task channel's 64,000-character result limit, including metadata.
        if (JSON.stringify([...findings, finding]).length > 20_000) throw new Error("Evidence budget reached; keep findings concise");
        try { await verifyFinding(root, finding, toolSignal ? AbortSignal.any([signal, toolSignal]) : signal); }
        catch {
          signal.throwIfAborted();
          toolSignal?.throwIfAborted();
          return { content: [{ type: "text" as const, text: "Citation rejected. Check the relative path, exact quoted lines, and line range. No finding was recorded." }], details: { accepted: false }, isError: true };
        }
        signal.throwIfAborted();
        findings.push(structuredClone(finding));
        await onFinding?.(finding);
        return { content: [{ type: "text" as const, text: "Citation checked and finding recorded. Claim remains unverified by execution." }], details: { accepted: true } };
      },
    });
  });
  return { findings, extension };
}

/** Render checked evidence and fixed host-owned limits; never relay unchecked report prose. */
export function renderFindings(findings: readonly Finding[]): string {
  return findings.map((finding, index) => [
    `${index + 1}. ${finding.kind}: ${finding.claim}`,
    ...finding.evidence.map(citation => `${citation.path}:${citation.startLine}-${citation.endLine}\n${citation.quote}`),
    ...(finding.suggestedChange ? [`Proposed, not applied: ${finding.suggestedChange}`] : []),
    ...(finding.limitations ?? []).map(limitation => `Limitation: ${limitation}`),
  ].join("\n\n")).concat(`${findings.length ? "Source citations were checked." : "No checked source evidence was collected."} Claims were not reproduced. Tests were not run. No files were changed by the investigator.`).join("\n\n");
}
