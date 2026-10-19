import { describe, expect, test } from 'vitest';
import { Type } from 'typebox';
import { createChannel } from './channel.ts';
import { runWorkflow } from './runner.ts';
import { task } from './task.ts';
import { log } from './log.ts';
import type { RunlingEvent } from './events.ts';

describe('execution services', () => {
  test('isolates concurrent host messages, logs, and verbosity', async () => {
    const events: RunlingEvent[][] = [[], []];
    const answer = task(
      { name: 'Answer', input: Type.String(), result: Type.String(), steering: Type.String() },
      async (ctx) => {
        for await (const answer of ctx.steering) {
          log.debug(`debug:${answer}`);
          log.info(answer);
          return answer;
        }
        throw new Error('No answer');
      }
    );
    const first = createChannel<string>();
    const second = createChannel<string>();
    const one = runWorkflow(answer, {
      input: 'one',
      verbose: true,
      messages: first,
      onEvent: (e) => events[0]!.push(e)
    });
    const two = runWorkflow(answer, {
      input: 'two',
      verbose: false,
      messages: second,
      onEvent: (e) => events[1]!.push(e)
    });
    await second.send('second answer');
    await first.send('first answer');
    expect((await one).output).toBe('first answer');
    expect((await two).output).toBe('second answer');
    const messages = (list: RunlingEvent[]) =>
      list.flatMap((e) => (e.type === 'log' && e.source === undefined ? [e.message] : []));
    expect(messages(events[0]!)).toEqual(['debug:first answer', 'first answer']);
    expect(messages(events[1]!)).toEqual(['second answer']);
  });
});
