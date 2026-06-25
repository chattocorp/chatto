<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import FormSection from './FormSection.svelte';

  const componentDescription = `
    Use FormSection to group related fields inside settings pages and modal-like forms. Prefer
    vertical rhythm over nested cards, and use the bordered option only when adjacent sections need
    stronger separation.
  `.trim();

  const { Story } = defineMeta({
    title: 'UI/FormSection',
    component: FormSection,
    tags: ['autodocs'],
    parameters: {
      docs: {
        description: { component: componentDescription }
      }
    }
  });
</script>

<script lang="ts">
  import { Checkbox, Form, TextArea, TextInput } from './form';

  let name = $state('Open Source Hangout');
  let description = $state('A friendly community for open source contributors.');
  let allowGuests = $state(true);
  let requireApproval = $state(false);
</script>

<Story name="Single section" asChild>
  <div class="bg-background p-6">
    <Form onsubmit={() => {}} maxWidth="max-w-xl">
      <FormSection title="General">
        <div class="flex flex-col gap-4">
          <TextInput id="fs-name" label="Space name" bind:value={name} required />
          <TextArea id="fs-description" label="Description" bind:value={description} rows={2} />
        </div>
      </FormSection>
    </Form>
  </div>
</Story>

<Story name="Multiple sections" asChild>
  <div class="bg-background p-6">
    <Form onsubmit={() => {}} maxWidth="max-w-xl" spacing="spacious">
      <FormSection title="General">
        <div class="flex flex-col gap-4">
          <TextInput id="fs2-name" label="Space name" bind:value={name} required />
          <TextArea id="fs2-description" label="Description" bind:value={description} rows={2} />
        </div>
      </FormSection>

      <FormSection title="Access" bordered>
        <div class="flex flex-col gap-3">
          <Checkbox id="fs2-guests" label="Allow guest accounts" bind:checked={allowGuests} />
          <Checkbox
            id="fs2-approval"
            label="Require admin approval to join"
            bind:checked={requireApproval}
          />
        </div>
      </FormSection>
    </Form>
  </div>
</Story>
