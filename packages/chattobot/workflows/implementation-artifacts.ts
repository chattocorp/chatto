/** Implementation input and private on-disk recovery state. */
import { lstat, readdir, readFile, stat } from 'node:fs/promises';
import { resolve } from 'node:path';
import { Type, type Static } from 'runling';
import { implementationPlanSchema } from './plan.ts';

/** Input accepted by an implementation task. */
export const implementationInput = Type.Object({
  request: Type.String({ minLength: 1, maxLength: 12_000 }),
  context: Type.Optional(Type.String({ maxLength: 24_000 })),
  plan: Type.Optional(implementationPlanSchema),
  resumeArtifactId: Type.Optional(Type.String({ pattern: '^implementation-[A-Za-z0-9_-]{6,}$' }))
});
export type ImplementationInput = Static<typeof implementationInput>;

/** Most follow-up instructions kept with one artifact; older ones are dropped first. */
export const MAX_FOLLOW_UPS = 5;

/** Worker-authored continuation notes, bounded before they reach a new model session. */
export const handoffSchema = Type.Object({
  summary: Type.String({ minLength: 1, maxLength: 2000 }),
  nextSteps: Type.Array(Type.String({ minLength: 1, maxLength: 500 }), { maxItems: 8 }),
  risks: Type.Array(Type.String({ minLength: 1, maxLength: 500 }), { maxItems: 8 })
});

/** Private on-disk recovery state. Only the matching conversation can reuse it. */
export interface ImplementationMetadata {
  branch: string;
  baseBranch: string;
  baseCommit: string;
  repository: string;
  stage:
    | 'setup'
    | 'editing'
    | 'blocked'
    | 'interrupted'
    | 'publishing'
    | 'pushed'
    | 'published'
    | 'publication_unknown';
  /** Hash of the conversation's thread key, never raw Chatto identifiers. */
  ownerKey?: string;
  /** Original input, retained so a new worker can continue the same request. */
  input?: Pick<ImplementationInput, 'request' | 'context'> & { plan?: unknown };
  /** Worker-authored continuation notes. The resumed worker must verify them against the diff. */
  handoff?: { summary: string; nextSteps: string[]; risks: string[] };
  /** Later user instructions from resume requests, oldest first. They refine the original request. */
  followUps?: Pick<ImplementationInput, 'request' | 'context'>[];
  commit?: string;
  prUrl?: string;
}
/** Bound worker handoff data before sending retained notes to a new model session. */
function isHandoff(value: unknown): value is NonNullable<ImplementationMetadata['handoff']> {
  if (typeof value !== 'object' || value === null || Array.isArray(value)) return false;
  const handoff = value as Record<string, unknown>;
  const shortList = (items: unknown) =>
    Array.isArray(items) &&
    items.length <= 8 &&
    items.every((item) => typeof item === 'string' && item.length > 0 && item.length <= 500);
  return (
    typeof handoff.summary === 'string' &&
    handoff.summary.length > 0 &&
    handoff.summary.length <= 2000 &&
    shortList(handoff.nextSteps) &&
    shortList(handoff.risks)
  );
}
/** Reject malformed or incomplete local metadata before selecting a worktree. */
function isImplementationMetadata(value: unknown): value is ImplementationMetadata {
  if (typeof value !== 'object' || value === null || Array.isArray(value)) return false;
  const metadata = value as Record<string, unknown>;
  if (
    typeof metadata.input !== 'object' ||
    metadata.input === null ||
    Array.isArray(metadata.input)
  )
    return false;
  const input = metadata.input as Record<string, unknown>;
  return (
    typeof metadata.branch === 'string' &&
    typeof metadata.baseBranch === 'string' &&
    typeof metadata.baseCommit === 'string' &&
    typeof metadata.repository === 'string' &&
    typeof metadata.stage === 'string' &&
    typeof metadata.ownerKey === 'string' &&
    typeof input.request === 'string' &&
    (input.context === undefined || typeof input.context === 'string') &&
    (metadata.commit === undefined || typeof metadata.commit === 'string') &&
    (metadata.prUrl === undefined || typeof metadata.prUrl === 'string') &&
    (metadata.handoff === undefined || isHandoff(metadata.handoff)) &&
    (metadata.followUps === undefined ||
      (Array.isArray(metadata.followUps) &&
        metadata.followUps.length <= MAX_FOLLOW_UPS &&
        metadata.followUps.every(
          (followUp) =>
            typeof followUp === 'object' &&
            followUp !== null &&
            typeof followUp.request === 'string' &&
            (followUp.context === undefined || typeof followUp.context === 'string')
        )))
  );
}

