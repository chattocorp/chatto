import '../../../app.css';
import { expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { flushSync } from 'svelte';
import SubjectPermissionsMatrix, { type MatrixData } from './SubjectPermissionsMatrix.svelte';

const data: MatrixData = {
  applicablePermissions: ['message.post', 'message.delete'],
  scopes: [
    { id: 'server', label: 'Server', kind: 'SERVER', parentGroupId: '' },
    { id: 'group:general', label: 'General', kind: 'GROUP', parentGroupId: '' }
  ],
  cells: [
    {
      permission: 'message.post',
      scopeId: 'server',
      override: 'ALLOW',
      effective: 'ALLOW'
    },
    {
      permission: 'message.post',
      scopeId: 'group:general',
      override: 'NONE',
      effective: 'ALLOW'
    },
    {
      permission: 'message.delete',
      scopeId: 'server',
      override: 'NONE',
      effective: 'NONE'
    },
    {
      permission: 'message.delete',
      scopeId: 'group:general',
      override: 'DENY',
      effective: 'DENY'
    }
  ]
};

it('highlights the hovered permission row and scope column', () => {
  const { container } = render(SubjectPermissionsMatrix, {
    props: { data, onCycle: vi.fn() }
  });
  const intersection = container.querySelector(
    'td[data-scope="group:general"][data-permission="message.post"]'
  ) as HTMLTableCellElement;
  const sameRow = container.querySelector(
    'td[data-scope="server"][data-permission="message.post"]'
  ) as HTMLTableCellElement;
  const sameColumn = container.querySelector(
    'td[data-scope="group:general"][data-permission="message.delete"]'
  ) as HTMLTableCellElement;
  const unrelated = container.querySelector(
    'td[data-scope="server"][data-permission="message.delete"]'
  ) as HTMLTableCellElement;
  const columnHeader = container.querySelector('th[data-scope="group:general"]') as HTMLElement;
  const permissionName = intersection.parentElement!.querySelector(
    '[data-testid="permission-name"]'
  ) as HTMLElement;

  intersection.dispatchEvent(new MouseEvent('mouseenter'));
  flushSync();

  expect(intersection.className).toContain('bg-action/15');
  expect(sameRow.className).toContain('bg-action/8');
  expect(sameColumn.className).toContain('bg-action/8');
  expect(unrelated.className).not.toContain('bg-action/');
  expect(columnHeader.className).toContain('bg-action/10');
  expect(permissionName.className).toContain('text-action');
  expect(getComputedStyle(intersection).backgroundColor).not.toBe(
    getComputedStyle(sameRow).backgroundColor
  );
});

it('renders one alphabetically ordered matrix without category dividers', () => {
  const { container } = render(SubjectPermissionsMatrix, {
    props: {
      data: {
        ...data,
        applicablePermissions: ['user.delete-self', 'room.manage', 'server.manage'],
        cells: [
          {
            permission: 'user.delete-self',
            scopeId: 'server',
            override: 'NONE',
            effective: 'NONE'
          },
          {
            permission: 'room.manage',
            scopeId: 'server',
            override: 'NONE',
            effective: 'NONE'
          },
          {
            permission: 'server.manage',
            scopeId: 'server',
            override: 'NONE',
            effective: 'NONE'
          }
        ]
      },
      onCycle: vi.fn()
    }
  });

  expect(container.querySelectorAll('table')).toHaveLength(1);
  expect(container.querySelector('.panel-header')?.textContent).toContain('Permissions');
  expect(container.querySelector('table')?.className).toContain('w-full');
  expect(container.querySelectorAll('[data-testid="permission-matrix-spacer"]')).toHaveLength(3);
  expect(container.querySelector('thead th:last-child')?.className).toContain('bg-background');
  expect([...container.querySelectorAll('[data-testid="permission-name"]')].map((row) => row.textContent)).toEqual(
    ['room.manage', 'server.manage', 'user.delete-self']
  );
});

it('filters permission names as the query changes', () => {
  const { container } = render(SubjectPermissionsMatrix, { props: { data, onCycle: vi.fn() } });
  const filter = container.querySelector<HTMLInputElement>('[data-testid="permission-filter"]')!;

  filter.value = 'delete';
  filter.dispatchEvent(new Event('input', { bubbles: true }));
  flushSync();

  expect([...container.querySelectorAll('[data-testid="permission-name"]')].map((row) => row.textContent)).toEqual(
    ['message.delete']
  );
});

it('visually hides the redundant filter label and focuses the filter with Cmd/Ctrl-/', () => {
  const { container } = render(SubjectPermissionsMatrix, { props: { data, onCycle: vi.fn() } });
  const filter = container.querySelector<HTMLInputElement>('[data-testid="permission-filter"]')!;
  const label = container.querySelector<HTMLLabelElement>('label[for="permission-filter"]')!;
  const shortcut = new KeyboardEvent('keydown', {
    key: '/',
    metaKey: true,
    bubbles: true,
    cancelable: true
  });

  filter.value = 'message';
  filter.dispatchEvent(new Event('input', { bubbles: true }));
  flushSync();
  window.dispatchEvent(shortcut);

  expect(label.className).toContain('sr-only');
  expect(filter.placeholder).toMatch(/(⌘\/|Ctrl-\/)/);
  expect(shortcut.defaultPrevented).toBe(true);
  expect(document.activeElement).toBe(filter);
  expect(filter.selectionStart).toBe(0);
  expect(filter.selectionEnd).toBe(7);
});
