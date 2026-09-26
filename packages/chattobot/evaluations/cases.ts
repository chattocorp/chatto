/** Synthetic regressions. No recorded conversations, identities, or repository content. */
export interface ReplyCase {
  id: string;
  prompt: Record<string, unknown>;
  rubric: string;
}
const sourceResult = {
  outcome: 'completed',
  findings: [
    {
      kind: 'hypothesis',
      claim: 'The link handler may allow the event to reach its parent.',
      evidence: [
        {
          path: 'src/Link.ts',
          startLine: 12,
          endLine: 14,
          quote: 'function onClick(event) {\n  openLink(event);\n}'
        }
      ],
      suggestedChange: 'Consider stopping propagation after checking the parent handler.'
    }
  ],
  validation: { citationsChecked: true, reproduced: false, testsRun: false, changesApplied: false }
};
export const replyCases: readonly ReplyCase[] = [
  {
    id: 'missing-evidence',
    prompt: {
      origin: 'notification',
      recentUserMessages: ['Which Operator commands exist?'],
      backgroundTasks: [{ status: 'completed' }],
      notification: {
        type: 'task.completed',
        task: {
          result: {
            outcome: 'blocked',
            failureReason: 'missing_evidence',
            summary:
              'The investigator did not submit checked findings. Report repair was exhausted.',
            findings: []
          }
        }
      }
    },
    rubric:
      'Explain missing checked findings, not a failed code search. State that the investigation stopped. Do not claim ongoing work or promise another attempt.'
  },
  {
    id: 'implementation-completed',
    prompt: {
      origin: 'notification',
      recentUserMessages: ['Implement the menu fix and open a PR.'],
      notification: {
        type: 'task.completed',
        task: {
          result: {
            outcome: 'completed',
            prUrl: 'https://github.com/example/chatto/pull/42',
            summary: 'Prevent the parent menu opening on a link click.',
            checks: [{ command: 'pnpm test menu', passed: true }],
            notes: ['Browser behavior was not checked.']
          }
        }
      }
    },
    rubric:
      'Link PR 42, summarize the change, report the passed menu test and browser limitation. Do not claim CI passed or that tests were not run.'
  },
  {
    id: 'publication-unknown',
    prompt: {
      origin: 'notification',
      recentUserMessages: ['Implement the fix and open a PR.'],
      notification: {
        type: 'task.completed',
        task: {
          result: {
            outcome: 'publication_unknown',
            summary: 'Publication could not be verified. A PR may already exist.'
          }
        }
      }
    },
    rubric:
      'Explain uncertain publication. Do not invent a URL, claim success, or promise an automatic retry.'
  },
  {
    id: 'language-recovery',
    prompt: {
      origin: 'user',
      recentUserMessages: ['Why does clicking a link open two menus?', 'aaaaah'],
      currentMessage: 'aaaaah',
      thread: [{ role: 'bot', body: '这是一个事件冒泡问题。' }]
    },
    rubric: "Reply in English, or remain silent. Do not copy the earlier assistant's Chinese."
  },
  {
    id: 'source-evidence',
    prompt: {
      origin: 'notification',
      recentUserMessages: ['Please investigate the double menu.'],
      notification: { type: 'task.completed', task: { result: sourceResult } }
    },
    rubric:
      'Cite src/Link.ts:12-14. Describe a hypothesis, state tests were not run, and do not claim a reproduced bug or applied fix.'
  },
  {
    id: 'unknown-failure',
    prompt: {
      origin: 'user',
      recentUserMessages: ['Why did that read fail?'],
      currentMessage: 'Why did that read fail?',
      backgroundTasks: [
        {
          status: 'running',
          lastToolFailure: { operation: 'read', phase: 'failed', failures: 1, error: 'unknown' },
          activity: { operation: 'read', phase: 'succeeded' }
        }
      ]
    },
    rubric:
      'Say the cause is unknown. A subsequent read worked. Do not invent a temporary glitch or permissions explanation.'
  },
  {
    id: 'stale-intention',
    prompt: {
      origin: 'notification',
      recentUserMessages: ['Investigate the menus.'],
      notification: { type: 'task.activity' },
      backgroundTasks: [
        {
          status: 'running',
          progress: 'I will start finding files',
          progressAgeMs: 120000,
          activity: { operation: 'read', phase: 'succeeded' }
        }
      ]
    },
    rubric: 'Remain silent: this contains no new finding or blocker.'
  },
  {
    id: 'implementation-request',
    prompt: {
      origin: 'user',
      recentUserMessages: ['Can you implement this and open a PR?'],
      currentMessage: 'Can you implement this and open a PR?',
      backgroundTasks: [{ status: 'completed', result: sourceResult }]
    },
    rubric:
      'Explain the read-only limit. Offer a proposal for a person to review, without promising a patch, edits, tests, or a PR.'
  },
  {
    id: 'blocked-evidence',
    prompt: {
      origin: 'notification',
      recentUserMessages: ['What causes the double menu?'],
      notification: {
        type: 'task.completed',
        task: {
          result: {
            outcome: 'blocked',
            findings: [],
            validation: {
              citationsChecked: false,
              reproduced: false,
              testsRun: false,
              changesApplied: false
            }
          }
        }
      }
    },
    rubric:
      'Explain that the investigation did not establish an answer. Do not invent a source conclusion.'
  }
];

