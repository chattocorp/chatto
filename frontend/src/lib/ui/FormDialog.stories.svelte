<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import FormDialog from './FormDialog.svelte';
  import { TextInput, TextArea, Select, Checkbox } from './form';

  const { Story } = defineMeta({
    title: 'UI/FormDialog',
    component: FormDialog,
    tags: ['autodocs']
  });
</script>

<script lang="ts">
  let basicVisible = $state(false);
  let basicName = $state('');
  let basicDesc = $state('');

  let loadingVisible = $state(false);
  let loadingName = $state('');
  let loading = $state(false);

  let dangerVisible = $state(false);
  let dangerConfirm = $state('');
  const expectedConfirmation = 'delete my account';

  let inviteVisible = $state(false);
  let inviteEmail = $state('');
  let inviteRole = $state('member');
  let inviteWelcome = $state('');
  let inviteSendEmail = $state(true);

  function fakeSubmit() {
    loading = true;
    setTimeout(() => {
      loading = false;
      loadingVisible = false;
      loadingName = '';
    }, 1200);
  }
</script>

<Story name="Basic" asChild>
  <button class="btn-primary" onclick={() => (basicVisible = true)}>Create Room…</button>

  <FormDialog
    bind:visible={basicVisible}
    title="Create a New Room"
    submitLabel="Create Room"
    disabled={!basicName.trim()}
    onsubmit={() => (basicVisible = false)}
    onclose={() => (basicVisible = false)}
  >
    {#snippet description()}
      Rooms are conversations within your space.
    {/snippet}

    <TextInput id="story-room-name" label="Room Name" bind:value={basicName} />
    <TextArea
      id="story-room-desc"
      label="Description (optional)"
      bind:value={basicDesc}
      rows={3}
    />
  </FormDialog>
</Story>

<Story name="Loading state" asChild>
  <button class="btn-primary" onclick={() => (loadingVisible = true)}>Open loading form</button>

  <FormDialog
    bind:visible={loadingVisible}
    title="Invite Member"
    submitLabel="Send Invite"
    submitLoadingText="Sending…"
    {loading}
    disabled={!loadingName.trim()}
    onsubmit={fakeSubmit}
    onclose={() => (loadingVisible = false)}
  >
    <TextInput
      id="story-invite-email"
      label="Email address"
      placeholder="hendrik@example.com"
      bind:value={loadingName}
    />
  </FormDialog>
</Story>

<Story name="Danger submit (typed confirmation)" asChild>
  <button class="btn-primary" onclick={() => (dangerVisible = true)}>Delete account…</button>

  <FormDialog
    bind:visible={dangerVisible}
    title="Delete Account"
    submitLabel="Delete Account"
    submitTone="danger"
    disabled={dangerConfirm.trim() !== expectedConfirmation}
    error={dangerConfirm && dangerConfirm.trim() !== expectedConfirmation
      ? 'Confirmation text does not match.'
      : undefined}
    onsubmit={() => (dangerVisible = false)}
    onclose={() => (dangerVisible = false)}
  >
    {#snippet description()}
      This permanently deletes all your data. This cannot be undone.
    {/snippet}

    <TextInput
      id="story-delete-confirm"
      label={`Type "${expectedConfirmation}" to confirm`}
      bind:value={dangerConfirm}
    />
  </FormDialog>
</Story>

<Story name="Rich form (icons + select + checkbox)" asChild>
  <button class="btn-primary" onclick={() => (inviteVisible = true)}>Invite member…</button>

  <FormDialog
    bind:visible={inviteVisible}
    title="Invite Member"
    submitLabel="Send Invite"
    disabled={!inviteEmail.trim()}
    onsubmit={() => (inviteVisible = false)}
    onclose={() => (inviteVisible = false)}
  >
    {#snippet description()}
      We'll send a one-time link they can use to join this space.
    {/snippet}

    <TextInput
      id="story-invite-email-rich"
      label="Email address"
      bind:value={inviteEmail}
      type="email"
      leadingIcon="uil--envelope"
      placeholder="hendrik@example.com"
      required
    />
    <Select
      id="story-invite-role"
      label="Role"
      bind:value={inviteRole}
      options={[
        { value: 'member', label: 'Member — can post in joined rooms' },
        { value: 'moderator', label: 'Moderator — can manage rooms and members' },
        { value: 'admin', label: 'Admin — full control of this space' }
      ]}
    />
    <TextArea
      id="story-invite-welcome"
      label="Welcome note (optional)"
      bind:value={inviteWelcome}
      rows={2}
      maxlength={140}
      placeholder="Welcome to the team!"
    />
    <Checkbox
      id="story-invite-send-email"
      label="Send a welcome email to this address"
      bind:checked={inviteSendEmail}
    />
  </FormDialog>
</Story>
