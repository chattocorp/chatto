<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import Select from './Select.svelte';

  const componentDescription = `
    Use Select for compact finite option sets where the user only chooses one value. Keep option
    labels plain and explicit; use helper text or surrounding copy for longer consequences.
  `.trim();

  const { Story } = defineMeta({
    title: 'Form/Select',
    component: Select,
    tags: ['autodocs'],
    parameters: {
      docs: {
        description: { component: componentDescription }
      }
    }
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
  let device = $state('device-12');
  const devices = [
    { value: '', label: 'System default' },
    ...Array.from({ length: 30 }, (_, index) => ({
      value: `device-${index}`,
      label: `Studio audio interface ${index + 1} — Virtual conference and recording microphone with a long device name`
    }))
  ];
</script>

<Story
  name="Default"
  asChild
  parameters={{
    docs: {
      description: { story: 'Default select with an existing value.' }
    }
  }}
>
  <div class="max-w-md">
    <Select id="visibility" label="Visibility" options={visibility} bind:value={v1} />
  </div>
</Story>

<Story name="Many devices and long labels" asChild>
  <div class="w-full max-w-md">
    <Select id="many-devices" label="Microphone" options={devices} bind:value={device} />
  </div>
</Story>

<Story name="Flat" asChild>
  <div class="max-w-md" style="--depth-strength: 0; --depth-width: 0">
    <Select id="flat-role" label="Role" options={role} value="member" />
  </div>
</Story>

<Story name="Kinda 3D" asChild>
  <div class="max-w-md" style="--depth-strength: 1; --depth-width: 1">
    <Select id="raised-role" label="Role" options={role} value="member" />
  </div>
</Story>

<Story name="Very 3D" asChild>
  <div class="max-w-md" style="--depth-strength: 1.75; --depth-width: 1.5">
    <Select id="very-raised-role" label="Role" options={role} value="member" />
  </div>
</Story>

<Story
  name="With placeholder"
  asChild
  parameters={{
    docs: {
      description: {
        story: 'Use placeholders when the user must actively choose from an empty state.'
      }
    }
  }}
>
  <div class="max-w-md">
    <Select id="role" label="Role" options={role} bind:value={v2} placeholder="Select a role..." />
  </div>
</Story>

<Story
  name="With error"
  asChild
  parameters={{
    docs: {
      description: { story: 'Select validation uses the same error treatment as text fields.' }
    }
  }}
>
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

<Story
  name="Disabled"
  asChild
  parameters={{
    docs: {
      description: { story: 'Disabled selects remain readable while clearly inactive.' }
    }
  }}
>
  <div class="max-w-md">
    <Select id="role-disabled" label="Role" options={role} value="member" disabled />
  </div>
</Story>
