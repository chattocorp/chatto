<script lang="ts">
  import type { TimelineEventView } from '@chatto/client/timeline/timelineEvents';
  import { VideoProcessingStatus } from '@chatto/client/timeline/messageAttachments';
  import VideoPlayer from '$lib/components/chat/VideoPlayer.svelte';
  import audioUrl from '../../../../../e2e/fixtures/test-audio.mp3?url';
  import videoUrl from '../../../../../e2e/fixtures/test-video.mp4?url';

  let { event }: { event: TimelineEventView } = $props();
</script>

<div
  data-event-id={event.id}
  class={event.id.startsWith('msg-') || event.id.startsWith('older-') ? 'h-15' : 'h-80'}
>
  <span>{event.id}</span>
  <div data-attachment-media>
    {#if event.id.startsWith('audio-') || event.id.startsWith('multiple-')}
      <audio controls loop src={audioUrl}>{event.id}</audio>
      {#if event.id.startsWith('multiple-')}
        <audio controls loop src={audioUrl}>Second attachment</audio>
      {/if}
    {:else if event.id.startsWith('video-')}
      <video controls loop src={videoUrl} class="h-24"><track kind="captions" /></video>
    {:else if event.id.startsWith('processed-') || event.id.startsWith('gif-')}
      <VideoPlayer
        status={VideoProcessingStatus.Completed}
        filename="test-video.mp4"
        variants={[{ url: videoUrl, quality: '240p', width: 320, height: 240, size: 1024 }]}
        autoLoop={event.id.startsWith('gif-')}
      />
    {/if}
  </div>
</div>