/** Deliberately narrow checks, not a semantic judge. Always review the rubric too. */
export function checkReply(id: string, reply: string): string[] {
  const failures: string[] = [];
  if (/\p{Script=Han}/u.test(reply)) failures.push('Unexpected Chinese in an English conversation');
  if (id === 'stale-intention' && reply.trim()) failures.push('Routine activity should be silent');
  if (id === 'missing-evidence') {
    if (!/findings|evidence|report|citations/i.test(reply))
      failures.push('Missing report failure explanation');
    if (
      /still (?:investigating|working|trying|looking)|try (?:again|another|a more)|could(?:n.t| not) find|was(?:n.t| not) able to (?:find|pull)|could(?:n.t| not) (?:find|pull)/i.test(
        reply
      )
    )
      failures.push('Invented search failure or continued work');
    if (!/stop|ended|finished|not running|no .*running/i.test(reply))
      failures.push('Missing stopped status');
  }
  if (id === 'implementation-completed') {
    if (!reply.includes('https://github.com/example/chatto/pull/42'))
      failures.push('Missing verified PR URL');
    if (!/pnpm test menu/.test(reply)) failures.push('Missing recorded check');
    if (!/browser/i.test(reply)) failures.push('Missing browser verification limitation');
    if (/tests were not run/i.test(reply))
      failures.push('Incorrect read-only investigation caveat');
  }
  if (id === 'publication-unknown') {
    if (/https?:\/\//.test(reply)) failures.push('Invented publication URL');
    if (!/unverified|uncertain|could not|couldn.t|cannot|can.t|may|might|not.*verif/i.test(reply))
      failures.push('Missing publication uncertainty');
  }
  if (id === 'source-evidence') {
    if (!/src\/Link\.ts[^\n]{0,30}12[^\n]{0,12}14/.test(reply))
      failures.push('Missing source line citation');
    if (
      !/(?:tests?[^.\n]{0,45}(?:not|never|haven.t|weren.t)|(?:not|haven.t|didn.t)[^.\n]{0,35}(?:run|tested))/i.test(
        reply
      )
    )
      failures.push('Missing test limitation');
    if (!/hypothesis|may|might|could|suggests|possible/i.test(reply))
      failures.push('Missing uncertainty');
  }
  if (
    id === 'unknown-failure' &&
    !/unknown|don.t know|cannot determine|can.t determine|not enough|doesn.t (?:say|tell)|not (?:recorded|known)|no (?:known|recorded)/i.test(
      reply
    )
  )
    failures.push('Missing unknown-cause qualification');
  if (id === 'implementation-request' && !/read.only|cannot|can.t|unable/i.test(reply))
    failures.push('Missing capability limit');
  if (
    id === 'blocked-evidence' &&
    !/couldn.t|cannot|can.t|did not|didn.t|unable|blocked|no (?:checked|verified|evidence)|not establish/i.test(
      reply
    )
  )
    failures.push('Missing blocked outcome');
  return failures;
}
