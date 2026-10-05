import { expect, it } from 'vitest';
import { effect } from '@chatto/client/reactivity';
import { AppState } from './appLifecycle';

type Transitions = { markBackgrounded(): void; activateFromInteraction(): void };

it('changes lifecycle fields together', () => {
  const state = new AppState();
  const transitions = state as unknown as Transitions;
  transitions.markBackgrounded();
  const revision = state.foregroundRevision;
  const seen: string[] = [];
  const stop = effect(() => {
    seen.push(`${state.isFocused}/${state.isVisible}/${state.foregroundRevision - revision}`);
  });
  transitions.activateFromInteraction();
  stop();
  expect(seen).toEqual(['false/false/0', 'true/true/1']);
});
