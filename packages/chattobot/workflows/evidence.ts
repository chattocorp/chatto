import { readFile, realpath, stat } from 'node:fs/promises';
import { isAbsolute, relative, resolve } from 'node:path';
import { Type, type Static } from 'runling';
import { defineAgentExtension } from 'runling/agents';

const citationSchema = Type.Object({
  path: Type.String({ minLength: 1, maxLength: 500 }),
  startLine: Type.Integer({ minimum: 1 }),
  endLine: Type.Integer({ minimum: 1 }),
  quote: Type.Optional(
    Type.String({
      minLength: 1,
      maxLength: 8000,
      description: 'Optional exact quote. Omit to let the host extract the cited lines.'
    })
  )
});

/** Claims remain model-authored. The host verifies only the cited source excerpt. */
export const findingSchema = Type.Object({
  claim: Type.String({ minLength: 1, maxLength: 3000 }),
  kind: Type.String({
    enum: ['observation', 'hypothesis'],
    description:
      'Use observation for what the quoted code directly shows, or hypothesis for an inference. These are the only allowed values.'
  }),
  evidence: Type.Array(citationSchema, { minItems: 1, maxItems: 5 }),
  suggestedChange: Type.Optional(Type.String({ maxLength: 4000 })),
  limitations: Type.Optional(
    Type.Array(Type.String({ minLength: 1, maxLength: 1000 }), { maxItems: 5 })
  )
});
export type Finding = Static<typeof findingSchema>;

/** Most lines and characters that one citation can cover: a citation shows the lines that
 * support a claim, not a whole region of a file. */
export const MAX_CITATION_LINES = 40;
export const MAX_CITATION_CHARS = 3_000;
/** Most findings in one investigation. */
export const MAX_FINDINGS = 12;

/** Findings without their checked excerpts. The host checked the excerpts when it recorded the
 * findings; the supervisor needs only the claims and their locations. This keeps a result well
 * below the task channel's 64,000-character limit. */
export function withoutExcerpts(findings: readonly Finding[]): Finding[] {
  return findings.map((finding) => ({
    ...structuredClone(finding),
    evidence: finding.evidence.map(({ path, startLine, endLine }) => ({ path, startLine, endLine }))
  }));
}

/** A citation problem that the investigator can correct. Its message is host-written. */
export class CitationError extends Error {}

/** Validate citations within the retained checkout; reject traversal, external symlinks,
 * oversized files, invalid ranges, and quotes that do not match the specified lines.
 * Fills quote from the source when omitted and normalizes supplied quotes to the
 * original excerpt. This does not prove a claim or reproduce a bug.
 */
export async function verifyFinding(
  root: string,
  finding: Finding,
  signal: AbortSignal
): Promise<void> {
  if (
    !finding.claim.trim() ||
    !['observation', 'hypothesis'].includes(finding.kind) ||
    !finding.evidence.length ||
    finding.evidence.length > 5
  )
    throw new CitationError('A finding needs a claim, a kind, and 1 to 5 citations');
  const base = await realpath(root);
  for (const citation of finding.evidence) {
    signal.throwIfAborted();
    if (isAbsolute(citation.path) || citation.path.split(/[\\/]/).includes('..'))
      throw new CitationError(`${citation.path} is not a relative path inside the checkout`);
    // File system errors name host paths; report only the cited path.
    const path = await realpath(resolve(base, citation.path)).catch(() => {
      throw new CitationError(`${citation.path} does not exist in the checkout`);
    });
    const local = relative(base, path);
    if (!local || local.startsWith('..') || isAbsolute(local))
      throw new CitationError(`${citation.path} is not inside the checkout`);
    const info = await stat(path);
    if (!info.isFile() || info.size > 1_000_000)
      throw new CitationError(`${citation.path} is not a small text file`);
    const lines = (await readFile(path, { encoding: 'utf8', signal }))
      .replace(/\r\n/g, '\n')
      .split('\n');
    const { startLine, endLine } = citation;
    if (
      !Number.isSafeInteger(startLine) ||
      !Number.isSafeInteger(endLine) ||
      startLine < 1 ||
      endLine < startLine ||
      endLine > lines.length
    )
      throw new CitationError(
        `${citation.path}:${startLine}-${endLine} is not a line range of the file, which has ${lines.length} lines`
      );
    if (endLine - startLine >= MAX_CITATION_LINES)
      throw new CitationError(
        `${citation.path}:${startLine}-${endLine} covers more than ${MAX_CITATION_LINES} lines; cite only the lines that show the claim`
      );
    const excerpt = lines.slice(startLine - 1, endLine).join('\n');
    if (!excerpt.trim())
      throw new CitationError(`${citation.path}:${startLine}-${endLine} contains only blank lines`);
    if (excerpt.length > MAX_CITATION_CHARS)
      throw new CitationError(
        `${citation.path}:${startLine}-${endLine} has more than ${MAX_CITATION_CHARS} characters; cite fewer lines`
      );
    if (
      citation.quote !== undefined &&
      (!citation.quote.trim() || excerpt.trim() !== citation.quote.replace(/\r\n/g, '\n').trim())
    )
      throw new CitationError(
        `The quote does not match ${citation.path}:${startLine}-${endLine}; read the file and correct the range, or omit the quote`
      );
    citation.quote = excerpt;
  }
}

