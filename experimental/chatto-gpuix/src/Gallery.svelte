<script lang="ts">
  import PaneHeader from './ui/PaneHeader.svelte';
  import Theme from './ui/Theme.svelte';
  import Avatar from './ui/Avatar.svelte';
  import ChannelRow from './ui/ChannelRow.svelte';
  import SidebarSection from './ui/SidebarSection.svelte';
  import MessageFrame from './ui/MessageFrame.svelte';
  import Composer from './ui/Composer.svelte';
  import LinkPreview from './LinkPreview.svelte';
  import ActionButton from './ActionButton.svelte';
  import { colors, markdownTheme } from './ui/theme.ts';

  let { onClose }: { onClose?: () => void } = $props();
  let selected = $state('chatto-development');
  let draft = $state('');
  let sent = $state<string[]>([]);
  let notice = $state('');
  function send() {
    if (!draft.trim()) return;
    sent = [...sent, draft];
    draft = '';
  }
</script>

<Theme>
  <div class="gallery">
    <PaneHeader title="Chatto components" description="Native design gallery">
      {#snippet actions()}{#if onClose}<ActionButton
            label="Back"
            quiet
            onclick={onClose}
            testid="close-gallery"
          />{/if}{/snippet}
    </PaneHeader>
    <div class="columns">
      <div class="sidebar">
        <PaneHeader title="Chatto HQ" testid="gallery-server-header" />
        <SidebarSection label="CHATTO">
          <ChannelRow
            label="announcements"
            selected={selected === 'announcements'}
            onclick={() => (selected = 'announcements')}
          />
          <ChannelRow
            label="random-chat"
            selected={selected === 'random-chat'}
            onclick={() => (selected = 'random-chat')}
          />
          <ChannelRow
            label="chatto-development"
            selected={selected === 'chatto-development'}
            onclick={() => (selected = 'chatto-development')}
            testid="gallery-channel"
          />
          <ChannelRow
            label="bug-reports"
            testid="gallery-bugs"
            unread={3}
            selected={selected === 'bug-reports'}
            onclick={() => (selected = 'bug-reports')}
          />
        </SidebarSection>
        <SidebarSection label="DIRECT MESSAGES">
          <ChannelRow
            label="Ada"
            direct
            selected={selected === 'Ada'}
            onclick={() => (selected = 'Ada')}
          />
        </SidebarSection>
        <div class="sidebar-spacer"></div>
        <div class="identity"><Avatar name="Sam Chen" size={30} /><text>Sam Chen</text></div>
      </div>
      <div class="main">
        <PaneHeader
          title={selected}
          icon={selected === 'Ada' ? 'message' : 'hash'}
          testid="gallery-channel-header"
        />
        <div class="examples">
          <MessageFrame author="Ada" time="10:30">
            <markdown
              source="The native client is starting to feel like **Chatto**. Compact channels, familiar colors, and room for the conversation."
              theme={markdownTheme()}
              class="body"
            ></markdown>
          </MessageFrame>
          <MessageFrame author="Chatto Bot" time="10:32" bot>
            <markdown
              source="Messages, previews, and attachments share the same visual language. The client still uses `@chatto/client`."
              theme={markdownTheme()}
              class="body"
            ></markdown>
            <LinkPreview
              preview={{
                url: 'https://chatto.run',
                siteName: 'Chatto',
                title: 'Your conversations, together.',
                description:
                  'A link card with the same surface, spacing, and controls as an attachment.'
              }}
              serverUrl="https://example.test"
              active={false}
            />
          </MessageFrame>
          {#each sent as body, index (index)}
            <MessageFrame author="You (gallery)" time="Now"
              ><text class="body">{body}</text></MessageFrame
            >
          {/each}
          <div class="controls">
            <text class="section-title">Controls</text>
            <div class="button-row">
              <ActionButton
                label="Primary"
                primary
                onclick={() => (notice = 'Primary action selected')}
                testid="gallery-primary"
              />
              <ActionButton label="Default" onclick={() => (notice = 'Default action selected')} />
              <ActionButton
                label="Quiet"
                quiet
                onclick={() => (notice = 'Quiet action selected')}
              />
              <ActionButton label="Disabled" disabled onclick={() => {}} />
              <ActionButton
                label="Search"
                icon="search"
                iconOnly
                quiet
                onclick={() => (notice = 'Search action selected')}
              />
            </div>
            {#if notice}<text class="muted">{notice}</text>{/if}
            <text class="section-title">Palette</text>
            <div class="swatches">
              {#each ['background', 'surface', 'border', 'text', 'muted', 'action'] as key (key)}
                {@const color = colors[key as keyof typeof colors]}
                <div class="swatch">
                  <div
                    style={`width: var(--size-28); height: var(--size-20); border-radius: var(--size-4); background-color: ${color};`}
                  ></div>
                  <text class="muted">{key}</text>
                </div>
              {/each}
            </div>
          </div>
        </div>
        <Composer
          {draft}
          sending={false}
          canSend={!!draft.trim()}
          onChange={(value) => (draft = value)}
          onSend={send}
        />
      </div>
    </div>
  </div>
</Theme>

<style>
  .gallery {
    display: flex;
    flex-direction: column;
    width: 100%;
    height: 100%;
  }
  .muted {
    font-size: var(--text-small);
    color: var(--color-muted);
  }
  .columns {
    display: flex;
    flex-direction: row;
    flex-grow: 1;
    min-height: 0;
  }
  .sidebar {
    display: flex;
    flex-direction: column;
    width: var(--size-238);
    flex-shrink: 0;
    border-right-width: var(--size-1);
    border-color: var(--color-border);
  }
  .sidebar-spacer {
    flex-grow: 1;
  }
  .identity {
    display: flex;
    align-items: center;
    gap: var(--size-9);
    margin: var(--size-8);
    padding: var(--size-8);
    background-color: var(--color-surface);
    border-radius: var(--radius-panel);
    font-size: var(--text-body);
  }
  .main {
    display: flex;
    flex-direction: column;
    flex-grow: 1;
    min-width: 0;
  }
  .examples {
    flex-grow: 1;
    min-height: 0;
    overflow-y: scroll;
  }
  .body {
    font-size: var(--text-body);
    color: var(--color-text);
    white-space: normal;
  }
  .controls {
    display: flex;
    flex-direction: column;
    gap: var(--size-12);
    padding: var(--size-16) var(--size-18);
    margin-left: var(--size-49);
  }
  .section-title {
    font-size: var(--text-body);
    font-weight: 600;
  }
  .button-row {
    display: flex;
    flex-direction: row;
    align-items: center;
    gap: var(--size-8);
  }
  .swatches {
    display: flex;
    flex-direction: row;
    gap: var(--size-12);
  }
  .swatch {
    display: flex;
    flex-direction: column;
    gap: var(--size-5);
  }
</style>
