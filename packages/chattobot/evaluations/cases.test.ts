import { expect, test } from 'vitest';
import { checkReply, replyCases } from './cases.ts';

test('cases have unique IDs and concrete review rubrics', () => {
  expect(new Set(replyCases.map((example) => example.id)).size).toBe(replyCases.length);
  expect(replyCases.every((example) => example.rubric.length > 30)).toBe(true);
});
test('detects the observed language, citation, uncertainty, and stale-update regressions', () => {
  expect(checkReply('language-recovery', '这是原因')).toHaveLength(1);
  expect(checkReply('source-evidence', 'I found it and fixed it.')).toHaveLength(3);
  expect(
    checkReply(
      'source-evidence',
      'The handler may allow bubbling (src/Link.ts:12-14). Tests were not run.'
    )
  ).toEqual([]);
  expect(checkReply('stale-intention', "I'm still investigating.")).toHaveLength(1);
  expect(checkReply('stale-intention', '')).toEqual([]);
  expect(checkReply('unknown-failure', 'It was a temporary glitch.')).toHaveLength(1);
  expect(checkReply('unknown-failure', 'The cause is unknown. A later read succeeded.')).toEqual(
    []
  );
  expect(checkReply('implementation-request', 'I will make a patch.')).toHaveLength(1);
  expect(checkReply('blocked-evidence', 'The bug is confirmed.')).toHaveLength(1);
});

test('implementation replies need the verified PR link, actual checks, and limitations', () => {
  expect(checkReply('implementation-completed', 'Done! Tests were not run.')).toHaveLength(4);
  expect(
    checkReply(
      'implementation-completed',
      '[PR](https://github.com/example/chatto/pull/42) fixes the menu. pnpm test menu passed. Browser behavior was not checked.'
    )
  ).toEqual([]);
  expect(
    checkReply('publication-unknown', 'Done: https://github.com/example/chatto/pull/99')
  ).toHaveLength(2);
  expect(
    checkReply('publication-unknown', 'Publication could not be verified; a PR may already exist.')
  ).toEqual([]);
});

test('missing evidence is a stopped report handoff, not an ongoing search', () => {
  expect(
    checkReply(
      'missing-evidence',
      'The worker did not submit checked findings. The investigation has stopped.'
    )
  ).toEqual([]);
  expect(
    checkReply(
      'missing-evidence',
      "I couldn't find the code. I'm still working and will try again."
    )
  ).not.toEqual([]);
});
