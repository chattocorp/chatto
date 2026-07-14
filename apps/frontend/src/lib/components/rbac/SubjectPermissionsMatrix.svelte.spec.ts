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
