<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import CenteredPage from './CenteredPage.svelte';
  import PaneHeader from './PaneHeader.svelte';
  import Panel from './Panel.svelte';
  import { Button, TextInput } from './form';

  const { Story } = defineMeta({
    title: 'UI/CenteredPage',
    component: CenteredPage,
    tags: ['autodocs']
  });
</script>

<Story name="Introduction" asChild>
  <div class="flex h-[36rem] w-full bg-background">
    <CenteredPage>
      <div class="flex flex-col items-center gap-6 text-center">
        <h1 class="text-4xl font-semibold text-text-top">Welcome to your server</h1>
        <p class="text-xl text-muted">Give your community a place to talk.</p>
        <Button>Set up your server</Button>
      </div>
    </CenteredPage>
  </div>
</Story>

<Story name="Form with fixed navigation" asChild>
  <div class="flex h-[40rem] w-full bg-background">
    <CenteredPage>
      {#snippet header()}<PaneHeader title="Set up your server" />{/snippet}
      <Panel title="Server details">
        <div class="flex flex-col gap-6">
          <TextInput id="centered-server-name" label="Server name" />
          <div class="flex justify-end"><Button>Continue</Button></div>
        </div>
      </Panel>
    </CenteredPage>
  </div>
</Story>

<Story name="Tall content in a narrow viewport" asChild>
  <div class="flex h-80 w-full max-w-sm bg-background">
    <CenteredPage>
      {#snippet header()}<PaneHeader title="Set up your server" />{/snippet}
      <Panel title="Account details">
        <div class="flex flex-col gap-6">
          <TextInput id="centered-username" label="Username" />
          <TextInput id="centered-display-name" label="Display name" />
          <TextInput id="centered-password" label="Password" type="password" />
          <TextInput id="centered-confirmation" label="Confirm password" type="password" />
          <Button>Finish setup</Button>
        </div>
      </Panel>
    </CenteredPage>
  </div>
</Story>
