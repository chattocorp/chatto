import { agent } from 'runling';

/** A directly runnable agent task: `runling run examples/agent.ts "Explain this repository"`. */
export default agent({
  name: 'Repository assistant',
  cwd: '.',
  model: 'openai-codex/gpt-5.6-sol',
  tools: ['read', 'grep', 'find', 'ls'],
  instructions: ['Answer from evidence in the repository. Cite the files that support your answer.']
});
