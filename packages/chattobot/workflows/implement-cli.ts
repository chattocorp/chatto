/** Entry point for running the implementation task with `runling run`, outside a Chatto
 * conversation. Settings come from the same environment variables as the bot. Runs started here
 * have no owner key, so the bot cannot resume their artifacts, and they cannot resume the bot's. */
import { task, Type } from 'runling';
import { ConfigurationError } from '../settings.ts';
import { implementationInput } from './implementation-artifacts.ts';
import { createImplementation, implementationSettings } from './implement.ts';

const settings = implementationSettings();
if (!settings)
  throw new ConfigurationError(
    'Set CHATTO_IMPLEMENTATION_REPOSITORY and CHATTO_SOURCE_DIRECTORY to run implementations'
  );
// With no parent task, `runling run` writes the task's updates, such as progress notices, to the
// log.
const implement = createImplementation(settings);

/** Run one implementation and state its outcome, PR, and CI result at the end of the log. */
export default task(
  {
    name: 'ChattoBot implementation',
    input: implementationInput,
    output: Type.Object({
      summary: Type.String(),
      details: Type.String(),
      outputs: Type.Record(Type.String(), Type.Unknown())
    })
  },
  async (ctx, input) => {
    const result = await implement(ctx, input);
    const ci = 'ci' in result ? result.ci : undefined;
    const failedChecks = result.checks.filter((check) => !check.passed);
    const details = [
      `- **Outcome:** ${result.outcome}`,
      ...(result.prUrl ? [`- **Pull request:** ${result.prUrl}`] : []),
      ...(ci
        ? [
            `- **CI:** ${ci.status}; ${ci.passed} passed, ${ci.failed} failed, ${ci.pending} pending; ${ci.repairs} repair attempts${ci.reason ? `. ${ci.reason}` : ''}`
          ]
        : []),
      ...(failedChecks.length
        ? [`- **Failed host checks:** ${failedChecks.map((check) => check.command).join(', ')}`]
        : []),
      ...(result.artifactId ? [`- **Artifact:** ${result.artifactId}`] : []),
      '',
      result.summary,
      ...(result.notes.length ? ['', ...result.notes.map((note) => `- ${note}`)] : [])
    ].join('\n');
    return {
      summary:
        result.outcome === 'completed'
          ? `Implementation completed: ${result.prUrl}`
          : `Implementation ${result.outcome}`,
      details,
      outputs: JSON.parse(JSON.stringify(result))
    };
  }
);
