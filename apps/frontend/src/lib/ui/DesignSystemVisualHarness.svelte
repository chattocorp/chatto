<script lang="ts">
  import Panel from '$lib/components/admin/Panel.svelte';
  import Button from '$lib/ui/form/Button.svelte';
  import TextInput from '$lib/ui/form/TextInput.svelte';
  import ChoiceRow from './ChoiceRow.svelte';
  import Hint from './Hint.svelte';
  import Pill from './Pill.svelte';

  let displayName = $state('Ada Lovelace');
</script>

<section
  data-testid="design-system-visual-harness"
  class="w-[343px] bg-background p-4 text-text sm:w-180 sm:p-6"
>
  <div class="mb-5 flex items-start justify-between gap-4">
    <div>
      <Pill tone="server">Server settings</Pill>
      <h1 class="mt-2 text-2xl font-black text-text-top">Design system checkpoint</h1>
      <p class="mt-1 text-sm text-muted">Stable primitives across themes and viewports.</p>
    </div>
    <Pill tone="success">Healthy</Pill>
  </div>

  <Panel title="Profile" subtitle="How other members see you">
    <div class="grid gap-5 sm:grid-cols-[minmax(0,1fr)_minmax(0,0.9fr)]">
      <div class="flex flex-col gap-4">
        <TextInput
          id="visual-display-name"
          label="Display name"
          description="Use the name people know you by."
          bind:value={displayName}
        />

        <Hint tone="info">Changes to your profile are visible immediately.</Hint>

        <div class="flex flex-wrap gap-2">
          <Button>Save changes</Button>
          <Button variant="secondary">Cancel</Button>
          <Button variant="danger-secondary">Delete</Button>
        </div>
      </div>

      <div class="flex flex-col gap-2" role="radiogroup" aria-label="Presence visibility">
        <ChoiceRow
          label="Everyone"
          description="Show presence to all members."
          selected
        />
        <ChoiceRow
          label="Contacts only"
          description="Limit presence to people you know."
        />
        <ChoiceRow label="Nobody" description="Always appear offline." disabled />
      </div>
    </div>
  </Panel>
</section>
