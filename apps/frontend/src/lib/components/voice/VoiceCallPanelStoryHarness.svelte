<script lang="ts">
  import { onMount } from 'svelte';
  import { RoomWithViewerState } from '@chatto/api-types/api/v1/room_directory_pb';
  import type { Component } from 'svelte';
  import type { Track } from 'livekit-client';
  import type { CallParticipantInfo } from '$lib/state/server/voiceCall.svelte';
  import { provideUserProfiles } from '$lib/state/userProfiles.svelte';
  import { serverRegistry, serverConnectionManager } from '$lib/client';
  import { type RegisteredServer } from '@chatto/client/server/registry';
  import { provideServerScope } from '$lib/state/server/scope.svelte';
  import { serverUi } from '$lib/state/server/serverUi';

  type VoiceCallPanelProps = {
    roomId: string;
    livekitUrl: string;
    layout?: 'sidebar' | 'stage';
  };

  let {
    layout = 'stage',
    scenario = 'screen',
    animateVoice = false,
    initiallyMuted = false,
    playableMedia = false
  }: {
    layout?: 'sidebar' | 'stage';
    scenario?: 'screen' | 'screen-voice' | 'screen-single-secondary' | 'camera' | 'voice' | 'idle';
    animateVoice?: boolean;
    initiallyMuted?: boolean;
    /** Use a local canvas stream to exercise native media controls without a call server. */
    playableMedia?: boolean;
  } = $props();

  const roomId = 'storybook-call-room';
  const storybookServerId = 'storybook-call-server';
  provideUserProfiles();
  const getScopedServerId = () => serverRegistry.originServer?.id ?? storybookServerId;
  provideServerScope({
    get serverId() {
      return getScopedServerId();
    },
    get connection() {
      return serverConnectionManager.getClient(getScopedServerId());
    },
    get store() {
      return serverRegistry.getStore(getScopedServerId());
    },
    isCurrent: () => true
  });
  let Panel = $state<Component<VoiceCallPanelProps> | null>(null);
  let panelVisible = $state(true);

  function posterTrack(svg: string): Track {
    const poster = `data:image/svg+xml;charset=utf-8,${encodeURIComponent(svg)}`;
    const cleanups = new WeakMap<HTMLVideoElement, () => void>();
    return {
      attach(element: HTMLVideoElement) {
        element.poster = poster;
        if (playableMedia) {
          const canvas = document.createElement('canvas');
          canvas.width = 640;
          canvas.height = 360;
          const context = canvas.getContext('2d')!;
          let frame = 0;
          const draw = () => {
            context.fillStyle = '#293f50';
            context.fillRect(0, 0, canvas.width, canvas.height);
            context.fillStyle = '#ffbe2e';
            context.fillRect((frame++ * 4) % canvas.width, 250, 80, 24);
            context.fillStyle = '#ffffff';
            context.font = '28px sans-serif';
            context.fillText('Picture-in-picture preview', 40, 100);
          };
          draw();
          const stream = canvas.captureStream(10);
          element.srcObject = stream;
          const timer = window.setInterval(draw, 100);
          cleanups.set(element, () => {
            window.clearInterval(timer);
            for (const track of stream.getTracks()) track.stop();
            element.srcObject = null;
          });
        }
        return element;
      },
      detach(element: HTMLVideoElement) {
        cleanups.get(element)?.();
        cleanups.delete(element);
        element.removeAttribute('poster');
        return element;
      }
    } as unknown as Track;
  }

  const screenTrack = posterTrack(`
		<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1600 1000">
			<rect width="1600" height="1000" fill="#b86600"/>
			<path d="M-20 720C420 630 760 400 1120-20" stroke="#ffbe2e" stroke-width="110" fill="none"/>
			<path d="M1010-40c-80 390-40 720 180 1080" stroke="#f9a915" stroke-width="70" fill="none"/>
			<rect x="16" y="14" width="1568" height="34" fill="#5a2a00" opacity=".75"/>
			<rect x="120" y="160" width="520" height="300" rx="18" fill="#fff" opacity=".76"/>
			<rect x="730" y="160" width="740" height="680" rx="18" fill="#fff" opacity=".42"/>
		</svg>
	`);

  const cameraTrack = posterTrack(`
		<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1600 900">
			<defs>
				<linearGradient id="g" x1="0" x2="1" y1="0" y2="1">
					<stop stop-color="#dbc8ac"/>
					<stop offset="1" stop-color="#5b625c"/>
				</linearGradient>
			</defs>
			<rect width="1600" height="900" fill="url(#g)"/>
			<circle cx="760" cy="385" r="145" fill="#292929"/>
			<rect x="580" y="540" width="460" height="220" rx="70" fill="#343434"/>
			<path d="M0 0h460L210 520H0z" fill="#fff" opacity=".32"/>
		</svg>
	`);

  function participant(
    identity: string,
    name: string,
    overrides: Partial<CallParticipantInfo> = {}
  ): CallParticipantInfo {
    return {
      identity,
      name,
      login: identity,
      avatarUrl: null,
      isBot: false,
      isMuted: false,
      isLocal: false,
      connectionQuality: 'excellent',
      isCameraEnabled: false,
      videoTrack: null,
      isScreenShareEnabled: false,
      screenShareTrack: null,
      isLocallyMuted: false,
      ...overrides
    };
  }

  function participantsForScenario(): CallParticipantInfo[] {
    const viewer = participant('viewer', 'Alice', {
      isLocal: true,
      isCameraEnabled: scenario !== 'voice',
      videoTrack: scenario !== 'voice' ? cameraTrack : null
    });
    const bob = participant('bob', 'Bob', {
      isCameraEnabled: scenario === 'screen',
      videoTrack: scenario === 'screen' ? cameraTrack : null,
      isLocallyMuted: true
    });
    const chloe = participant('chloe', 'Chloe', {
      isMuted: true,
      connectionQuality: 'poor'
    });

    if (scenario === 'screen-voice') {
      return [
        participant('viewer', 'Alice with a longer display name', { isLocal: true }),
        participant('bob', 'Bob', { isScreenShareEnabled: true, screenShareTrack: screenTrack })
      ];
    }

    if (scenario === 'screen-single-secondary') {
      return [
        participant('viewer', 'Alice', {
          isLocal: true,
          isCameraEnabled: true,
          videoTrack: cameraTrack,
          isScreenShareEnabled: true,
          screenShareTrack: screenTrack
        })
      ];
    }

    if (scenario === 'screen') {
      return [
        participant('dana', 'Dana', {
          isScreenShareEnabled: true,
          screenShareTrack: screenTrack
        }),
        viewer,
        bob,
        chloe
      ];
    }

    if (scenario === 'camera') {
      return [viewer, bob, chloe];
    }

    return [participant('viewer', 'Alice', { isLocal: true }), bob, chloe];
  }

  function ensureStorybookServer(): RegisteredServer {
    const origin = typeof window === 'undefined' ? 'http://localhost' : window.location.origin;
    const existingOrigin = serverRegistry.originServer;
    if (existingOrigin) return existingOrigin;

    const server: RegisteredServer = {
      id: storybookServerId,
      url: origin,
      name: 'Storybook',
      iconUrl: null,
      token: null,
      userId: 'viewer',
      userLogin: 'alice',
      userDisplayName: 'Alice',
      userAvatarUrl: null,
      reauthRequiredAt: null,
      addedAt: Date.now()
    };
    serverRegistry.addServer(
      {
        id: server.id,
        url: server.url,
        name: server.name,
        iconUrl: server.iconUrl,
        addedAt: server.addedAt
      },
      {
        token: server.token,
        userId: server.userId,
        userLogin: server.userLogin,
        userDisplayName: server.userDisplayName,
        userAvatarUrl: server.userAvatarUrl,
        reauthRequiredAt: server.reauthRequiredAt
      }
    );
    return server;
  }

  function seedStore() {
    serverRegistry.init();
    const server = ensureStorybookServer();
    const store = serverRegistry.getStore(server.id);

    store.projection.rooms.set(
      roomId,
      new RoomWithViewerState({
        room: { id: roomId },
        viewerState: {
          isMember: true,
          permissions: ['start', 'join', 'voice', 'camera', 'screenshare'].map((name) => ({
            permission: `call.${name}`,
            granted: true
          }))
        }
      })
    );
    const voiceCall = serverUi(store).voiceCall;
    voiceCall.roomId = scenario === 'idle' ? null : roomId;
    voiceCall.connected = scenario !== 'idle';
    voiceCall.audioBoostAvailable = true;
    voiceCall.connecting = false;
    voiceCall.isMuted = initiallyMuted;
    voiceCall.isCameraEnabled = scenario !== 'voice' && scenario !== 'screen-voice';
    voiceCall.isScreenShareEnabled = scenario === 'screen';
    voiceCall.participants = scenario === 'idle' ? [] : participantsForScenario();
  }

  onMount(async () => {
    seedStore();
    Panel = (await import('./VoiceCallPanel.svelte')).default as Component<VoiceCallPanelProps>;
  });

  onMount(() => {
    if (!animateVoice) return;
    const call = serverUi(serverRegistry.getStore(getScopedServerId())).voiceCall;
    const original = call.getAudioLevel;
    const originalScreen = call.getScreenShareAudioLevel;
    call.getAudioLevel = (identity) => {
      const time = performance.now() / 1000 + identity.length;
      const audioLevel =
        time % 7 < 4.5 ? 0.005 + Math.pow((Math.sin(time * 8) + 1) / 2, 2) * 0.075 : 0;
      return { isSpeaking: audioLevel > 0, audioLevel };
    };
    call.getScreenShareAudioLevel = () => {
      const time = performance.now() / 1000;
      return time % 9 < 5 ? 0.03 + (Math.sin(time * 2) + 1) * 0.06 : 0;
    };
    return () => {
      call.getAudioLevel = original;
      call.getScreenShareAudioLevel = originalScreen;
    };
  });
</script>

{#if playableMedia}
  <button type="button" onclick={() => (panelVisible = !panelVisible)}>
    {panelVisible ? 'Hide call panel' : 'Show call panel'}
  </button>
  <button
    type="button"
    onclick={() =>
      serverUi(serverRegistry.getStore(getScopedServerId())).voiceCall.handleRoomAccessRevoked(
        roomId
      )}
  >
    End call
  </button>
{/if}
{#if Panel && panelVisible}
  <Panel {roomId} livekitUrl="wss://livekit.invalid" {layout} />
{/if}
