import { describe, it, expect } from 'vitest';
import { flushSync } from 'svelte';
import { render } from 'vitest-browser-svelte';
import PermissionExplanationTable from './PermissionExplanationTable.svelte';
import { q } from '$lib/test-utils';

type DecisionKind = 'ALLOW' | 'DENY' | 'NONE';
type Level = 'INSTANCE' | 'SPACE' | 'ROOM';

type Explanation = {
  permission: string;
  state: DecisionKind;
  decidedAt?: Level | null;
  decidedByRole?: string | null;
  trace: { level: Level; roleName: string; decision: DecisionKind; applied: boolean }[];
};

function granted(roleName: string, level: Level): Explanation {
  return {
    permission: 'message.post',
    state: 'ALLOW',
    decidedAt: level,
    decidedByRole: roleName,
    trace: [{ level, roleName, decision: 'ALLOW', applied: true }]
  };
}

function denied(roleName: string, level: Level): Explanation {
  return {
    permission: 'message.post',
    state: 'DENY',
    decidedAt: level,
    decidedByRole: roleName,
    trace: [{ level, roleName, decision: 'DENY', applied: true }]
  };
}

function none(): Explanation {
  return {
    permission: 'message.post',
    state: 'NONE',
    decidedAt: null,
    decidedByRole: null,
    trace: []
  };
}

describe('PermissionExplanationTable', () => {
  it('renders a header row', async () => {
    const { container } = render(PermissionExplanationTable, { props: { explanations: [] } });
    await expect.element(q(container, 'div')).toBeInTheDocument();
    expect(container.textContent).toContain('Permission');
    expect(container.textContent).toContain('State');
    expect(container.textContent).toContain('Decided by');
  });

  it('renders the granting role and level when state is ALLOW', () => {
    const { container } = render(PermissionExplanationTable, {
      props: { explanations: [granted('admin', 'SPACE')] }
    });
    expect(container.textContent).toContain('Space');
    expect(container.textContent).toContain('admin');
  });

  it('renders "no role decided" when state is NONE', () => {
    const { container } = render(PermissionExplanationTable, {
      props: { explanations: [none()] }
    });
    expect(container.textContent).toContain('No role decided');
  });

  it('shows the deny role when state is DENY', () => {
    const { container } = render(PermissionExplanationTable, {
      props: { explanations: [denied('moderator', 'SPACE')] }
    });
    expect(container.textContent).toContain('moderator');
  });

  it('expands the trace when the toggle is clicked', async () => {
    const exp: Explanation = {
      permission: 'message.post',
      state: 'ALLOW',
      decidedAt: 'INSTANCE',
      decidedByRole: 'instance-admin',
      trace: [
        { level: 'INSTANCE', roleName: 'instance-admin', decision: 'ALLOW', applied: true },
        { level: 'SPACE', roleName: 'everyone', decision: 'DENY', applied: false }
      ]
    };
    const { container } = render(PermissionExplanationTable, {
      props: { explanations: [exp] }
    });

    expect(container.textContent).not.toContain('Resolution trace');
    const toggle = q(container, 'button[aria-expanded="false"]') as HTMLButtonElement | null;
    if (!toggle) throw new Error('toggle button not rendered');
    toggle.click();
    flushSync();
    expect(container.querySelector('button[aria-expanded="true"]')).toBeTruthy();
    expect(container.textContent).toContain('Resolution trace');
    expect(container.textContent).toContain('winning decision');
    expect(container.textContent).toContain('everyone');
  });
});
