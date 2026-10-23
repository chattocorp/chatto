import { runAgent, step, concat, task, Type } from 'runling';

const model = 'openai-codex/gpt-5.6-sol';

export const ask = task(
  {
    name: 'Answer repository question',
    input: Type.Object({
      directory: Type.String({ minLength: 1 }),
      prompt: Type.String({ minLength: 1, description: 'A question about the repository' })
    }),
    result: Type.String({ description: 'The answer in Markdown' })
  },
  async (ctx, { directory, prompt: question }) => {
    const report = await step('Investigating repository', () =>
      runAgent(
        ctx,
        concat(
          'Answer this question about the repository:',
          question,
          '',
          'Inspect the repository before answering. Ground the answer in the current files rather than assumptions.'
        ),
        {
          cwd: directory,
          model,
          thinkingLevel: 'medium',
          tools: ['read', 'grep', 'find', 'ls'],
          instructions: [
            'Inspect the repository without modifying it.',
            'Answer only from evidence available in the repository.',
            'Put a concise direct answer in the report summary.',
            'Put supporting explanation in the report details, citing relevant file paths and line numbers.',
            'State clearly when the repository does not contain enough information to answer part of the question.'
          ]
        }
      )
    );

    return report.details ? `${report.summary}\n\n${report.details}` : report.summary;
  }
);

export default ask;
