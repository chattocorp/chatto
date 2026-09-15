<script lang="ts">
  import { beforeNavigate, goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { Code, ConnectError } from '@connectrpc/connect';
  import { createQuery } from '@tanstack/svelte-query';
  import { onDestroy } from 'svelte';
  import type { VerifiedEmail } from '$lib/api-client/account';
  import { createAccountAPI } from '$lib/api-client/account';
  import { m } from '$lib/i18n/messages';
  import { queryClient } from '$lib/query/client';
  import { adminQueryKeys } from '$lib/query/admin';
  import { settingsQueryKeys } from '$lib/query/settings';
  import { serverIdToSegment } from '$lib/navigation';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import type { ServerConnection } from '$lib/state/server/serverConnection.svelte';
  import { DataTable, FormDialog, Hint, Panel, Pill } from '$lib/ui';
  import { Button, TextInput } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast/toastState.svelte';
  import {
    clearPendingEmailVerification,
    storePendingEmailVerification
  } from '$lib/verifiedEmailChallenge';

  const serverScope = useServerScope();
  let componentActive = true;
  let navigationGeneration = 0;
  let email = $state('');
  let emailConfirmation = $state('');
  let addEmailVisible = $state(false);
  let addEmailError = $state('');
  let actionError = $state('');
  let requesting = $state(false);
  let selectingEmail = $state('');

  beforeNavigate(() => {
    navigationGeneration += 1;
  });
  onDestroy(() => {
    componentActive = false;
    navigationGeneration += 1;
  });

  type EmailActionScope = {
    serverId: string;
    connection: ServerConnection;
    userId: string;
    navigationGeneration: number;
  };

  const viewerUserId = $derived(serverScope.store.currentUser.user?.id ?? '');

  function emailActionScope(): EmailActionScope {
    return {
      serverId: serverScope.serverId,
      connection: serverScope.connection,
      userId: viewerUserId,
      navigationGeneration
    };
  }

  function isCurrentEmailContext(scope: EmailActionScope): boolean {
    return (
      componentActive &&
      serverScope.isCurrent() &&
      scope.serverId === serverScope.serverId &&
      scope.connection.queryScope === serverScope.connection.queryScope &&
      scope.userId !== '' &&
      scope.userId === viewerUserId
    );
  }

  function isCurrentEmailAction(scope: EmailActionScope): boolean {
    return (
      isCurrentEmailContext(scope) && scope.navigationGeneration === navigationGeneration
    );
  }

  const verificationPath = $derived(
    resolve('/chat/[serverId]/settings/account/verify-email', {
      serverId: serverIdToSegment(serverScope.serverId)
    })
  );

  const emailsQuery = createQuery(
    () => {
      const connection = serverScope.connection;
      const userId = viewerUserId;
      return {
        queryKey: settingsQueryKeys.verifiedEmails(serverScope.serverId, connection, userId),
        queryFn: () => connection.getAPI(createAccountAPI).listVerifiedEmails(userId),
        enabled: userId !== ''
      };
    },
    () => queryClient
  );

  const verifiedEmails = $derived(viewerUserId ? (emailsQuery.data ?? []) : []);
  const emailsMatch = $derived(
    email.trim() !== '' &&
      emailConfirmation.trim() !== '' &&
      email.trim().toLowerCase() === emailConfirmation.trim().toLowerCase()
  );
  const emailConfirmationError = $derived(
    emailConfirmation.trim() !== '' && !emailsMatch
      ? m('settings.account.email.addresses_mismatch')
      : ''
  );
  const error = $derived(
    actionError ||
      (emailsQuery.error instanceof Error
        ? emailsQuery.error.message
        : emailsQuery.error
          ? m('settings.account.email.load_failed')
          : '')
  );

  function setEmails(scope: EmailActionScope, value: VerifiedEmail[]) {
    queryClient.setQueryData(
      settingsQueryKeys.verifiedEmails(scope.serverId, scope.connection, scope.userId),
      value
    );
  }

  function openAddEmail() {
    email = '';
    emailConfirmation = '';
    addEmailError = '';
    addEmailVisible = true;
  }

  function closeAddEmail() {
    if (requesting) return;
    email = '';
    emailConfirmation = '';
    addEmailError = '';
    addEmailVisible = false;
  }

  async function requestCode(event: SubmitEvent) {
    event.preventDefault();
    const address = email.trim().toLowerCase();
    if (!address || !emailsMatch) return;
    if (verifiedEmails.some((verified) => verified.email.toLowerCase() === address)) {
      addEmailError = m('settings.account.email.already_verified');
      return;
    }
    const scope = emailActionScope();
    if (
      !scope.userId ||
      !storePendingEmailVerification(scope.serverId, scope.userId, address)
    ) {
      addEmailError = m('settings.account.email.request_failed');
      return;
    }
    requesting = true;
    addEmailError = '';
    try {
      await scope.connection
        .getAPI(createAccountAPI)
        .requestEmailVerification(scope.userId, address);
      if (!isCurrentEmailAction(scope)) return;
      addEmailVisible = false;
      await goto(verificationPath);
    } catch (err) {
      clearPendingEmailVerification(scope.serverId, scope.userId, address);
      if (!isCurrentEmailContext(scope)) return;
      addEmailError =
        err instanceof ConnectError && err.code === Code.AlreadyExists
          ? m('settings.account.email.already_verified')
          : err instanceof Error
            ? err.message
            : m('settings.account.email.request_failed');
    } finally {
      if (isCurrentEmailContext(scope)) requesting = false;
    }
  }

  async function setPrimary(address: string) {
    const scope = emailActionScope();
    if (!scope.userId) return;
    selectingEmail = address;
    actionError = '';
    try {
      const next = await scope.connection
        .getAPI(createAccountAPI)
        .setPrimaryEmail(scope.userId, address);
      if (!isCurrentEmailAction(scope)) return;
      setEmails(scope, next);
      void queryClient.invalidateQueries({
        queryKey: adminQueryKeys.membersRoot(scope.serverId, scope.connection)
      });
      void queryClient.invalidateQueries({
        queryKey: adminQueryKeys.member(scope.serverId, scope.connection, scope.userId),
        exact: true
      });
      toast.success(m('settings.account.email.primary_changed'));
    } catch (err) {
      if (!isCurrentEmailContext(scope)) return;
      actionError = err instanceof Error ? err.message : m('settings.account.email.primary_failed');
    } finally {
      if (isCurrentEmailContext(scope)) selectingEmail = '';
    }
  }
</script>

<Panel
  title={m('settings.account.email.title')}
  subtitle={m('settings.account.email.description')}
  icon="iconify icon-[uil--envelope-check]"
  noPadding
>
  {#snippet actions()}
    <Button size="sm" onclick={openAddEmail}>
      <span class="iconify icon-[uil--plus]" aria-hidden="true"></span>
      {m('settings.account.email.add')}
    </Button>
  {/snippet}

  {#if error}
    <div class="border-b border-border p-4">
      <Hint tone="danger">{error}</Hint>
    </div>
  {/if}

  <DataTable
    items={verifiedEmails}
    columns={2}
    getKey={(verified) => verified.email}
    emptyMessage={emailsQuery.isPending && !emailsQuery.data
      ? m('settings.account.email.loading')
      : m('settings.account.email.none')}
  >
    {#snippet header()}
      <th class="table-header-cell">{m('settings.account.email.address_label')}</th>
      <th class="table-header-cell text-end">{m('settings.account.email.primary')}</th>
    {/snippet}
    {#snippet row(verified)}
      <td class="px-4 py-2 font-medium">
        <div class="flex h-10 items-center">
          <bdi class="whitespace-nowrap" dir="ltr" title={verified.email}>{verified.email}</bdi>
        </div>
      </td>
      <td class="px-4 py-2 text-end">
        <div class="flex h-10 items-center justify-end">
          {#if verified.primary}
            <Pill tone="success">{m('settings.account.email.primary')}</Pill>
          {:else}
            <Button
              size="sm"
              variant="secondary"
              loading={selectingEmail === verified.email}
              disabled={selectingEmail !== ''}
              onclick={() => setPrimary(verified.email)}
            >
              <span class="iconify icon-[uil--check-circle]" aria-hidden="true"></span>
              {m('settings.account.email.make_primary')}
            </Button>
          {/if}
        </div>
      </td>
    {/snippet}
  </DataTable>
</Panel>

<FormDialog
  bind:visible={addEmailVisible}
  title={m('settings.account.email.add')}
  size="sm"
  submitLabel={m('settings.account.email.send_code')}
  submitIcon="iconify icon-[uil--envelope]"
  loading={requesting}
  disabled={!emailsMatch}
  error={addEmailError}
  onsubmit={requestCode}
  onclose={closeAddEmail}
>
  {#snippet description()}
    {m('settings.account.email.add_description')}
  {/snippet}

  <TextInput
    id="verified-email-address"
    label={m('settings.account.email.address_label')}
    type="email"
    autocomplete="email"
    bind:value={email}
    disabled={requesting}
    required
  />
  <TextInput
    id="verified-email-address-confirmation"
    label={m('settings.account.email.confirm_address_label')}
    type="email"
    autocomplete="off"
    bind:value={emailConfirmation}
    error={emailConfirmationError}
    disabled={requesting}
    required
  />
</FormDialog>
