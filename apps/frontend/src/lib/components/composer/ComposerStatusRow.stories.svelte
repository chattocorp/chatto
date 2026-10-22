<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { expect, waitFor } from 'storybook/test';
  import ComposerStatusRow from './ComposerStatusRow.svelte';
  import ComposerStatusRowMock from './ComposerStatusRowMock.svelte';
  const { Story } = defineMeta({
    title: 'Composer/Status badges',
    component: ComposerStatusRow,
    play: async ({ canvas }) => {
      const typing = canvas.queryByTestId('typing-indicator');
      // Check contrast after the badge's entrance transition reaches full opacity.
      if (typing) await waitFor(() => expect(getComputedStyle(typing).opacity).toBe('1'));
    }
  });
</script>

<Story name="Ready with typing" asChild><ComposerStatusRowMock /></Story>
<Story name="Cooldown with typing" asChild><ComposerStatusRowMock remaining={23} /></Story>
<Story name="Exempt" asChild><ComposerStatusRowMock remaining={23} bypassed /></Story>
<Story name="Typing alone" asChild><ComposerStatusRowMock seconds={0} /></Story>
<Story name="Slow mode alone" asChild><ComposerStatusRowMock remaining={23} typing={false} /></Story
>
<Story name="Disabled" asChild><ComposerStatusRowMock seconds={0} typing={false} /></Story>
<Story name="Right to left" asChild><ComposerStatusRowMock remaining={23} direction="rtl" /></Story>
