/** Delegate a current maintainer message to the GitHub worker as a background job. Identity
 * comes from host-owned messages, not from the supervisor's arguments or thread text. */
import { defineAgentExtension, Type, type TaskContext } from 'runling';
import { replyTo, type IncomingMessage } from '../chatto/message.ts';
import type { Jobs } from '../harness/jobs.ts';
import type { createGitHubWorker } from '../tasks/github.ts';

export interface GitHubWorkOptions {
  jobs: Jobs;
  maintainers: ReadonlySet<string>;
  /** Only addressed messages in the current turn can start work. */
  messages: () => readonly IncomingMessage[];
}

/** One worker per message per conversation, and one active GitHub worker per thread. */
export function githubWorkExtension(
  ctx: TaskContext<unknown, unknown>,
  worker: ReturnType<typeof createGitHubWorker>,
  { jobs, maintainers, messages }: GitHubWorkOptions
) {
  const started = new Set<string>();
  return defineAgentExtension((pi) => {
    pi.registerTool({
      name: 'githubWork',
      label: 'Delegate GitHub work',
      description:
        'Start a GitHub worker for a maintainer request in this turn. It can create and update issues using gh. Give the request message ID and self-contained context. Never invoke it for a request from a non-maintainer. It runs in the background and its result reaches you in a later turn.',
      parameters: Type.Object({
        requestId: Type.String({ description: 'The message ID of the maintainer request' }),
        context: Type.String({ maxLength: 20_000 })
      }),
      async execute(_id, { requestId, context }) {
        const request = messages().find((message) => message.id === requestId);
        if (!request || !maintainers.has(request.authorId))
          throw new Error('GitHub work requires a message from a maintainer in this turn.');
        ctx.signal.throwIfAborted();
        if (started.has(requestId))
          throw new Error('GitHub work already started for this message.');
        if (jobs.list().some((job) => job.kind === 'github'))
          throw new Error('A GitHub worker is still running. Wait for its result.');
        started.add(requestId);
        const job = jobs.start(
          { kind: 'github', label: 'GitHub work', replyTo: replyTo(request) },
          () => worker(ctx, { request: request.body, context })
        );
        return {
          content: [
            {
              type: 'text',
              text: `GitHub worker ${job.id} started. Its result reaches you in a later turn. Tell the maintainer briefly, then keep answering messages.`
            }
          ],
          details: {}
        };
      }
    });
  });
}
