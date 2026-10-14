/** Put a tool behind human approval. A call starts a background job that posts the exact action,
 * waits for an approver's decision, and runs the tool only when it is approved. The call returns
 * at once; the outcome reaches the agent in a later turn. */
import type { Destination } from '@chatto/client';
import type { AgentTool } from 'runling';
import type { Approvals } from './approvals.ts';
import type { Jobs } from './jobs.ts';

/** A tool that needs approval. `describe` renders the exact action for the approvers. */
export type ApprovableTool = AgentTool & { describe(input: any): string };

export interface ApprovalToolOptions {
  jobs: Jobs;
  approvals: Approvals;
  /** Where the request goes: the thread of the current turn. */
  replyTo: () => Destination;
  /** The conversation's signal. The job outlives the tool call, not the conversation. */
  signal: AbortSignal;
}

export function requireApproval(
  { describe, ...tool }: ApprovableTool,
  { jobs, approvals, replyTo, signal }: ApprovalToolOptions
): AgentTool {
  return {
    ...tool,
    description: `${tool.description} A maintainer must approve each call: the host posts the exact action, and the outcome reaches you in a later turn.`,
    async execute(toolCallId, input, _signal, _onUpdate, ctx) {
      const destination = replyTo();
      const job = jobs.start(
        { kind: 'approval', label: `${tool.label}, waiting for approval`, replyTo: destination },
        async (report) => {
          const decision = await approvals.request(describe(input), destination, signal);
          if (decision === 'declined') return `${tool.label}: not run, because it was declined.`;
          if (decision === 'expired')
            return `${tool.label}: not run, because nobody approved it in time.`;
          report('Approved');
          const result = await tool.execute(toolCallId, input, signal, undefined, ctx);
          const text = result.content.flatMap((part) => (part.type === 'text' ? [part.text] : []));
          return `${tool.label}: approved and done. ${text.join('\n')}`;
        }
      );
      return {
        content: [
          {
            type: 'text',
            text: `Approval requested (job ${job.id}). The host posted the exact action for the maintainers. Tell the person briefly that it waits for a maintainer's approval; the outcome reaches you in a later turn.`
          }
        ],
        details: {}
      };
    }
  };
}
