<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import FormSection from '$lib/ui/FormSection.svelte';
  import Form from './Form.svelte';

  const componentDescription = `
    Use Form for standalone, non-modal forms in pages, panels, and auth flows. It owns vertical
    rhythm, optional max-width, form-level error placement, and footer alignment without bringing
    in Dialog or FormDialog.
  `.trim();

  const { Story } = defineMeta({
    title: 'Form/Form',
    component: Form,
    tags: ['autodocs'],
    parameters: {
      docs: {
        description: { component: componentDescription }
      }
    }
  });
</script>

<script lang="ts">
  import { Button, Checkbox, Select, TextArea, TextInput } from '$lib/ui/form';

  const visibilityOptions = [
    { value: 'public', label: 'Public - anyone can join' },
    { value: 'invite', label: 'Invite only' },
    { value: 'private', label: 'Private - hidden from listings' }
  ];

  let name = $state('Open Source Hangout');
  let description = $state('A friendly community for open source contributors.');
  let visibility = $state('public');
  let allowGuests = $state(true);
  let requireApproval = $state(false);
  let email = $state('');
  let password = $state('');
  let hasError = $state(false);

  function noopSubmit() {}
</script>

<Story
  name="Settings section"
  asChild
  parameters={{
    docs: {
      description: {
        story:
          'Standalone settings forms use Form for the outer rhythm, FormSection for section headings, and edge-aligned labels/fields.'
      }
    }
  }}
>
  <div class="bg-background p-6">
    <Form onsubmit={noopSubmit} maxWidth="max-w-xl">
      <FormSection title="General">
        <div class="flex flex-col gap-4">
          <TextInput id="form-name" label="Space name" bind:value={name} required />
          <TextArea id="form-description" label="Description" bind:value={description} rows={3} />
        </div>
      </FormSection>

      {#snippet footer()}
        <Button type="submit" disabled={!name.trim()}>
          <span class="iconify uil--check"></span>
          Save changes
        </Button>
        <Button type="button" variant="secondary">Cancel</Button>
      {/snippet}
    </Form>
  </div>
</Story>

<Story
  name="Multiple sections"
  asChild
  parameters={{
    docs: {
      description: {
        story:
          'Use bordered FormSection instances for follow-on groups when the form needs visible section separation.'
      }
    }
  }}
>
  <div class="bg-background p-6">
    <Form onsubmit={noopSubmit} maxWidth="max-w-lg" error={hasError ? 'Save failed.' : undefined}>
      <FormSection title="General">
        <div class="flex flex-col gap-4">
          <TextInput id="form2-name" label="Space name" bind:value={name} required />
          <Select
            id="form2-visibility"
            label="Visibility"
            options={visibilityOptions}
            bind:value={visibility}
          />
        </div>
      </FormSection>

      <FormSection title="Access" bordered>
        <div class="flex flex-col gap-3">
          <Checkbox id="form2-guests" label="Allow guest accounts" bind:checked={allowGuests} />
          <Checkbox
            id="form2-approval"
            label="Require admin approval to join"
            bind:checked={requireApproval}
          />
        </div>
      </FormSection>

      {#snippet footer()}
        <Button type="button" variant="ghost" onclick={() => (hasError = !hasError)}>
          Toggle error
        </Button>
        <Button type="submit">Save</Button>
      {/snippet}
    </Form>
  </div>
</Story>

<Story
  name="Auth stack"
  asChild
  parameters={{
    docs: {
      description: {
        story: 'Auth forms can use Form without sections when the form is a single compact stack.'
      }
    }
  }}
>
  <div class="max-w-sm bg-background p-6">
    <Form onsubmit={noopSubmit}>
      <TextInput
        id="form-auth-email"
        label="Email"
        type="email"
        bind:value={email}
        autocomplete="email"
        required
      />
      <TextInput
        id="form-auth-password"
        label="Password"
        type="password"
        bind:value={password}
        autocomplete="current-password"
        required
      />
      <Button type="submit" size="lg" fullWidth disabled={!email || !password}>
        <span class="iconify mdi--login"></span>
        Sign in
      </Button>
    </Form>
  </div>
</Story>
