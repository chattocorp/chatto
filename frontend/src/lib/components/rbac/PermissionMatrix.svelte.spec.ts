import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { flushSync } from 'svelte';
import PermissionMatrix from './PermissionMatrix.svelte';

// Production urql returns an `OperationResultSource` that's both `await`-able
// (via `then`) and `.toPromise()`-able. The mocks below return the same
// shape so the matrix's `await client.query(...)` resolves identically.

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

function thenable(value: unknown) {
  return {
    then: (resolve: (v: unknown) => void) => Promise.resolve(value).then(resolve),
    toPromise: () => Promise.resolve(value)
  };
}

vi.mock('$lib/state/instance/connection.svelte', () => ({
  useConnection: () => () => ({
    isConnected: true,
    showConnectionLostBanner: false,
    client: {
      query: vi.fn(() => thenable({ data: { tierRoles: nextTierRoles }, error: null })),
      mutation: vi.fn(() => thenable({ data: {}, error: null })),
      subscription: vi.fn()
    }
  })
}));

beforeEach(() => {
  nextTierRoles = HAPPY_TIER_ROLES;
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
    // "Permission" + "@admin" + "@moderator" per category panel; two
    // categories ('message' and 'room'), so 6 header cells total.
    expect(container.querySelectorAll('thead th').length).toBe(6);
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

  it('shows the "no roles" hint when the resolver returns no roles', async () => {
    nextTierRoles = { applicablePermissions: [], roles: [] };
    const { container } = render(PermissionMatrix, { props: { spaceId: 'space-1' } });
    await settle();

    expect(container.textContent).toContain('No roles applicable at this scope');
  });
});