/** Per-investigation tool. Only verified citations enter the report returned to the owner. */
export function evidenceCollector(
  root: string,
  signal: AbortSignal,
  onFinding?: (finding: Finding) => Promise<void>
) {
  const findings: Finding[] = [];
  const extension = defineAgentExtension((pi) => {
    // Findings whose citation check is still running.
    let pending = 0;
    pi.registerTool({
      name: 'recordFinding',
      label: 'Record source evidence',
      description: `Record a finding with file paths and line ranges. Omit quotes: the host extracts the exact source. Set kind to observation or hypothesis. The host checks citations, not reasoning. Cite only the lines that show the claim: at most ${MAX_CITATION_LINES} lines and ${MAX_CITATION_CHARS} characters per citation. An investigation records at most ${MAX_FINDINGS} findings.`,
      parameters: findingSchema,
      async execute(_id, finding, toolSignal) {
        const answer = (text: string, accepted: boolean, isError = false) => ({
          content: [{ type: 'text' as const, text }],
          details: { accepted },
          ...(isError ? { isError: true } : {})
        });
        // Limits are normal answers, not tool failures: repeated failures stop the task. Checks in
        // progress count too, so that parallel calls cannot pass the limit.
        if (findings.length + pending >= MAX_FINDINGS)
          return answer(
            `The finding limit (${MAX_FINDINGS}) is reached. Finish your report with the findings recorded so far.`,
            false
          );
        finding = structuredClone(finding);
        pending++;
        try {
          await verifyFinding(
            root,
            finding,
            toolSignal ? AbortSignal.any([signal, toolSignal]) : signal
          );
        } catch (error) {
          pending--;
          signal.throwIfAborted();
          toolSignal?.throwIfAborted();
          return answer(
            `Citation rejected: ${error instanceof CitationError ? error.message : 'the cited file could not be read'}. No finding was recorded.`,
            false,
            true
          );
        }
        pending--;
        signal.throwIfAborted();
        findings.push(structuredClone(finding));
        await onFinding?.(finding);
        return answer(
          `Citation checked and finding recorded. Claim remains unverified by execution. ${MAX_FINDINGS - findings.length} more findings are possible.`,
          true
        );
      }
    });
  });
  return { findings, extension };
}

/** Render checked evidence and fixed host-owned limits; never relay unchecked report prose.
 * Excerpts are included when the findings still have them. */
export function renderFindings(findings: readonly Finding[]): string {
  return findings
    .map((finding, index) =>
      [
        `${index + 1}. ${finding.kind}: ${finding.claim}`,
        ...finding.evidence.map(
          (citation) =>
            `${citation.path}:${citation.startLine}-${citation.endLine}${citation.quote ? `\n${citation.quote}` : ''}`
        ),
        ...(finding.suggestedChange ? [`Proposed, not applied: ${finding.suggestedChange}`] : []),
        ...(finding.limitations ?? []).map((limitation) => `Limitation: ${limitation}`)
      ].join('\n\n')
    )
    .concat(
      `${findings.length ? 'Source citations were checked.' : 'No checked source evidence was collected.'} Claims were not reproduced. Tests were not run. No files were changed by the investigator.`
    )
    .join('\n\n');
}
