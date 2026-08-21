import '../../../app.css';
import { beforeEach, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { flushSync } from 'svelte';
import { loadLocaleMessages } from '$lib/i18n/messages';
import { setReactiveLocale } from '$lib/i18n/state.svelte';
import SubjectPermissionsMatrix, { type MatrixData } from './SubjectPermissionsMatrix.svelte';

beforeEach(async () => {
  await loadLocaleMessages('en-GB');
  setReactiveLocale('en-GB');
});

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
  expect(container.querySelector('.data-table-viewport')?.className).toContain('overflow-x-auto');
  expect(container.querySelector('.data-table-viewport')?.className).not.toContain('max-h-[70dvh]');
  expect(container.querySelector('thead')?.className).not.toContain('sticky');
  expect(container.querySelectorAll('[data-testid="permission-matrix-spacer"]')).toHaveLength(3);
  expect(container.querySelector('thead th:last-child')?.className).toContain('bg-background');
  expect(
    [...container.querySelectorAll('[data-testid="permission-name"]')].map((row) => row.textContent)
  ).toEqual(['room.manage', 'server.manage', 'user.delete-self']);
});

it('filters permission names as the query changes', () => {
  const { container } = render(SubjectPermissionsMatrix, { props: { data, onCycle: vi.fn() } });
  const filter = container.querySelector<HTMLInputElement>('[data-testid="permission-filter"]')!;

  filter.value = 'delete';
  filter.dispatchEvent(new Event('input', { bubbles: true }));
  flushSync();

  expect(
    [...container.querySelectorAll('[data-testid="permission-name"]')].map((row) => row.textContent)
  ).toEqual(['message.delete']);
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

it('locks inherited binary grants and only clears direct grants', () => {
  const onCycle = vi.fn();
  const { container } = render(SubjectPermissionsMatrix, {
    props: { data, onCycle, decisionMode: 'binary' }
  });

  const inheritedAllow = container.querySelector(
    'td[data-scope="group:general"][data-permission="message.post"] button'
  ) as HTMLButtonElement;
  expect(inheritedAllow.querySelector('[class~="bg-success/15"]')).not.toBeNull();
  expect(inheritedAllow.querySelector('[class~="icon-[uil--lock]"]')).not.toBeNull();
  expect(inheritedAllow.disabled).toBe(true);
  inheritedAllow.click();
  expect(onCycle).not.toHaveBeenCalled();

  const directAllow = container.querySelector(
    'td[data-scope="server"][data-permission="message.post"] button'
  ) as HTMLButtonElement;
  directAllow.click();
  expect(onCycle).toHaveBeenLastCalledWith(
    expect.objectContaining({ id: 'server' }),
    'message.post',
    'neutral'
  );

  const legacyDeny = container.querySelector(
    'td[data-scope="group:general"][data-permission="message.delete"] button'
  ) as HTMLButtonElement;
  legacyDeny.click();
  expect(onCycle).toHaveBeenLastCalledWith(
    expect.objectContaining({ id: 'group:general' }),
    'message.delete',
    'neutral'
  );
  expect(container.querySelector('[class~="icon-[uil--times]"]')).toBeNull();
});

it('renders an ungrantable binary permission as a non-interactive locked cell', () => {
  const onCycle = vi.fn();
  const { container } = render(SubjectPermissionsMatrix, {
    props: {
      data: {
        applicablePermissions: ['message.post'],
        scopes: [{ id: 'server', label: 'Server', kind: 'SERVER', parentGroupId: '' }],
        cells: [
          {
            permission: 'message.post',
            scopeId: 'server',
            override: 'NONE',
            effective: 'NONE',
            allowPermitted: false
          }
        ]
      },
      onCycle,
      decisionMode: 'binary'
    }
  });
  const button = container.querySelector(
    'button[aria-label^="message.post is"]'
  ) as HTMLButtonElement;

  expect(button.disabled).toBe(true);
  expect(button.className).toContain('cursor-not-allowed');
  expect(button.className).not.toContain('cursor-pointer');
  expect(button.firstElementChild!.className).not.toContain('hover:');
  expect(button.querySelector('[class~="icon-[uil--lock]"]')).not.toBeNull();
  expect(button.querySelector('[class~="icon-[uil--exclamation-triangle]"]')).toBeNull();

  button.click();
  expect(onCycle).not.toHaveBeenCalled();
});

it('locks an inherited room grant while allowing an older denial to be cleared', async () => {
  const onCycle = vi.fn();
  const roomData: MatrixData = {
    applicablePermissions: ['message.post'],
    scopes: [
      { id: 'server', label: 'Server', kind: 'SERVER', parentGroupId: '' },
      { id: 'group:general', label: 'General', kind: 'GROUP', parentGroupId: '' },
      { id: 'room:lobby', label: 'Lobby', kind: 'ROOM', parentGroupId: 'general' }
    ],
    cells: [
      {
        permission: 'message.post',
        scopeId: 'server',
        override: 'NONE',
        effective: 'NONE'
      },
      {
        permission: 'message.post',
        scopeId: 'group:general',
        override: 'ALLOW',
        effective: 'ALLOW'
      },
      {
        permission: 'message.post',
        scopeId: 'room:lobby',
        override: 'NONE',
        effective: 'ALLOW',
        allowPermitted: false
      }
    ]
  };
  const rendered = render(SubjectPermissionsMatrix, {
    props: { data: roomData, onCycle, decisionMode: 'binary' }
  });
  const roomCell = () =>
    rendered.container.querySelector(
      'td[data-scope="room:lobby"][data-permission="message.post"] button'
    ) as HTMLButtonElement;

  expect(roomCell().title).toContain('Enabled · Inherited from a broader scope');
  expect(roomCell().title).toContain('Currently unavailable');
  expect(roomCell().title).toContain("bot's owner");
  expect(roomCell().querySelector('[class~="bg-warning/20"]')).not.toBeNull();
  expect(roomCell().querySelector('[class~="icon-[uil--lock]"]')).not.toBeNull();
  expect(roomCell().disabled).toBe(true);
  roomCell().click();
  expect(onCycle).not.toHaveBeenCalled();

  await rendered.rerender({
    data: {
      ...roomData,
      cells: roomData.cells.map((cell) =>
        cell.scopeId === 'room:lobby' ? { ...cell, override: 'DENY' as const } : cell
      )
    },
    onCycle,
    decisionMode: 'binary'
  });
  expect(roomCell().disabled).toBe(false);
  roomCell().click();
  expect(onCycle).toHaveBeenLastCalledWith(
    expect.objectContaining({ id: 'room:lobby' }),
    'message.post',
    'neutral'
  );

  await rendered.rerender({ data: roomData, onCycle, decisionMode: 'binary' });
  expect(roomCell().title).toContain('Enabled · Inherited from a broader scope');
  expect(roomCell().querySelector('[class~="bg-warning/20"]')).not.toBeNull();
  expect(roomCell().querySelector('[class~="icon-[uil--lock]"]')).not.toBeNull();
  expect(roomCell().disabled).toBe(true);
});

it('localizes binary cell labels, state details, and owner ceilings', async () => {
  await loadLocaleMessages('de-DE');
  setReactiveLocale('de-DE');
  const { container } = render(SubjectPermissionsMatrix, {
    props: {
      data: {
        applicablePermissions: ['message.post'],
        scopes: [{ id: 'server', label: 'Server', kind: 'SERVER', parentGroupId: '' }],
        cells: [
          {
            permission: 'message.post',
            scopeId: 'server',
            override: 'ALLOW',
            effective: 'ALLOW',
            allowPermitted: false
          }
        ]
      },
      onCycle: vi.fn(),
      subjectKind: 'Bot',
      decisionMode: 'binary'
    }
  });
  const button = container.querySelector(
    'button[aria-label^="message.post"]'
  ) as HTMLButtonElement;

  expect(button.ariaLabel).toBe('message.post ist für Bot in Server aktiviert');
  expect(button.title).toContain('Derzeit nicht verfügbar');
  expect(button.title).toContain('Du kannst message.post in Server nicht vergeben');
});
