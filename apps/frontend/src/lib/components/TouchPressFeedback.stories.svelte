<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { resolve } from '$app/paths';
  import { DataTable, MotdContent, SegmentedControl } from '$lib/ui';
  import { Button } from '$lib/ui/form';
  import TouchPressFeedback from './TouchPressFeedback.svelte';
  import ServerProfileCard from './ServerProfileCard.svelte';
  import MessageReplyAttribution from '../../routes/chat/[serverId]/[roomId]/MessageReplyAttribution.svelte';

  const { Story } = defineMeta({
    title: 'UI/Press feedback',
    component: TouchPressFeedback,
    parameters: { layout: 'fullscreen' }
  });
</script>

<script lang="ts">
  let mode = $state('all');
  let result = $state('Press a control to try its feedback.');
  const rows = [
    { name: 'Garden room', description: 'Plants and projects' },
    { name: 'Reading room', description: 'Books and conversations' }
  ];
</script>

<Story name="Shared surfaces" asChild>
  <TouchPressFeedback />
  <div class="mx-auto flex max-w-4xl flex-col gap-6 p-6">
    <h1 class="text-xl font-semibold">Press feedback</h1>
    <p>Try the row gaps, reply connector, card description, inline link, and segmented options.</p>
    <div class="grid gap-6 md:grid-cols-2">
      <div class="flex flex-col gap-6">
        <section class="flex flex-col gap-3">
          <h2 class="font-semibold">Complete table rows</h2>
          <DataTable items={rows} columns={2} getKey={(row) => row.name}>
            {#snippet header()}
              <th class="px-3 py-2 text-start">Room</th>
              <th class="px-3 py-2 text-start">Topic</th>
            {/snippet}
            {#snippet row(item)}
              <td class="px-3 py-3">
                <a
                  class="data-table-row-link link"
                  href={resolve('/chat/servers')}
                  onclick={(event) => {
                    event.preventDefault();
                    result = `Opened ${item.name}`;
                  }}>{item.name}</a
                >
              </td>
              <td class="px-3 py-3">{item.description}</td>
            {/snippet}
          </DataTable>
        </section>
        <section class="flex flex-col gap-3">
          <h2 class="font-semibold">Reply target</h2>
          <MessageReplyAttribution
            compact
            preview={{
              name: 'Earlier message',
              body: 'Let’s meet in the garden tomorrow.',
              actor: null,
              deleted: false,
              attachmentSummary: null
            }}
            onJump={() => (result = 'Jumped to the earlier message')}
          />
        </section>
        <section class="flex flex-col gap-3">
          <h2 class="font-semibold">Inline text and segmented controls</h2>
          <p class="max-w-64">
            Here is <a
              class="link"
              href={resolve('/chat/servers')}
              onclick={(event) => {
                event.preventDefault();
                result = 'Opened the inline link';
              }}>an inline link long enough to wrap naturally</a
            >
            beside a <span class="mention" data-press-feedback>@mention</span>.
          </p>
          <SegmentedControl
            label="Messages"
            options={[
              { value: 'all', label: 'All' },
              { value: 'unread', label: 'Unread' }
            ]}
            value={mode}
            onchange={(value) => (mode = value)}
          />
        </section>
        <section class="flex flex-col gap-3">
          <h2 class="font-semibold">Message of the day</h2>
          <MotdContent
            motd="Welcome to the garden!"
            onclick={() => (result = 'Opened the message of the day')}
          />
        </section>
      </div>
      <div>
        <ServerProfileCard
          origin="https://garden.example"
          profile={{
            name: 'The Garden',
            description:
              'A place for plants, projects, and good conversation. Press this description or the banner to move the visible card.',
            iconUrl: null,
            bannerUrl: null
          }}
          onactivate={() => (result = 'Opened The Garden')}
          actionLabel="Open The Garden"
        >
          {#snippet actions()}
            <Button variant="secondary" onclick={() => (result = 'Used the separate card action')}
              >Separate action</Button
            >
          {/snippet}
        </ServerProfileCard>
      </div>
    </div>
    <output class="text-muted">{result}</output>
  </div>
</Story>
