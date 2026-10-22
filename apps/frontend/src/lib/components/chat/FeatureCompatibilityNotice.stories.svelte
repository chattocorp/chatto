<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { limitedServerFeatures } from '@chatto/client/server/compatibility';
  import FeatureCompatibilityNotice from './FeatureCompatibilityNotice.svelte';
  import { PaneHeader, SegmentedControl } from '$lib/ui';

  const { Story } = defineMeta({
    title: 'Chat/Feature compatibility',
    component: FeatureCompatibilityNotice,
    args: { serverSegment: 'chat.example.test' },
    tags: ['autodocs']
  });
  const features = limitedServerFeatures('0.5.0-beta.11').filter(
    (feature) => feature === 'threadViews'
  );
</script>

<Story name="Older server threads" asChild>
  <div class="max-w-3xl bg-background text-text">
    <PaneHeader title="Threads" subtitle="Example community · Server 0.5.0-beta.11" />
    <div class="flex flex-col gap-3 p-4">
      <FeatureCompatibilityNotice {features} serverSegment="chat.example.test" />
      <div class="flex justify-end">
        <SegmentedControl
          label="Thread view"
          options={[
            { value: 'followed', label: 'Followed' },
            { value: 'mine', label: 'Started by you' }
          ]}
          value="mine"
          onchange={() => {}}
        />
      </div>
      <p class="p-4 text-muted">Thread results remain available on this server.</p>
    </div>
  </div>
</Story>

<Story name="Supported server" args={{ features: limitedServerFeatures('0.5.0-beta.18') }} />
<Story name="Unknown version" args={{ features: limitedServerFeatures('dev') }} />
