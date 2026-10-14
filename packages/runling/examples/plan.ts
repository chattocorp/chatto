import { agent, concat, task, Type } from 'runling';

const model = 'openai-codex/gpt-5.6-sol';
const thinkingLevel = 'medium';

/** Interview the human through workflow messages: each question is an update, and the next
 * message from the host is the answer. Run it with `runling run`, then type the answers. */
export const plan = task(
  {
    name: 'Plan change',
    input: Type.Object({
      directory: Type.String({ minLength: 1 }),
      prompt: Type.String({ minLength: 1, description: 'The change to plan' })
    }),
    output: Type.String({ description: 'The implementation plan in Markdown' }),
    receives: Type.String({ description: 'An answer to the latest question' }),
    emits: Type.String({ description: 'A question for the human' })
  },
  async (ctx, { directory, prompt: request }) => {
    const answers = ctx.inbox[Symbol.asyncIterator]();

    await using planner = await agent({
      cwd: directory,
      model,
      thinkingLevel,
      tools: ['read', 'grep', 'find', 'ls'],
      instructions: [
        'Inspect the repository without modifying it.',
        'Interview the human before finalizing the implementation plan.',
        'Ask one question at a time, only when its answer would materially change the implementation.',
        'Do not ask questions whose answers can be discovered from the repository.',
        'To ask a question, report a blocked outcome and put the complete concise question in the report summary.',
        'When the request is sufficiently specified, report a completed outcome with a concise summary and a detailed Markdown implementation plan.',
        'The plan should capture the goal, relevant repository context, decisions made, implementation steps, validation, and material risks.'
      ]
    });

    let message = concat(
      'Investigate this request, then begin the planning interview.',
      'Ask a question rather than making an arbitrary choice whenever the answer would materially affect the result.',
      '',
      'Request:',
      request
    );

    while (true) {
      const report = await planner.runOutcome(ctx, message);

      if (report.outcome === 'completed') return report.details ?? report.summary;
      if (report.outcome === 'failed') throw new Error(`Planning failed: ${report.summary}`);

      await ctx.emit(report.summary);
      const answer = await answers.next();
      if (answer.done) throw new Error('The planning interview ended without an answer');
      message = concat(
        'The human answered your question:',
        answer.value,
        '',
        'Continue the interview. Ask the next material question, or complete the plan if the request is now sufficiently specified.'
      );
    }
  }
);

export default plan;
