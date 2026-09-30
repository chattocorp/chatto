<!--
@component

Renders a LiveKit video track in a thumbnail-sized `<video>` element.
It can optionally include a small avatar overlay in the top-left corner for
identification.

Manages the attach/detach lifecycle imperatively — only detaches/reattaches
when the track reference actually changes, not on every parent re-render.
This prevents flicker from the 60ms audio level polling in VoiceCallPanel.

The explicit width/height attributes tell LiveKit's `adaptiveStream` what
resolution to request for sidebar-width tiles.

**Props:**
- `track` - The LiveKit video Track to display
- `name` - Participant display name (shown as tooltip)
- `user` - User object for the avatar overlay (same shape as UserAvatar's `user` prop)
- `showIdentityOverlay` - Whether to show the avatar overlay
- `fit` - How the video track should fit the tile. Camera thumbnails default to `cover`; screen shares should use `contain` to avoid cropping shared content.
- `fill` - Whether the video should fill its parent's height instead of using thumbnail aspect-ratio sizing.
-->
<script lang="ts">
  import { formatAccountName } from '@chatto/client/timeline/accountName';
  import { on } from 'svelte/events';
  import type { Track } from 'livekit-client';
  import type { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
  import UserAvatar from '$lib/components/UserAvatar.svelte';
  import { registerCallVideo, releaseCallVideo } from '$lib/state/callPictureInPicture';

  let {
    track,
    name,
    user,
    showIdentityOverlay = true,
    fit = 'cover',
    fill = false
  }: {
    track: Track;
    name: string;
    user: {
      id: string;
      login: string;
      displayName: string;
      isBot?: boolean;
      deleted?: boolean;
      avatarUrl: string | null;
      presenceStatus: PresenceStatus;
    };
    showIdentityOverlay?: boolean;
    fit?: 'cover' | 'contain';
    fill?: boolean;
  } = $props();

  /** Keep native mouse menus while the card still owns touch long-press gestures. */
  function nativeVideoMenu(element: HTMLVideoElement) {
    let fromTouch = false;
    const cleanups = [
      on(element, 'pointerdown', (event) => {
        fromTouch = event.pointerType === 'touch';
      }),
      on(element, 'contextmenu', (event) => {
        if (fromTouch || (event instanceof PointerEvent && event.pointerType === 'touch')) return;
        event.stopPropagation();
      })
    ];
    return () => {
      for (const cleanup of cleanups) cleanup();
    };
  }

  /** Give each track its own element so a replacement cannot overwrite retained PiP media. */
  function attachVideo(element: HTMLVideoElement) {
    const attachedTrack = track;
    registerCallVideo(attachedTrack, element);
    attachedTrack.attach(element);
    return () => releaseCallVideo(attachedTrack, element);
  }
</script>

<div
  class={[
    'relative block w-full overflow-hidden rounded-md',
    fit === 'contain' ? 'bg-black' : 'bg-surface-emphasized',
    fill ? 'h-full min-h-0' : 'aspect-video'
  ]}
>
  {#key track}
    <video
      {@attach attachVideo}
      {@attach nativeVideoMenu}
      width="640"
      height="360"
      class={['h-full w-full', fit === 'contain' ? 'object-contain' : 'object-cover']}
      title={formatAccountName(name, user)}
      autoplay
      playsinline
      muted
    ></video>
  {/key}
  {#if showIdentityOverlay}
    <div class="absolute start-2 top-2 h-6 w-6 rounded-full ring-[1.5px] ring-surface">
      <UserAvatar {user} size="xs" />
    </div>
  {/if}
</div>
