import '../../../app.css';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { flushSync } from 'svelte';
import BotPermissionsMatrix from './BotPermissionsMatrix.svelte';

const mocks = vi.hoisted(() => ({
  getPermissionMatrix: vi.fn(),
  setPermission: vi.fn()
}));

vi.mock('$lib/state/server/connection.svelte', () => ({
  useConnection: () => () => ({ connectBaseUrl: '/api/connect', bearerToken: 'token' })
}));

vi.mock('$lib/api-client/bots', () => ({
  createBotAPI: () => ({
    getPermissionMatrix: mocks.getPermissionMatrix,
    setPermission: mocks.setPermission
  })
}));

const deniedByOwnerMatrix = {
  botId: 'bot-1',
  applicablePermissions: ['room.manage'],
  scopes: [{ id: 'server', label: 'Server', kind: 'SERVER' as const, parentGroupId: '' }],
  cells: [
    {
      permission: 'room.manage',
      scopeId: 'server',
      directDecision: 'NONE' as const,
      effectiveDecision: 'DENY' as const,
      ownerAllowed: false
    }
  ]
};

async function settle() {
  await Promise.resolve();
  await Promise.resolve();
  await Promise.resolve();
  flushSync();
}

function permissionButton(container: HTMLElement): HTMLButtonElement {
  const button = container.querySelector<HTMLButtonElement>(
    'td[data-permission="room.manage"] button'
  );
  if (!button) throw new Error('room.manage permission button not found');
  return button;
}

describe('BotPermissionsMatrix', () => {
  beforeEach(() => {
    mocks.getPermissionMatrix.mockReset();
    mocks.getPermissionMatrix.mockResolvedValue(structuredClone(deniedByOwnerMatrix));
    mocks.setPermission.mockReset();
    mocks.setPermission.mockResolvedValue(undefined);
  });

  it('never sends an allow when the owner is denied', async () => {
    const { container } = render(BotPermissionsMatrix, { props: { botId: 'bot-1' } });
    await settle();

    permissionButton(container).click();
    await settle();

    expect(mocks.setPermission).toHaveBeenCalledOnce();
    expect(mocks.setPermission).toHaveBeenCalledWith(
      expect.objectContaining({ permission: 'room.manage', decision: 'DENY' })
    );
    expect(mocks.setPermission).not.toHaveBeenCalledWith(
      expect.objectContaining({ decision: 'ALLOW' })
    );
  });

  it('never sends an allow over an inherited deny', async () => {
    const inheritedDeny = structuredClone(deniedByOwnerMatrix);
    inheritedDeny.cells[0].ownerAllowed = true;
    mocks.getPermissionMatrix.mockResolvedValue(inheritedDeny);
    const { container } = render(BotPermissionsMatrix, { props: { botId: 'bot-1' } });
    await settle();

    permissionButton(container).click();
    await settle();

    expect(mocks.setPermission).toHaveBeenCalledWith(
      expect.objectContaining({ permission: 'room.manage', decision: 'DENY' })
    );
    expect(mocks.setPermission).not.toHaveBeenCalledWith(
      expect.objectContaining({ decision: 'ALLOW' })
    );
  });

  it('renders at full height without an internal vertical viewport', async () => {
    const { container } = render(BotPermissionsMatrix, { props: { botId: 'bot-1' } });
    await settle();

    const tableViewport = container.querySelector('table')?.parentElement;
    expect(tableViewport?.className).toContain('overflow-x-auto');
    expect(tableViewport?.className).not.toContain('overflow-y-auto');
  });

  it('clears an in-flight cell state when navigating to another bot', async () => {
    let resolveMutation: (() => void) | undefined;
    mocks.setPermission.mockReturnValue(
      new Promise<void>((resolve) => {
        resolveMutation = resolve;
      })
    );
    const { container, rerender } = render(BotPermissionsMatrix, {
      props: { botId: 'bot-1' }
    });
    await settle();

    permissionButton(container).click();
    await settle();
    expect(permissionButton(container).getAttribute('aria-busy')).toBe('true');

    await rerender({ botId: 'bot-2' });
    await settle();
    expect(permissionButton(container).getAttribute('aria-busy')).toBeNull();

    resolveMutation?.();
  });
});
