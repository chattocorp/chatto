/** Delegate a current maintainer message to the GitHub worker as a background job. Identity
 * comes from host-owned messages, not from the supervisor's arguments or thread text. */
import { defineAgentExtension, log, Type, type TaskContext } from 'runling';
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
        // Only a host-owned message ID enters the log; unknown model input can contain secrets.
        const deny = (reason: string, error: string): never => {
          log.info(
            `GitHub authorization denied: request=${request?.id ?? 'not-current'}; reason=${reason}`
          );
          throw new Error(error);
        };
        if (request?.trigger === 'intake')
          return deny(
            'not-addressed',
            'GitHub work requires a maintainer message addressed to the bot.'
          );
        if (!request || !maintainers.has(request.authorId))
          return deny(
            request ? 'not-maintainer' : 'no-current-message',
            'GitHub work requires a message from a maintainer in this turn.'
          );
        ctx.signal.throwIfAborted();
        if (started.has(requestId))
          deny('already-started', 'GitHub work already started for this message.');
        if (jobs.list().some((job) => job.kind === 'github'))
          deny('worker-running', 'A GitHub worker is still running. Wait for its result.');
        started.add(requestId);
        log.info(`GitHub authorization allowed: request=${request.id}; reason=current-maintainer`);
        const job = jobs.start(
          { kind: 'github', label: 'GitHub work', replyTo: replyTo(request) },
          () => worker(ctx, { request: request.body, context })
        );
        return {
          content: [
            {
              type: 'text',
              text: `GitHub worker ${job.id} started. The requested change has not been confirmed. Its result reaches you in a later turn. Acknowledge once with ongoing language such as "I'm filing the report", never "I've filed it". Do not claim completion or invent an issue link. Omit worker or job details, then end this turn.`
            }
          ],
          details: {}
        };
      }
    });
  });
}
