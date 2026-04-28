import { describe, it, expect, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { flushSync } from 'svelte';
import PermissionMatrix from './PermissionMatrix.svelte';

// The matrix queries `tierRoles` itself; we mock the connection so the
// component reads its data straight from the resolver mock. Production
// urql returns an `OperationResultSource` which is both `await`-able (via
// `then`) and `.toPromise()`-able; the mock matches both shapes.

const mockTierRoles = {
  applicablePermissions: ['message.post', 'room.create'],
  roles: [
    {
      roleName: 'admin',
      displayName: 'Admin',
      description: '',
      isInstanceRole: false,
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
      isInstanceRole: false,
      isSystem: true,
      position: 2,
      override: { permissions: [], permissionDenials: ['room.create'] },
      inheritedAllows: ['message.post'],
      inheritedDenials: []
    }
  ]
};

function thenableResult(data: unknown) {
  const value = { data, error: null };
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
      query: vi.fn(() => thenableResult({ tierRoles: mockTierRoles })),
      mutation: vi.fn(() => thenableResult({ grantSpacePermission: true })),
      subscription: vi.fn()
    }
  })
}));

describe('PermissionMatrix', () => {
  async function renderAndWait() {
    const result = render(PermissionMatrix, { props: { spaceId: 'space-1' } });
    // Microtask + sync flush is enough — the mock query resolves immediately.
    await Promise.resolve();
    await Promise.resolve();
    flushSync();
    return result;
  }

  it('renders one column per role and one row per permission', async () => {
    const { container } = await renderAndWait();

    const tables = container.querySelectorAll('table');
    expect(tables.length).toBeGreaterThan(0);

    // "Permission" + "Admin" + "Moderator" per category panel. There are two
    // categories ('message' and 'room'), each with its own table.
    const headerCells = container.querySelectorAll('thead th');
    expect(headerCells.length).toBe(6);

    // Rows: one per permission, total 2 across categories.
    const dataRows = container.querySelectorAll('tbody tr');
    expect(dataRows.length).toBe(2);
  });

  it('reflects override + inherited state in cell aria-pressed', async () => {
    const { container } = await renderAndWait();

    // For Admin / message.post: override = allow → aria-pressed = true
    const adminMessagePost = container.querySelector(
      'button[aria-label*="Admin"][aria-label*="message.post"]'
    );
    expect(adminMessagePost?.getAttribute('aria-pressed')).toBe('true');

    // For Moderator / message.post: no override but inherited allow.
    const modMessagePost = container.querySelector(
      'button[aria-label*="Moderator"][aria-label*="message.post"]'
    );
    expect(modMessagePost?.getAttribute('aria-pressed')).toBe('false');
    // Inherited indicator surfaces through the icon: a 'check' is present.
    expect(modMessagePost?.querySelector('.uil--check')).not.toBeNull();
  });

  it('invokes onRoleClick when a column header is clicked', async () => {
    const onRoleClick = vi.fn();
    const result = render(PermissionMatrix, {
      props: { spaceId: 'space-1', onRoleClick }
    });
    await Promise.resolve();
    await Promise.resolve();
    flushSync();
    const { container } = result;

    const headerButtons = Array.from(
      container.querySelectorAll('thead button')
    ) as HTMLButtonElement[];
    const adminHeader = headerButtons.find((b) => b.textContent?.trim() === '@admin');
    expect(adminHeader).toBeDefined();
    adminHeader!.click();
    flushSync();
    expect(onRoleClick).toHaveBeenCalledWith(
      expect.objectContaining({ roleName: 'admin', isInstanceRole: false })
    );
  });
});
