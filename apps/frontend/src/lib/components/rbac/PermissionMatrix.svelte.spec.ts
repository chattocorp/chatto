import '../../../app.css';
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

vi.mock('$lib/api-client/permissions', () => ({
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
  it('renders one alphabetically ordered permission matrix without category dividers', async () => {
    const { container } = render(PermissionMatrix, { props: { spaceId: 'space-1' } });
    await settle();

    const tables = container.querySelectorAll('table');
    expect(tables).toHaveLength(1);
    // The permission and role columns, followed by the flexible spacer.
    expect(container.querySelectorAll('thead th')).toHaveLength(5);
    expect(container.querySelectorAll('tbody tr')).toHaveLength(2);
    expect(container.querySelectorAll('[data-testid="permission-section-divider"]')).toHaveLength(
      0
    );
    expect(container.querySelector('table')?.className).toContain('w-full');
    expect(container.querySelectorAll('[data-testid="permission-matrix-spacer"]')).toHaveLength(2);
    expect(container.querySelector('thead th:last-child')?.className).toContain('bg-background');
    expect(container.querySelectorAll('tbody h3')).toHaveLength(0);
  });

  it('orders permission names alphabetically', async () => {
    nextTierRoles = {
      ...HAPPY_TIER_ROLES,
      applicablePermissions: ['user.delete-self', 'room.manage', 'server.manage', 'user.delete-any']
    };
    const { container } = render(PermissionMatrix, { props: { spaceId: 'space-1' } });
    await settle();

    expect(
      [...container.querySelectorAll('[data-testid="permission-name"]')].map(
        (permission) => permission.textContent
      )
    ).toEqual(['room.manage', 'server.manage', 'user.delete-any', 'user.delete-self']);
  });

  it('filters permission names as the query changes', async () => {
    const { container } = render(PermissionMatrix, { props: { spaceId: 'space-1' } });
    await settle();

    const filter = container.querySelector<HTMLInputElement>('[data-testid="permission-filter"]')!;
    filter.value = 'room';
    filter.dispatchEvent(new Event('input', { bubbles: true }));
    flushSync();

    expect(
      [...container.querySelectorAll('[data-testid="permission-name"]')].map(
        (row) => row.textContent
      )
    ).toEqual(['room.create']);

    filter.value = 'missing';
    filter.dispatchEvent(new Event('input', { bubbles: true }));
    flushSync();

    expect(container.textContent).toContain('No permissions match this filter.');
  });

  it('visually hides the redundant filter label and focuses the filter with Cmd/Ctrl-/', async () => {
    const { container } = render(PermissionMatrix, { props: { spaceId: 'space-1' } });
    await settle();

    const filter = container.querySelector<HTMLInputElement>('[data-testid="permission-filter"]')!;
    const label = container.querySelector<HTMLLabelElement>('label[for="permission-filter"]')!;
    const shortcut = new KeyboardEvent('keydown', {
      key: '/',
      ctrlKey: true,
      bubbles: true,
      cancelable: true
    });

    filter.value = 'room';
    filter.dispatchEvent(new Event('input', { bubbles: true }));
    flushSync();
    window.dispatchEvent(shortcut);

    expect(label.className).toContain('sr-only');
    expect(filter.placeholder).toMatch(/(⌘\/|Ctrl-\/)/);
    expect(shortcut.defaultPrevented).toBe(true);
    expect(document.activeElement).toBe(filter);
    expect(filter.selectionStart).toBe(0);
    expect(filter.selectionEnd).toBe(4);
  });

  it('labels the server-level matrix', async () => {
    const { container } = render(PermissionMatrix);
    await settle();

    expect(container.querySelector('.panel-header')?.textContent).toContain('Server Permissions');
  });

  it('renders an optional panel subtitle', async () => {
    const { container } = render(PermissionMatrix, {
      props: { subtitle: 'Default permissions that apply before room overrides.' }
    });
    await settle();

    expect(container.querySelector('.panel-header')?.textContent).toContain(
      'Default permissions that apply before room overrides.'
    );
  });

  it('renders a final column that links to create a role when supplied', async () => {
    const { container } = render(PermissionMatrix, {
      props: { newRoleHref: '/chat/server/manage/server/permissions/new' }
    });
    await settle();

    const newRoleColumn = container.querySelector('[data-testid="new-role-column"]');
    expect(newRoleColumn?.getAttribute('href')).toBe('/chat/server/manage/server/permissions/new');
    expect(newRoleColumn?.textContent).toContain('+ New Role');
    expect(container.querySelectorAll('thead th')).toHaveLength(6);
    expect(container.querySelectorAll('tbody tr')).toHaveLength(2);
  });

  it('contrasts the panel inset and sticky cells with the surface table header', async () => {
    const { container } = render(PermissionMatrix, { props: { spaceId: 'space-1' } });
    await settle();

    const panel = container.querySelector('.panel-shell') as HTMLElement;
    const tableHeader = panel.querySelector('thead tr') as HTMLElement;
    const stickyHeader = panel.querySelector('thead th.sticky') as HTMLElement;
    const stickyBody = panel.querySelector('tbody td.sticky') as HTMLElement;
    const surfaceColor = getComputedStyle(panel).backgroundColor;
    const headerColor = getComputedStyle(tableHeader).backgroundColor;
    const viewport = panel.querySelector('table')?.parentElement?.parentElement as HTMLElement;
    const inset = panel.querySelector(':scope > div:last-child > div') as HTMLElement;
    const frame = inset.parentElement as HTMLElement;

    const backgroundColor = getComputedStyle(inset).backgroundColor;

    expect(surfaceColor).not.toBe('rgba(0, 0, 0, 0)');
    expect(headerColor).toBe(surfaceColor);
    expect(backgroundColor).not.toBe(surfaceColor);
    expect(getComputedStyle(inset).backgroundColor).toBe(backgroundColor);
    expect(getComputedStyle(viewport).backgroundColor).toBe(backgroundColor);
    expect(frame.className).toContain('px-1');
    expect(frame.className).toContain('pb-1');
    expect(viewport.className).toContain('data-table-viewport');
    expect(viewport.className).not.toContain('rounded-md');
    expect((panel.querySelector('table')?.parentElement as HTMLElement).className).toContain(
      'overflow-y-auto'
    );
    expect((panel.querySelector('table')?.parentElement as HTMLElement).className).toContain(
      'overflow-x-auto'
    );
    expect(getComputedStyle(tableHeader).backgroundColor).toBe(headerColor);
    expect(getComputedStyle(panel.querySelector('thead') as HTMLElement).position).toBe('sticky');
    expect(getComputedStyle(stickyHeader).backgroundColor).toBe(backgroundColor);
    expect(getComputedStyle(stickyBody).backgroundColor).toBe(backgroundColor);
    expect(stickyBody.className).toContain('z-10');
    const fades = panel.querySelectorAll<HTMLElement>('[aria-hidden="true"]');
    expect(fades[fades.length - 1].className).toContain('z-30');
  });

  it('flows vertically with the page when contained scrolling is disabled', async () => {
    const { container } = render(PermissionMatrix, {
      props: { roomId: 'room-1', scrollContents: false }
    });
    await settle();

    const table = container.querySelector('table') as HTMLTableElement;
    const viewport = table.parentElement as HTMLElement;
    const header = container.querySelector('thead') as HTMLElement;

    expect(viewport.className).toContain('overflow-x-auto');
    expect(viewport.className).not.toContain('overflow-y-auto');
    expect(viewport.className).not.toContain('max-h-[70dvh]');
    expect(header.className).not.toContain('sticky');
  });

  it('highlights the hovered permission row and the role column across categories', async () => {
    const { container } = render(PermissionMatrix, { props: { spaceId: 'space-1' } });
    await settle();

    const intersection = container.querySelector(
      'td[data-role="moderator"][data-permission="message.post"]'
    ) as HTMLTableCellElement;
    const sameRow = container.querySelector(
      'td[data-role="admin"][data-permission="message.post"]'
    ) as HTMLTableCellElement;
    const sameColumn = container.querySelector(
      'td[data-role="moderator"][data-permission="room.create"]'
    ) as HTMLTableCellElement;
    const unrelated = container.querySelector(
      'td[data-role="admin"][data-permission="room.create"]'
    ) as HTMLTableCellElement;
    const columnHeader = container.querySelector('th[data-role="moderator"]') as HTMLElement;
    const rowLabel = intersection.parentElement!.querySelector('td.sticky') as HTMLElement;

    intersection.dispatchEvent(new MouseEvent('mouseenter'));
    flushSync();

    expect(intersection.className).toContain('bg-action/15');
    expect(sameRow.className).toContain('bg-action/8');
    expect(sameColumn.className).toContain('bg-action/8');
    expect(unrelated.className).not.toContain('bg-action/');
    expect(columnHeader.className).toContain('bg-action/10');
    expect(rowLabel.className).toContain('bg-action/8');
    expect(rowLabel.querySelector('[data-testid="permission-name"]')!.className).toContain(
      'text-action'
    );
    expect(getComputedStyle(intersection).backgroundColor).not.toBe(
      getComputedStyle(sameRow).backgroundColor
    );
    expect(getComputedStyle(sameRow).backgroundColor).not.toBe(
      getComputedStyle(unrelated).backgroundColor
    );

    intersection.dispatchEvent(new MouseEvent('mouseleave'));
    flushSync();

    expect(intersection.className).not.toContain('bg-action/');
    expect(sameRow.className).not.toContain('bg-action/');
    expect(sameColumn.className).not.toContain('bg-action/');
    expect(rowLabel.querySelector('[data-testid="permission-name"]')!.className).not.toContain(
      'text-action'
    );
  });

  it('keeps the coordinate highlight visible for keyboard focus', async () => {
    nextTierRoles = {
      ...HAPPY_TIER_ROLES,
      applicablePermissions: ['message.post', 'message.delete']
    };
    const { container } = render(PermissionMatrix, { props: { spaceId: 'space-1' } });
    await settle();

    const button = container.querySelector(
      'td[data-role="moderator"][data-permission="message.post"] button'
    ) as HTMLButtonElement;
    const cell = button.closest('td')!;

    button.focus();
    flushSync();
    expect(cell.className).toContain('bg-action/15');

    button.blur();
    flushSync();
    expect(cell.className).not.toContain('bg-action/');
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

  it('shows feedback immediately until a permission update completes', async () => {
    let resolveUpdate: (() => void) | undefined;
    permissionMocks.setRolePermission.mockImplementation(
      () => new Promise<void>((resolve) => (resolveUpdate = resolve))
    );
    const { container } = render(PermissionMatrix, { props: { spaceId: 'space-1' } });
    await settle();

    const button = container.querySelector(
      'button[aria-label*="Moderator"][aria-label*="room.create"]'
    ) as HTMLButtonElement;
    button.click();
    flushSync();

    expect(button.getAttribute('aria-busy')).toBe('true');
    expect(button.querySelector('.animate-spin.uil--spinner')).not.toBeNull();

    resolveUpdate?.();
    await settle();

    expect(button.hasAttribute('aria-busy')).toBe(false);
    expect(button.querySelector('.animate-spin.uil--spinner')).toBeNull();
    expect(button.querySelector('.uil--minus')).not.toBeNull();
  });

  it('invokes onRoleClick when a column header is clicked', async () => {
    const onRoleClick = vi.fn();
    const { container } = render(PermissionMatrix, {
      props: { onRoleClick }
    });
    await settle();

    const buttons = Array.from(container.querySelectorAll('thead button')) as HTMLButtonElement[];
    const adminHeader = buttons.find((b) => b.textContent?.trim() === '@admin');
    expect(adminHeader).toBeDefined();
    adminHeader!.click();
    flushSync();
    expect(onRoleClick).toHaveBeenCalledWith(expect.objectContaining({ roleName: 'admin' }));
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
