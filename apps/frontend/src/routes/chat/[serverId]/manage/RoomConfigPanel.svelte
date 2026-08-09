<script lang="ts">
	import { createMutation, createQuery } from '@tanstack/svelte-query';
	import {
		createAdminRoomConfigAPI,
		type RoomConfigScope
	} from '$lib/api-client/adminRoomConfig';
	import { Panel } from '$lib/components/admin';
	import { m } from '$lib/i18n/messages';
	import { getFormattingLocale } from '$lib/i18n/runtime';
	import { adminQueryKeys } from '$lib/query/admin';
	import { queryClient } from '$lib/query/client';
	import { useServerScope } from '$lib/state/server/scope.svelte';
	import FormField from '$lib/ui/form/FormField.svelte';
	import { toast } from '$lib/ui/toast';

	let { scope }: { scope: RoomConfigScope } = $props();

	const serverScope = useServerScope();
	const scopeKey = $derived(
		scope.kind === 'server' ? 'server' : `${scope.kind}:${scope.id}`
	);
	const supportsRoomConfig = $derived(
		serverScope.store.serverInfo.supportsFeature('roomConfig')
	);

	const roomConfigQuery = createQuery(
		() => ({
			queryKey: adminQueryKeys.roomConfig(
				serverScope.serverId,
				serverScope.connection,
				scopeKey
			),
			queryFn: ({ signal }) =>
				serverScope.connection
					.getAPI(createAdminRoomConfigAPI)
					.getRoomConfig(scope, { signal }),
			enabled: supportsRoomConfig,
			refetchOnMount: 'always' as const,
			// Administrative configuration can change through another session or
			// because a room moves between groups. Keep an open panel convergent
			// even when a more specific layer supplies its effective value.
			refetchInterval: 15_000,
			refetchIntervalInBackground: false
		}),
		() => queryClient
	);

	let pendingChoice = $state<string | null>(null);
	const storedChoice = $derived(
		roomConfigQuery.data?.authorEditWindowSeconds === null ||
			roomConfigQuery.data?.authorEditWindowSeconds === undefined
			? 'inherit'
			: String(roomConfigQuery.data.authorEditWindowSeconds)
	);
	const choice = $derived(pendingChoice ?? storedChoice);

	const presetOptions = $derived([
		{ value: 'inherit', label: m('admin.room_config.inherit') },
		{ value: '0', label: m('admin.room_config.disabled') },
		{ value: '1800', label: m('admin.room_config.thirty_minutes') },
		{ value: '10800', label: m('admin.room_config.three_hours') },
		{ value: '86400', label: m('admin.room_config.one_day') },
		{ value: '604800', label: m('admin.room_config.seven_days') }
	]);
	const options = $derived.by(() => {
		if (
			storedChoice === 'inherit' ||
			presetOptions.some((option) => option.value === storedChoice)
		) {
			return presetOptions;
		}
		return [
			...presetOptions,
			{ value: storedChoice, label: formatSeconds(Number(storedChoice)) }
		];
	});

	function formatSeconds(seconds: number): string {
		return new Intl.NumberFormat(getFormattingLocale(), {
			style: 'unit',
			unit: 'second',
			unitDisplay: 'long'
		}).format(seconds);
	}

	function durationLabel(seconds: number): string {
		return options.find((option) => option.value === String(seconds))?.label ?? formatSeconds(seconds);
	}

	const updateMutation = createMutation(
		() => ({
			mutationFn: (value: number | null) =>
				serverScope.connection
					.getAPI(createAdminRoomConfigAPI)
					.updateRoomConfig(scope, value),
			onSuccess: (configuration) => {
				queryClient.setQueryData(
					adminQueryKeys.roomConfig(
						serverScope.serverId,
						serverScope.connection,
						scopeKey
					),
					configuration
				);
				pendingChoice = null;
				toast.success(m('common.saved'));
			},
			onError: () => {
				pendingChoice = null;
				toast.error(m('server_settings.save_failed'));
			}
		}),
		() => queryClient
	);

	function updateChoice(event: Event): void {
		const value = (event.currentTarget as HTMLSelectElement).value;
		if (value === choice || updateMutation.isPending) return;
		pendingChoice = value;
		updateMutation.mutate(value === 'inherit' ? null : Number(value));
	}
</script>

{#if supportsRoomConfig}
	<Panel
		title={m('admin.room_config.title')}
		subtitle={m('admin.room_config.subtitle')}
		icon="iconify icon-[uil--sliders-v-alt]"
	>
		{#if roomConfigQuery.isPending}
			<p class="text-muted">{m('admin.common.loading')}</p>
		{:else if roomConfigQuery.error}
			<p class="text-danger">{m('common.error.generic')}</p>
		{:else if roomConfigQuery.data}
			<div class="flex flex-col gap-4">
				<FormField
					id={`author-edit-window-${scopeKey}`}
					label={m('admin.room_config.author_edit_window')}
					description={m('admin.room_config.author_edit_window_description')}
				>
					<select
						id={`author-edit-window-${scopeKey}`}
						class="input"
						value={choice}
						disabled={updateMutation.isPending}
						onchange={updateChoice}
					>
						{#each options as option (option.value)}
							<option value={option.value}>{option.label}</option>
						{/each}
					</select>
				</FormField>

				<div class="surface-box flex flex-wrap items-center justify-between gap-2 px-4 py-3">
					<span class="font-medium text-text">{m('admin.room_config.effective')}</span>
					<span class="text-muted">
						{durationLabel(roomConfigQuery.data.effectiveAuthorEditWindowSeconds)}
						{#if roomConfigQuery.data.authorEditWindowSeconds === null}
							· {m('admin.room_config.inherit')}
						{/if}
					</span>
				</div>
			</div>
		{/if}
	</Panel>
{/if}
