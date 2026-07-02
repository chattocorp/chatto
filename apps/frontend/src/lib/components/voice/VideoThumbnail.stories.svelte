<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import VideoThumbnail from './VideoThumbnail.svelte';
  import { PresenceStatus } from '$lib/render/types';
  import type { Track } from 'livekit-client';

  const { Story } = defineMeta({
    title: 'Voice/Video thumbnail',
    component: VideoThumbnail,
    tags: ['autodocs'],
    parameters: {
      docs: {
        description: {
          component:
            'LiveKit video track thumbnail used by camera and screen-share call tiles.'
        }
      }
    }
  });

  const user = {
    id: 'alice',
    login: 'alice',
    displayName: 'Alice',
    avatarUrl: null,
    presenceStatus: PresenceStatus.Online
  } as const;

  function posterTrack(svg: string): Track {
    const poster = `data:image/svg+xml;charset=utf-8,${encodeURIComponent(svg)}`;
    return {
      attach(element: HTMLVideoElement) {
        element.poster = poster;
        return element;
      },
      detach(element: HTMLVideoElement) {
        element.removeAttribute('poster');
        return element;
      }
    } as unknown as Track;
  }

  const cameraTrack = posterTrack(`
    <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1600 900">
      <defs>
        <linearGradient id="g" x1="0" x2="1" y1="0" y2="1">
          <stop stop-color="#f6efe2"/>
          <stop offset="1" stop-color="#8d826f"/>
        </linearGradient>
      </defs>
      <rect width="1600" height="900" fill="url(#g)"/>
      <circle cx="740" cy="420" r="150" fill="#2d2d2d"/>
      <rect x="600" y="570" width="460" height="220" rx="70" fill="#343434"/>
      <path d="M0 0h460L210 520H0z" fill="#fff" opacity=".42"/>
    </svg>
  `);

  const screenTrack = posterTrack(`
    <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1600 1000">
      <rect width="1600" height="1000" fill="#b86600"/>
      <path d="M-20 720C420 630 760 400 1120-20" stroke="#ffbe2e" stroke-width="110" fill="none"/>
      <path d="M1010-40c-80 390-40 720 180 1080" stroke="#f9a915" stroke-width="70" fill="none"/>
      <rect x="16" y="14" width="1568" height="34" fill="#5a2a00" opacity=".75"/>
    </svg>
  `);
</script>

<Story name="Camera cover" asChild>
  <div class="w-96 rounded-md border border-border bg-surface-100 p-1.5">
    <VideoThumbnail track={cameraTrack} name="Alice" {user} showIdentityOverlay={false} />
  </div>
</Story>

<Story name="Screen share contain" asChild>
  <div class="w-[720px] rounded-md border border-border bg-surface-100 p-1.5">
    <VideoThumbnail
      track={screenTrack}
      name="Alice's screen"
      {user}
      showIdentityOverlay={false}
      fit="contain"
    />
  </div>
</Story>
