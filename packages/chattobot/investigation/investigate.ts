/**
 * The investigator: a read-only subagent that answers questions about the code in a directory.
 * Each investigation keeps its agent session in a file, so a caller can resume it later with a
 * follow-up question, also after the investigation ended. The caller chooses the directory, for
 * example a checkout or a worktree.
 *
 * Run it on its own: `runling run investigation/investigate.ts --input '{"directory":
 * "/path/to/repo", "question": "..."}'`.
 */
import { mkdir, stat } from 'node:fs/promises';
import { resolve } from 'node:path';
import {
  agent,
  task,
  Type,
  type AgentOptions,
  type RunlingAgent,
  type TaskContext,
  type ThinkingLevel
} from 'runling';
import { ConfigurationError, setting, thinkingSetting } from '../settings.ts';
import { checkCitations, findCitations } from './citations.ts';
import { gitExtension, runGit } from './git.ts';

export interface InvestigatorSettings {
  model: string;
  thinkingLevel?: ThinkingLevel;
  /** Where investigations keep their agent sessions. The files contain the code that the
   * investigator read and the questions, so keep the directory private. */
  sessions: string;
}

const Input = Type.Object({
  directory: Type.String({ minLength: 1, description: 'The directory to investigate' }),
  question: Type.String({ minLength: 1, maxLength: 20_000 }),
  session: Type.Optional(
    Type.String({
      pattern: '^[0-9a-f-]{36}$',
      description: 'The ID of an earlier investigation to continue'
    })
  )
});

const Output = Type.Object({
  /** Pass it as `session` to continue this investigation. */
  session: Type.String(),
  answer: Type.String(),
  /** The commit of the investigated directory, when it is a Git checkout. */
  commit: Type.Optional(Type.String()),
  /** File references in the answer that name existing lines. */
  citations: Type.Array(
    Type.Object({ path: Type.String(), start: Type.Integer(), end: Type.Integer() })
  )
});

const INSTRUCTIONS = [
  'You investigate the repository in your working directory to answer questions from another agent. You can only read: you cannot change files or run programs.',
  'Start from what the repository documents about itself where it exists: AGENTS.md files, architecture and decision-record indexes, and the glossary. Then read the code that the question is about. Use git to see when and why code changed.',
  'Answer the question directly. Support each claim with a reference to lines that you read, as `path:line` or `path:start-end` with the full path relative to the repository root, also when you cite the same file again. Say which statements are inferences.',
  'End with "Checked:" (what you read) and "Not checked:" (what could change the answer). If you cannot answer from the code, say so.',
  'Be concise. The reader is an agent that will summarize your answer.'
];

/** Investigations that run now, by session ID. One session runs one question at a time. */
const active = new Set<string>();

/** Create the investigator task. `createAgent` is injectable for tests. */
export function createInvestigator(
  settings: InvestigatorSettings,
  createAgent: (options: AgentOptions) => Promise<RunlingAgent> = agent
) {
  return task({ name: 'Investigate', input: Input, output: Output }, async (ctx, input) => {
    const directory = resolve(input.directory);
    if (!(await stat(directory).catch(() => undefined))?.isDirectory())
      throw new Error('The directory to investigate does not exist');
    await mkdir(settings.sessions, { recursive: true, mode: 0o700 });
    const session = input.session ?? crypto.randomUUID();
    const sessionFile = resolve(settings.sessions, `${session}.jsonl`);
    if (input.session && !(await stat(sessionFile).catch(() => undefined)))
      throw new Error(`There is no investigation ${input.session}`);
    if (active.has(session))
      throw new Error(`Investigation ${session} is answering another question. Wait for it.`);
    active.add(session);
    try {
      await using investigator = await createAgent({
        label: 'investigate',
        model: settings.model,
        thinkingLevel: settings.thinkingLevel ?? 'medium',
        output: 'text',
        cwd: directory,
        sessionFile,
        tools: ['read', 'grep', 'find', 'ls', 'git'],
        extensions: [gitExtension(directory)],
        // Repository instructions help; host extensions and skills do not belong here.
        resources: { extensions: false, skills: false, promptTemplates: false, themes: false },
        instructions: INSTRUCTIONS
      });
      let answer = await answerOf(investigator, ctx, input.question);
      let checked = await checkCitations(directory, findCitations(answer));
      const problems = checked.flatMap((citation) => citation.problem ?? []);
      // One chance to correct invalid references; the answer is kept either way.
      if (problems.length) {
        answer = await answerOf(
          investigator,
          ctx,
          `Some references in your answer are wrong:\n${problems.map((problem) => `- ${problem}`).join('\n')}\nCheck them, then write your complete answer again with correct references.`
        );
        checked = await checkCitations(directory, findCitations(answer));
      }
      const commit = await runGit(directory, ['rev-parse', 'HEAD'], ctx.signal).then(
        (output) => output.trim(),
        () => undefined
      );
      return {
        session,
        answer,
        ...(commit ? { commit } : {}),
        citations: checked
          .filter((citation) => !citation.problem)
          .map(({ path, start, end }) => ({ path, start, end }))
      };
    } finally {
      active.delete(session);
    }
  });
}

async function answerOf(investigator: RunlingAgent, ctx: TaskContext<any, any>, prompt: string) {
  const result = await investigator.runOutcome(ctx, prompt);
  if (result.outcome !== 'completed') throw new Error(result.summary);
  return result.summary;
}

/** Investigator settings from the environment. */
export function investigatorSettings(): InvestigatorSettings {
  const model = setting('CHATTO_INVESTIGATION_MODEL') ?? setting('CHATTO_AGENT_MODEL');
  if (!model) throw new ConfigurationError('Set CHATTO_INVESTIGATION_MODEL or CHATTO_AGENT_MODEL');
  return {
    model,
    thinkingLevel: thinkingSetting('CHATTO_INVESTIGATION_THINKING', 'medium'),
    // The old investigations directory holds worktrees of the previous bot; keep them apart.
    sessions: resolve('.runling/subagents/investigate')
  };
}

/** The investigator with settings from the environment, for `runling run`. */
export default task({ name: 'Investigate code', input: Input, output: Output }, (ctx, input) =>
  createInvestigator(investigatorSettings())(ctx, input)
);
