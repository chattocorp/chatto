<script lang="ts">
	import { onMount } from 'svelte';
	import type { Component } from 'svelte';
	import type { Track } from 'livekit-client';
	import type { CallParticipantInfo } from '$lib/state/server/voiceCall.svelte';
	import type { ServerPermissions } from '$lib/state/server/permissions.svelte';
	import { createPresenceCache } from '$lib/state/presenceCache.svelte';
	import { createUserProfileCache } from '$lib/state/userProfiles.svelte';
	import { serverRegistry, type RegisteredServer } from '$lib/state/server/registry.svelte';

	type VoiceCallPanelProps = {
		roomId: string;
		livekitUrl: string;
		layout?: 'sidebar' | 'stage';
	};

	let {
		layout = 'stage',
		scenario = 'screen'
	}: {
		layout?: 'sidebar' | 'stage';
		scenario?: 'screen' | 'screen-single-secondary' | 'camera' | 'voice';
	} = $props();

	const roomId = 'storybook-call-room';
	const storybookServerId = 'storybook-call-server';
	createPresenceCache();
	createUserProfileCache();
	let Panel = $state<Component<VoiceCallPanelProps> | null>(null);

	const permissions: ServerPermissions = {
		loaded: true,
		canViewAdmin: false,
		canStartDMs: false,
		canAdminViewUsers: false,
		canAdminManageAccounts: false,
		canAssignRoles: false,
		canAdminViewRoles: false,
		canAdminManageRoles: false,
		canAdminViewSystem: false,
		canAdminViewAudit: false
	};

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

		return [
			participant('viewer', 'Alice', { isLocal: true }),
			bob,
			chloe
		];
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
		serverRegistry.addServer(server);
		return server;
	}

	function seedStore() {
		const server = ensureStorybookServer();
		const store = serverRegistry.getStore(server.id);

		store.permissions = permissions;
		store.rooms.currentUserId = 'viewer';
		store.voiceCall.roomId = roomId;
		store.voiceCall.connected = true;
		store.voiceCall.connecting = false;
		store.voiceCall.isMuted = false;
		store.voiceCall.isCameraEnabled = scenario !== 'voice';
		store.voiceCall.isScreenShareEnabled = scenario === 'screen';
		store.voiceCall.participants = participantsForScenario();
	}

	onMount(async () => {
		seedStore();
		Panel = (await import('./VoiceCallPanel.svelte')).default as Component<VoiceCallPanelProps>;
	});
</script>

{#if Panel}
	<Panel {roomId} livekitUrl="wss://livekit.invalid" {layout} />
{/if}
