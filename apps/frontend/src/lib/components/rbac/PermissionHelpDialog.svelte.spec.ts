import '../../../app.css';
import { beforeEach, describe, expect, it } from 'vitest';
import { page } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import { loadLocaleMessages } from '$lib/i18n/messages';
import { setReactiveLocale } from '$lib/i18n/state.svelte';
import PermissionHelpDialog from './PermissionHelpDialog.svelte';

const permissions = [
  'message.manage',
  'message.post',
  'message.post-in-thread',
  'message.read',
  'message.read-interactions'
];

beforeEach(async () => {
  await loadLocaleMessages('en-GB');
  setReactiveLocale('en-GB');
});

function openHelp(permission: string) {
  render(PermissionHelpDialog, { props: { visible: true, permission, permissions } });
  return page.getByTestId('permission-help');
}

describe('PermissionHelpDialog', () => {
  it('explains a permission with its summary, details, and scopes', async () => {
    const help = openHelp('message.post');

    await expect.element(page.getByRole('dialog', { name: 'message.post' })).toBeVisible();
    await expect.element(help).toHaveTextContent('Post messages and thread replies, and start DMs');
    await expect.element(help).toHaveTextContent('To stop all posting');
    await expect.element(help).toHaveTextContent('Can be set for Server, Group, Room, DM');
    await expect.element(help).not.toHaveTextContent('Needs privileged mode');
  });

  it('lists only configurable related permissions and switches to them', async () => {
    const help = openHelp('message.post');

    await expect.element(help).toHaveTextContent('Includes message.post-in-thread');
    // message.post-in-interactions is not in the configurable list.
    await expect
      .element(page.getByRole('button', { name: 'message.post-in-interactions' }))
      .not.toBeInTheDocument();

    await page.getByRole('button', { name: 'message.post-in-thread' }).click();

    await expect
      .element(page.getByRole('dialog', { name: 'message.post-in-thread' }))
      .toBeVisible();
    await expect.element(help).toHaveTextContent('Included in message.post');
  });

  it('marks permissions that need privileged mode', async () => {
    const help = openHelp('message.manage');

    await expect.element(help).toHaveTextContent('Needs privileged mode');
  });

  it('shows only the category for a permission from a newer server', async () => {
    const help = openHelp('future.permission');

    await expect.element(help).toHaveTextContent('Category Other');
    await expect.element(help).not.toHaveTextContent('Can be set for');
  });
});
