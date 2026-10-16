/** Replay a reported bug and its withdrawal through the real supervisor and GitHub worker.
 * Only the model provider is external; Chatto messages and GitHub effects are synthetic. */
import assert from 'node:assert/strict';
import { resolve } from 'node:path';
import { createAgentSession, createChannel, type TaskContext } from 'runling';
import { createConversation } from '../tasks/conversation.ts';
import { createGitHubWorker } from '../tasks/github.ts';
import { thinkingSetting } from '../settings.ts';
import { FakeGitHub, issueUrl } from './fake-github.ts';
import { checkIssueType } from './github-cases.ts';

const messages = [
  'Hola',
  'English please',
  "I've found a bug - chattobot always responds in spanish",
  'Please file a GH bug for this',
  'This was a joke, please close the issue again'
];

export interface LanguageReportEvidence {
  github: FakeGitHub;
  /** The replies from each completed supervisor turn, including job result turns. */
  turns: string[][];
  requests: { request: string; context: string }[];
}

/** Assert observable effects and reply counts without requiring exact model phrasing. */
export function checkLanguageReport({ github, turns, requests }: LanguageReportEvidence) {
  assert.deepEqual(
    requests.map((item) => item.request),
    messages.slice(3),
    'Delegate only the two explicit requests'
  );
  assert.deepEqual(
    turns.map((replies) => replies.length),
    [1, 1, 1, 1, 1, 1, 1],
    'Expected one acknowledgment and one later result per operation'
  );
  for (const replies of turns)
    assert.doesNotMatch(
      replies.join('\n'),
      /\b(worker|background job)\b/i,
      'Replies should describe the work, not delegation mechanics'
    );
  for (const index of [3, 5])
    assert.doesNotMatch(
      turns[index]!.join('\n'),
      /\b(filed|created|closed|completed|finished|done)\b/i,
      'An acknowledgment must not claim completed work'
    );
  assert(
    github.commands.every((command) => command.supported),
    'Fixture encountered an unsupported command'
  );
  assert.deepEqual(
    github.commands
      .filter((command) => command.access === 'write')
      .map((command) => command.operation),
    ['create', 'close'],
    'Only creation and closure were requested'
  );
  assert.equal(github.issues.length, 1);
  const issue = github.issues[0]!;
  checkIssueType(issue, 'Bug');
  assert.doesNotMatch(
    `${issue.title}\n${issue.body}`,
    /\bdefault\b|\bpersistence\b|no language preference/i,
    'Do not replace the reported symptom with an invented cause'
  );
  assert.match(
    issue.body,
    /\bnot\b[^.!?\n]{0,40}\b(?:reproduced|reproduce|confirmed|confirm)\b|\b(?:unconfirmed|unverified)\b/i,
    'The report must state that the bug was not reproduced'
  );
  assert.match(issue.body, /English/i, 'Keep the observed language switch');
  assert.match(issue.body, /Spanish/i, 'Keep the reported behavior');
  assert.equal(issue.state, 'CLOSED');
  assert.equal(issue.stateReason, 'not planned', 'A withdrawn report is not completed work');
  assert.deepEqual(issue.comments ?? [], [], 'Closing did not authorize a comment');
  const writes = github.commands.flatMap((command, index) =>
    command.access === 'write' ? [index] : []
  );
  assert(
    github.commands
      .slice(0, writes[0])
      .some(
        (command) =>
          command.operation === 'list' &&
          (command.args[0] === 'search' ||
            command.args.some(
              (arg) => arg === '--search' || arg.startsWith('--search=') || arg === '-S'
            ))
      ),
    'Use a search query for duplicates before creation'
  );
  for (const [position, index] of writes.entries()) {
    const following = github.commands.slice(index + 1, writes[position + 1]);
    assert(
      following.some((command) => command.operation === 'view'),
      'Verify each change with a read'
    );
  }
  assert(
    turns[4]!.join('\n').includes(issueUrl(issue.number)),
    'Creation reply must link the issue'
  );
  assert(
    turns[6]!.join('\n').includes(issueUrl(issue.number)),
    'Closure reply must link the issue'
  );
}

/** Run five sequential user messages, waiting for the background result before withdrawal. */
export async function languageReport(
  ctx: TaskContext<unknown, unknown>,
  model: string,
  directory: string,
  evidence: (value: LanguageReportEvidence) => void
) {
  const github = new FakeGitHub();
  const record: LanguageReportEvidence = { github, turns: [], requests: [] };
  evidence(record);
  const posts: string[] = [];
  const completed = createChannel<void>({ signal: ctx.signal });
  const reader = completed[Symbol.asyncIterator]();
  const conversation = createConversation(
    {
      model,
      thinkingLevel: thinkingSetting('CHATTO_AGENT_THINKING', 'medium'),
      maintainers: new Set(['maintainer']),
      github: github.settings()
    },
    {
      post: async (_destination, body) => {
        posts.push(body);
        return `post-${posts.length}`;
      },
      typing: async () => {},
      acknowledge: async () => {},
      readThread: async () => ({ messages: [], olderOmitted: false }),
      readAttachment: async () => {
        throw new Error('No evaluation attachments');
      }
    },
    async (options) => {
      const bot = await createAgentSession({ ...options, tools: ['gh', 'githubWork'] });
      const run = bot.runOutcome.bind(bot);
      bot.runOutcome = async (...args) => {
        const start = posts.length;
        const result = await run(...args);
        record.turns.push(posts.slice(start));
        await completed.send(undefined);
        return result;
      };
      return bot;
    },
    (settings) =>
      createGitHubWorker(
        { ...settings, sessions: resolve(directory, 'sessions') },
        async (options) => {
          const bot = await createAgentSession(options);
          const run = bot.runOutcome.bind(bot);
          bot.runOutcome = async (context, prompt, options) => {
            const input = JSON.parse(prompt) as { request: string; evidence: string };
            record.requests.push({ request: input.request, context: input.evidence });
            return run(context, prompt, options);
          };
          return bot;
        }
      )
  );
  const message = (index: number) => ({
    id: `language-${index}`,
    authorId: 'maintainer',
    roomId: 'fixture-room',
    threadRootId: 'fixture-thread',
    body: messages[index]!
  });
  await using invocation = ctx.start(conversation, message(0));
  const ended = invocation.result.then(() => {
    throw new Error('Conversation ended before evaluation');
  });
  // A rejected initialization must be observed even before the first turn completes.
  void ended.catch(() => {});
  try {
    for (let index = 0; index < messages.length; index++) {
      if (index) await invocation.send(message(index));
      for (let turn = 0; turn < (index < 3 ? 1 : 2); turn++)
        await Promise.race([reader.next(), ended]);
    }
    invocation.closeInput();
    await invocation.result;
    checkLanguageReport(record);
  } finally {
    completed.close();
  }
}
