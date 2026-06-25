<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';

  const { Story } = defineMeta({
    title: 'Demos/Settings page',
    parameters: {
      layout: 'fullscreen'
    }
  });
</script>

<script lang="ts">
  import FormSection from '$lib/ui/FormSection.svelte';
  import Hint from '$lib/ui/Hint.svelte';
  import Divider from '$lib/ui/Divider.svelte';
  import { TextInput, TextArea, Select, Checkbox, Button } from '$lib/ui/form';

  let name = $state('Open Source Hangout');
  let description = $state(
    'A friendly community for people who hack on open source projects in their spare time.'
  );
  let visibility = $state('public');
  let allowGuests = $state(true);
  let messageRetention = $state('forever');
  let saving = $state(false);

  function handleSubmit(e: SubmitEvent) {
    e.preventDefault();
    saving = true;
    setTimeout(() => (saving = false), 1200);
  }
</script>

<Story name="Space settings" asChild>
  <div class="mx-auto max-w-2xl p-6">
    <header class="mb-6">
      <h1 class="text-2xl font-bold">Space settings</h1>
      <p class="text-sm text-muted">Configure how this space appears and who can join.</p>
    </header>

    <Hint tone="info" icon="uil--info-circle">
      Changes here apply immediately to all members of this space.
    </Hint>

    <form class="mt-6 flex flex-col gap-6" onsubmit={handleSubmit}>
      <FormSection title="General">
        <div class="flex flex-col gap-4">
          <TextInput
            id="space-name"
            label="Space name"
            bind:value={name}
            required
            description="Shown in the sidebar and on the discovery page."
          />
          <TextArea
            id="space-description"
            label="Description"
            bind:value={description}
            rows={3}
            maxlength={200}
            description="A short summary shown on the space card."
          />
        </div>
      </FormSection>

      <Divider />

      <FormSection title="Access">
        <div class="flex flex-col gap-4">
          <Select
            id="visibility"
            label="Visibility"
            bind:value={visibility}
            options={[
              { value: 'public', label: 'Public — anyone can join' },
              { value: 'invite', label: 'Invite only' },
              { value: 'private', label: 'Private — hidden from listings' }
            ]}
          />
          <Checkbox
            id="allow-guests"
            bind:checked={allowGuests}
            label="Allow unauthenticated guests to read public rooms"
            description="Guests can read but not post."
          />
        </div>
      </FormSection>

      <Divider />

      <FormSection title="Retention">
        <Select
          id="retention"
          label="Message retention"
          bind:value={messageRetention}
          options={[
            { value: 'forever', label: 'Keep forever' },
            { value: '90', label: 'Delete after 90 days' },
            { value: '30', label: 'Delete after 30 days' },
            { value: '7', label: 'Delete after 7 days' }
          ]}
          description="Older messages are removed automatically. Affects all rooms."
        />
      </FormSection>

      <Divider />

      <div class="flex items-center justify-between">
        <Button variant="danger">Delete space</Button>
        <div class="flex gap-2">
          <Button variant="secondary">Cancel</Button>
          <Button type="submit" loading={saving} loadingText="Saving...">Save changes</Button>
        </div>
      </div>
    </form>
  </div>
</Story>
