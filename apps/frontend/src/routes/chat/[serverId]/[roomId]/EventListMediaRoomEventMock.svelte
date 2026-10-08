<script lang="ts">
  import type { TimelineEventView } from '@chatto/client/timeline/timelineEvents';
  import {
    VideoProcessingStatus,
    type MessageAttachmentView
  } from '@chatto/client/timeline/messageAttachments';
  import MessageAttachments from './MessageAttachments.svelte';
  import audioUrl from '../../../../../e2e/fixtures/test-audio.mp3?url';
  import videoUrl from '../../../../../e2e/fixtures/test-video.mp4?url';

  let {
    event,
    onPlaybackChange
  }: { event: TimelineEventView; onPlaybackChange?: (active: boolean) => void } = $props();
  let attachmentsRemoved = $state(false);

  function fixtureAttachments(id: string): MessageAttachmentView[] {
    const audio = id.startsWith('audio-') || id.startsWith('multiple-');
    const processed = id.startsWith('processed-') || id.startsWith('gif-');
    if (!audio && !processed && !id.startsWith('video-')) return [];
    const assetUrl = { url: audio ? audioUrl : videoUrl, expiresAt: '2099-01-01T00:00:00Z' };
    const attachment: MessageAttachmentView = {
      id: 'first',
      filename: audio ? 'audio.mp3' : 'video.mp4',
      contentType: audio ? 'audio/mpeg' : id.startsWith('gif-') ? 'image/gif' : 'video/mp4',
      width: 320,
      height: 240,
      assetUrl,
      videoProcessing: processed
        ? {
            status: VideoProcessingStatus.Completed,
            sourceAvailable: true,
            variants: [{ assetUrl, quality: '240p', width: 320, height: 240, size: 1024 }]
          }
        : null
    };
    return id.startsWith('multiple-')
      ? [attachment, { ...attachment, id: 'second' }]
      : [attachment];
  }
</script>

<div
  data-event-id={event.id}
  class={event.id.startsWith('msg-') || event.id.startsWith('older-') ? 'h-15' : 'h-80'}
>
  <span>{event.id}</span>
  <MessageAttachments
    attachments={attachmentsRemoved ? [] : fixtureAttachments(event.id)}
    serverId="server-1"
    roomId="room-1"
    eventId={event.id}
    {onPlaybackChange}
  />
  <button hidden data-testid="remove-attachments" onclick={() => (attachmentsRemoved = true)}>
    Remove attachments
  </button>
</div>
