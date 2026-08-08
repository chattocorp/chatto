<script lang="ts">
	import { createMutation, createQuery } from '@tanstack/svelte-query';
	import {
		createAdminPolicyAPI,
		type PolicyConfiguration,
		type PolicyScope
	} from '$lib/api-client/adminPolicies';
	import { Panel } from '$lib/components/admin';
	import { m } from '$lib/i18n/messages';
	import { getFormattingLocale } from '$lib/i18n/runtime';
	import { adminQueryKeys } from '$lib/query/admin';
	import { queryClient } from '$lib/query/client';
	import { useServerScope } from '$lib/state/server/scope.svelte';
	import FormField from '$lib/ui/form/FormField.svelte';
	import { toast } from '$lib/ui/toast';

	let { scope }: { scope: PolicyScope } = $props();

	const serverScope = useServerScope();
	const scopeKey = $derived(
		scope.kind === 'server' ? 'server' : `${scope.kind}:${scope.id}`
	);
	const supportsPolicies = $derived(
		serverScope.store.serverInfo.supportsFeature('runtimePolicies')
	);

	const policyQuery = createQuery(
		() => ({
			queryKey: adminQueryKeys.policies(
				serverScope.serverId,
				serverScope.connection,
				scopeKey
			),
			queryFn: ({ signal }) =>
				serverScope.connection
					.getAPI(createAdminPolicyAPI)
					.getPolicyConfiguration(scope, { signal }),
			enabled: supportsPolicies,
			refetchOnMount: 'always' as const,
			// Administrative configuration can change through another session or
			// because a room moves between groups. Keep an open panel convergent
			// even when its effective value is masked by a more specific override.
			refetchInterval: 15_000,
			refetchIntervalInBackground: false
		}),
		() => queryClient
	);

	let pendingChoice = $state<string | null>(null);
	const storedChoice = $derived(
		policyQuery.data?.authorEditWindowSeconds === null ||
			policyQuery.data?.authorEditWindowSeconds === undefined
			? 'inherit'
			: String(policyQuery.data.authorEditWindowSeconds)
	);
	const choice = $derived(pendingChoice ?? storedChoice);

	const presetOptions = $derived([
		{ value: 'inherit', label: m('admin.policies.inherit') },
		{ value: '0', label: m('admin.policies.disabled') },
		{ value: '1800', label: m('admin.policies.thirty_minutes') },
		{ value: '10800', label: m('admin.policies.three_hours') },
		{ value: '86400', label: m('admin.policies.one_day') },
		{ value: '604800', label: m('admin.policies.seven_days') }
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

	function sourceLabel(configuration: PolicyConfiguration): string {
		switch (configuration.authorEditWindowSource.kind) {
			case 'product-default':
				return m('admin.policies.source_product_default');
			case 'server':
				return m('admin.policies.source_server');
			case 'room-group':
				return m('admin.policies.source_room_group');
			case 'room':
				return m('admin.policies.source_room');
			default:
				return m('admin.common.unknown');
		}
	}

	const updateMutation = createMutation(
		() => ({
			mutationFn: (value: number | null) =>
				serverScope.connection
					.getAPI(createAdminPolicyAPI)
					.updatePolicyConfiguration(scope, value),
			onSuccess: (configuration) => {
				queryClient.setQueryData(
					adminQueryKeys.policies(
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

{#if supportsPolicies}
	<Panel
		title={m('admin.policies.title')}
		subtitle={m('admin.policies.subtitle')}
		icon="iconify icon-[uil--sliders-v-alt]"
	>
		{#if policyQuery.isPending}
			<p class="text-muted">{m('admin.common.loading')}</p>
		{:else if policyQuery.error}
			<p class="text-danger">{m('common.error.generic')}</p>
		{:else if policyQuery.data}
			<div class="flex flex-col gap-4">
				<FormField
					id={`author-edit-window-${scopeKey}`}
					label={m('admin.policies.author_edit_window')}
					description={m('admin.policies.author_edit_window_description')}
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
					<span class="font-medium text-text">{m('admin.policies.effective')}</span>
					<span class="text-muted">
						{durationLabel(policyQuery.data.effectiveAuthorEditWindowSeconds)} ·
						{sourceLabel(policyQuery.data)}
					</span>
				</div>
			</div>
		{/if}
	</Panel>
{/if}