/** Stages whose artifacts can continue: they have neither a commit nor a pull request. */
const RESUMABLE_STAGES = ['setup', 'editing', 'blocked', 'interrupted'];

/** A stopped implementation that this conversation can continue, for the supervisor's prompt. */
export interface ResumableImplementation {
  artifactId: string;
  request: string;
  stage: string;
  /** Time of the artifact's last change, in milliseconds. */
  updatedAt: number;
  /** True when the worker's conversation was saved, so a continuation keeps its full context. */
  sessionSaved: boolean;
}

/** The newest stopped implementations of this conversation, at most `limit`. Unreadable
 * artifacts are skipped. */
export async function listResumableArtifacts(
  artifacts: string,
  expected: { ownerKey: string; repository: string; baseBranch: string },
  limit = 3
): Promise<ResumableImplementation[]> {
  let names: string[];
  try {
    names = (await readdir(artifacts)).filter((name) =>
      /^implementation-[A-Za-z0-9_-]{6,}$/.test(name)
    );
  } catch {
    return [];
  }
  const found: ResumableImplementation[] = [];
  for (const artifactId of names) {
    try {
      const { folder, metadata } = await loadResumableArtifact(artifacts, artifactId, expected);
      const metadataStat = await stat(resolve(folder, 'metadata.json'));
      found.push({
        artifactId,
        request: (metadata.input?.request ?? '').slice(0, 300),
        stage: metadata.stage,
        updatedAt: metadataStat.mtimeMs,
        sessionSaved: await stat(resolve(folder, WORKER_SESSION_FILE)).then(
          (file) => file.size > 0,
          () => false
        )
      });
    } catch {
      // Another conversation's, published, or unreadable artifact.
    }
  }
  return found.sort((a, b) => b.updatedAt - a.updatedAt).slice(0, limit);
}

/** The worker's saved conversation inside an artifact folder. */
export const WORKER_SESSION_FILE = 'worker-session.jsonl';

/** Load a stopped artifact for continuation. It must belong to this conversation, repository, and
 * base branch, and must not have been published. Throws when any check fails. */
export async function loadResumableArtifact(
  artifacts: string,
  artifactId: string,
  expected: { ownerKey?: string; repository: string; baseBranch: string }
): Promise<{ folder: string; metadata: ImplementationMetadata }> {
  if (!expected.ownerKey) throw new Error('Missing conversation identity');
  const folder = resolve(artifacts, artifactId);
  if (!(await lstat(folder)).isDirectory()) throw new Error('Artifact is not a directory');
  const metadata: unknown = JSON.parse(await readFile(resolve(folder, 'metadata.json'), 'utf8'));
  if (
    !isImplementationMetadata(metadata) ||
    metadata.ownerKey !== expected.ownerKey ||
    metadata.repository !== expected.repository ||
    metadata.baseBranch !== expected.baseBranch ||
    metadata.commit ||
    metadata.prUrl ||
    !RESUMABLE_STAGES.includes(metadata.stage) ||
    !/^chattobot\/[0-9a-f-]{36}$/.test(metadata.branch) ||
    !/^[0-9a-f]{40}$/.test(metadata.baseCommit)
  )
    throw new Error('Artifact does not match this conversation');
  return { folder, metadata };
}
