<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import Select from './Select.svelte';

  const { Story } = defineMeta({
    title: 'Form/Select',
    component: Select,
    tags: ['autodocs']
  });
</script>

<script lang="ts">
  const visibility = [
    { value: 'public', label: 'Public — anyone can join' },
    { value: 'invite', label: 'Invite only' },
    { value: 'private', label: 'Private — hidden from listings' }
  ];

  const role = [
    { value: 'admin', label: 'Admin' },
    { value: 'moderator', label: 'Moderator' },
    { value: 'member', label: 'Member' },
    { value: 'guest', label: 'Guest' }
  ];

  let v1 = $state('public');
  let v2 = $state('');
  let v3 = $state('');
</script>

<Story name="Default" asChild>
  <div class="max-w-md">
    <Select id="visibility" label="Visibility" options={visibility} bind:value={v1} />
  </div>
</Story>

<Story name="With placeholder" asChild>
  <div class="max-w-md">
    <Select
      id="role"
      label="Role"
      options={role}
      bind:value={v2}
      placeholder="Select a role..."
    />
  </div>
</Story>

<Story name="With error" asChild>
  <div class="max-w-md">
    <Select
      id="role-err"
      label="Role"
      options={role}
      bind:value={v3}
      placeholder="Select a role..."
      error="Please choose a role to continue."
    />
  </div>
</Story>

<Story name="Disabled" asChild>
  <div class="max-w-md">
    <Select id="role-disabled" label="Role" options={role} value="member" disabled />
  </div>
</Story>
