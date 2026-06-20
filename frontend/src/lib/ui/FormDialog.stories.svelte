<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { Button, Checkbox, Select, TextArea, TextInput } from '$lib/ui/form';
  import FormDialog from './FormDialog.svelte';

  const componentDescription = `
    Use FormDialog for modal forms with a single submit action and cancel/close behavior. It owns
    the footer action pattern, loading state, disabled state, and top-level form error treatment.
  `.trim();

  const { Story } = defineMeta({
    title: 'UI/FormDialog',
    component: FormDialog,
    tags: ['autodocs'],
    parameters: {
      docs: {
        description: { component: componentDescription }
      }
    }
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

<Story
  name="Basic"
  asChild
  parameters={{
    docs: {
      description: {
        story:
          'Basic form dialog with description copy, required field state, and generated footer actions.'
      }
    }
  }}
>
  <Button onclick={() => (basicVisible = true)}>Create Room...</Button>

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
    <TextArea id="story-room-desc" label="Description (optional)" bind:value={basicDesc} rows={3} />
  </FormDialog>
</Story>

<Story
  name="Loading state"
  asChild
  parameters={{
    docs: {
      description: {
        story: 'Loading state locks the footer action while async submission is in progress.'
      }
    }
  }}
>
  <Button onclick={() => (loadingVisible = true)}>Open loading form</Button>

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

<Story
  name="Danger submit (typed confirmation)"
  asChild
  parameters={{
    docs: {
      description: {
        story:
          'Danger submit uses the dialog footer button tone while the field handles typed confirmation.'
      }
    }
  }}
>
  <Button variant="danger" onclick={() => (dangerVisible = true)}>Delete account...</Button>

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

<Story
  name="Rich form (icons + select + checkbox)"
  asChild
  parameters={{
    docs: {
      description: {
        story: 'A denser form combines shared field primitives without adding custom dialog layout.'
      }
    }
  }}
>
  <Button onclick={() => (inviteVisible = true)}>Invite member...</Button>

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
