import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { flushSync } from 'svelte';
import PermissionMatrix from './PermissionMatrix.svelte';

type TierRoles = {
  applicablePermissions: string[];
  roles: Array<{
    roleName: string;
    displayName: string;
    description: string;
    isSystem: boolean;
    position: number;
    override: { permissions: string[]; permissionDenials: string[] };
    inheritedAllows: string[];
    inheritedDenials: string[];
  }>;
};

const HAPPY_TIER_ROLES: TierRoles = {
  applicablePermissions: ['message.post', 'room.create'],
  roles: [
    {
      roleName: 'owner',
      displayName: 'Owner',
      description: '',
      isSystem: true,
      position: 1000,
      override: { permissions: [], permissionDenials: [] },
      inheritedAllows: [],
      inheritedDenials: []
    },
    {
      roleName: 'admin',
      displayName: 'Admin',
      description: '',
      isSystem: true,
      position: 1,
      override: { permissions: ['message.post'], permissionDenials: [] },
      inheritedAllows: [],
      inheritedDenials: []
    },
    {
      roleName: 'moderator',
      displayName: 'Moderator',
      description: '',
      isSystem: true,
      position: 2,
      override: { permissions: [], permissionDenials: ['room.create'] },
      inheritedAllows: ['message.post'],
      inheritedDenials: []
    }
  ]
};

// A module-level holder so individual tests can swap the resolver payload
// before rendering. The `useConnection` mock dereferences it on every call.
let nextTierRoles: TierRoles | null = HAPPY_TIER_ROLES;
const permissionMocks = vi.hoisted(() => ({
  getRolePermissionTierMatrix: vi.fn(),
  setRolePermission: vi.fn()
}));

vi.mock('@chatto/api-client/permissions', () => ({
  createPermissionAPI: vi.fn(() => ({
    getRolePermissionTierMatrix: permissionMocks.getRolePermissionTierMatrix,
    setRolePermission: permissionMocks.setRolePermission
  }))
}));

vi.mock('$lib/state/server/connection.svelte', () => ({
  useConnection: () => () => ({
    isConnected: true,
    showConnectionLostBanner: false,
    connectBaseUrl: '/api/connect',
    bearerToken: 'token'
  })
}));

beforeEach(() => {
  nextTierRoles = HAPPY_TIER_ROLES;
  permissionMocks.getRolePermissionTierMatrix.mockReset();
  permissionMocks.getRolePermissionTierMatrix.mockImplementation(async () => nextTierRoles);
  permissionMocks.setRolePermission.mockReset();
  permissionMocks.setRolePermission.mockResolvedValue(true);
});

async function settle() {
  // Resolve the mock query (1 microtask) then any chained then() inside the
  // matrix's load(); flushSync to commit Svelte state reads.
  await Promise.resolve();
  await Promise.resolve();
  await Promise.resolve();
  flushSync();
}

describe('PermissionMatrix', () => {
  it('renders one column per role and one row per permission', async () => {
    const { container } = render(PermissionMatrix, { props: { spaceId: 'space-1' } });
    await settle();

    const tables = container.querySelectorAll('table');
    expect(tables.length).toBeGreaterThan(0);
    // "Permission" + "@owner" + "@admin" + "@moderator" per category panel;
    // two categories ('message' and 'room'), so 8 header cells total.
    expect(container.querySelectorAll('thead th').length).toBe(8);
    expect(container.querySelectorAll('tbody tr').length).toBe(2);
  });

  it('reflects override + inherited state in cell aria-pressed', async () => {
    const { container } = render(PermissionMatrix, { props: { spaceId: 'space-1' } });
    await settle();

    // Admin / message.post: explicit override Allow → aria-pressed=true.
    const adminMessagePost = container.querySelector(
      'button[aria-label*="Admin"][aria-label*="message.post"]'
    );
    expect(adminMessagePost?.getAttribute('aria-pressed')).toBe('true');

    // Moderator / message.post: no override but inherited allow → aria-pressed=false,
    // visible icon is the check (allow).
    const modMessagePost = container.querySelector(
      'button[aria-label*="Moderator"][aria-label*="message.post"]'
    );
    expect(modMessagePost?.getAttribute('aria-pressed')).toBe('false');
    expect(modMessagePost?.querySelector('.uil--check')).not.toBeNull();
  });

  it('invokes onRoleClick when a column header is clicked', async () => {
    const onRoleClick = vi.fn();
    const { container } = render(PermissionMatrix, {
      props: { onRoleClick }
    });
    await settle();

    const buttons = Array.from(
      container.querySelectorAll('thead button')
    ) as HTMLButtonElement[];
    const adminHeader = buttons.find((b) => b.textContent?.trim() === '@admin');
    expect(adminHeader).toBeDefined();
    adminHeader!.click();
    flushSync();
    expect(onRoleClick).toHaveBeenCalledWith(
      expect.objectContaining({ roleName: 'admin' })
    );
  });

  it('renders headers as plain text when isRoleClickable returns false', async () => {
    const onRoleClick = vi.fn();
    const { container } = render(PermissionMatrix, {
      props: {
        onRoleClick,
        isRoleClickable: (role: { roleName: string }) => role.roleName !== 'admin'
      }
    });
    await settle();

    const headerCells = Array.from(container.querySelectorAll('thead th'));
    const adminTh = headerCells.find((th) => th.textContent?.includes('@admin')) as HTMLElement;
    const modTh = headerCells.find((th) => th.textContent?.includes('@moderator')) as HTMLElement;
    expect(adminTh.querySelector('button')).toBeNull();
    expect(modTh.querySelector('button')).not.toBeNull();
  });

  it('renders owner cells as read-only effective allows', async () => {
    const { container } = render(PermissionMatrix, { props: { spaceId: 'space-1' } });
    await settle();

    const ownerMessagePost = container.querySelector(
      'button[aria-label*="Owner"][aria-label*="message.post"]'
    ) as HTMLButtonElement | null;
    expect(ownerMessagePost).not.toBeNull();
    expect(ownerMessagePost?.disabled).toBe(true);
    expect(ownerMessagePost?.getAttribute('aria-pressed')).toBe('true');
    expect(ownerMessagePost?.querySelector('.uil--check')).not.toBeNull();
  });

  it('shows the "no roles" hint when the resolver returns no roles', async () => {
    nextTierRoles = { applicablePermissions: [], roles: [] };
    const { container } = render(PermissionMatrix, { props: { spaceId: 'space-1' } });
    await settle();

    expect(container.textContent).toContain('No roles applicable at this scope');
  });
});
