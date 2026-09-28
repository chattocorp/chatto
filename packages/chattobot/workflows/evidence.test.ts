import { mkdtemp, rm, writeFile, symlink } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { afterEach, expect, test } from 'vitest';
import {
  CitationError,
  verifyFinding,
  renderFindings,
  evidenceCollector,
  withoutExcerpts,
  type Finding
} from './evidence.ts';
import type { AgentExtensionAPI } from 'runling/agents';

const folders: string[] = [];
afterEach(async () => {
  await Promise.all(folders.splice(0).map((path) => rm(path, { recursive: true, force: true })));
});
async function fixture() {
  const root = await mkdtemp(join(tmpdir(), 'source-evidence-'));
  folders.push(root);
  await writeFile(join(root, 'example.ts'), 'first\nconst answer = 42;\nlast\n');
  const finding: Finding = {
    claim: 'The answer is constant.',
    kind: 'observation',
    evidence: [{ path: 'example.ts', startLine: 2, endLine: 2, quote: 'const answer = 42;' }]
  };
  return { root, finding };
}
test('checks exact source lines without claiming reproduction or applied changes', async () => {
  const { root, finding } = await fixture();
  await verifyFinding(root, finding, new AbortController().signal);
  expect(renderFindings([finding])).toContain('example.ts:2-2');
  expect(renderFindings([finding])).toContain('Tests were not run.');
  expect(renderFindings([])).toContain('No checked source evidence');
});

test('extracts source excerpts without asking the agent to reproduce tabs and whitespace', async () => {
  const { root, finding } = await fixture();
  await writeFile(join(root, 'example.ts'), 'first\n\tconst answer = 42;\nlast\n');
  delete finding.evidence[0]!.quote;
  await verifyFinding(root, finding, new AbortController().signal);
  expect(finding.evidence[0]!.quote).toBe('\tconst answer = 42;');
});
test.each([
  { startLine: 1, endLine: 1 },
  { endLine: 10 },
  { startLine: 0 },
  { quote: 'invented quote' },
  { path: '../outside.ts' },
  { path: '/etc/passwd' },
  { path: 'missing.ts' }
])('rejects invalid citations %j', async (invalid) => {
  const { root, finding } = await fixture();
  Object.assign(finding.evidence[0]!, invalid);
  await expect(verifyFinding(root, finding, new AbortController().signal)).rejects.toThrow();
});
test('rejects symlinks outside the checkout and cancelled reads', async () => {
  const { root, finding } = await fixture();
  const outside = await fixture();
  await symlink(join(outside.root, 'example.ts'), join(root, 'escape.ts'));
  finding.evidence[0]!.path = 'escape.ts';
  await expect(verifyFinding(root, finding, new AbortController().signal)).rejects.toThrow(
    'inside the checkout'
  );
  await expect(verifyFinding(root, finding, AbortSignal.abort())).rejects.toThrow();
});

test('the tool records only accepted citations and bounds the number of findings', async () => {
  const { root, finding } = await fixture();
  const collector = evidenceCollector(root, new AbortController().signal);
  let execute!: (id: string, value: Finding) => Promise<{ isError?: boolean }>;
  const factory =
    typeof collector.extension === 'function' ? collector.extension : collector.extension.factory;
  await factory({
    registerTool(tool: { execute: typeof execute }) {
      execute = tool.execute;
    }
  } as unknown as AgentExtensionAPI);
  const invalid = structuredClone(finding);
  invalid.evidence[0]!.startLine = 1;
  expect(await execute('bad', invalid)).toMatchObject({ isError: true });
  expect(collector.findings).toEqual([]);
  await execute('good', finding);
  expect(collector.findings).toEqual([finding]);
  for (let i = 1; i < 12; i++) await execute(`more-${i}`, finding);
  expect(collector.findings).toHaveLength(12);
  // The finding limit is a normal answer, not a tool failure that would stop the task.
  const overflow = (await execute('overflow', finding)) as {
    isError?: boolean;
    content: { text: string }[];
  };
  expect(overflow.isError).toBeUndefined();
  expect(overflow.content[0]!.text).toMatch(/finding limit \(12\) is reached/);
  expect(collector.findings).toHaveLength(12);
});

test('results carry findings without their checked excerpts', async () => {
  const { finding } = await fixture();
  const checked = {
    ...finding,
    evidence: [{ path: 'example.txt', startLine: 1, endLine: 1, quote: 'original' }]
  };
  const [stripped] = withoutExcerpts([checked]);
  expect(stripped!.evidence).toEqual([{ path: 'example.txt', startLine: 1, endLine: 1 }]);
  expect(checked.evidence[0]!.quote).toBe('original');
  expect(renderFindings([stripped!])).toContain('example.txt:1-1');
  expect(renderFindings([stripped!])).not.toContain('original');
});

test('rejections say what to correct, without host paths', async () => {
  const { root, finding } = await fixture();
  const cases: [Partial<Finding['evidence'][number]>, RegExp][] = [
    [{ path: 'missing.ts' }, /missing\.ts does not exist in the checkout/],
    [{ startLine: 1, endLine: 999 }, /is not a line range of the file/],
    [{ quote: 'something else' }, /The quote does not match/]
  ];
  for (const [change, message] of cases) {
    const citation = { ...finding.evidence[0]!, ...change };
    const error = await verifyFinding(
      root,
      { ...finding, evidence: [citation] },
      new AbortController().signal
    ).catch((reason: Error) => reason);
    expect(error).toBeInstanceOf(CitationError);
    expect((error as Error).message).toMatch(message);
    expect((error as Error).message).not.toContain(root);
  }
});

test('a citation covers at most 40 lines', async () => {
  const { root, finding } = await fixture();
  const { writeFile } = await import('node:fs/promises');
  const { join } = await import('node:path');
  await writeFile(
    join(root, 'long.ts'),
    Array.from({ length: 60 }, (_, n) => `line ${n}`).join('\n')
  );
  const citation = { path: 'long.ts', startLine: 1, endLine: 41 };
  await expect(
    verifyFinding(root, { ...finding, evidence: [citation] }, new AbortController().signal)
  ).rejects.toThrow('covers more than 40 lines');
  await verifyFinding(
    root,
    { ...finding, evidence: [{ ...citation, endLine: 40 }] },
    new AbortController().signal
  );
});
