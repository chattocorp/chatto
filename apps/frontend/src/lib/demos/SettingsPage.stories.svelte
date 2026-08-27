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
  import { Panel } from '$lib/components/admin';
  import FormSection from '$lib/ui/FormSection.svelte';
  import Hint from '$lib/ui/Hint.svelte';
  import PaneContent from '$lib/ui/PaneContent.svelte';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
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
  <div class="pane-page h-[48rem] bg-background">
    <PaneHeader
      title="Space settings"
      subtitle="Configure how this space appears and who can join."
    />

    <PaneContent>
      <div class="flex flex-col gap-6">
        <Hint tone="info" icon="icon-[uil--info-circle]">
          Changes here apply immediately to all members of this space.
        </Hint>

        <form onsubmit={handleSubmit}>
          <Panel title="Space details" icon="iconify icon-[uil--edit]">
            <div class="flex max-w-2xl flex-col gap-6">
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

              <FormSection title="Access" bordered>
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

              <FormSection title="Retention" bordered>
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

              <div class="flex justify-end gap-2 border-t border-border pt-5">
                <Button type="button" variant="secondary">Cancel</Button>
                <Button type="submit" loading={saving} loadingText="Saving...">Save changes</Button>
              </div>
            </div>
          </Panel>
        </form>

        <Panel title="Danger zone" icon="iconify icon-[uil--exclamation-triangle]">
          <Button variant="danger">Delete space</Button>
        </Panel>
      </div>
    </PaneContent>
  </div>
</Story>
