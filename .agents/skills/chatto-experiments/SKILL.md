---
name: chatto-experiments
description: Add, review, adopt, or retire an opt-in experiment in Chatto’s bundled frontend. Use for features controlled from App Preferences → Experiments.
---

# Chatto client experiments

Use a client experiment when a user can choose the behavior independently in their app. The choice applies across connected servers and tabs that share browser storage. Server operator settings, permissions, and server capability discovery have separate owners. See [FDR-059](../../../docs/fdr/FDR-059-client-experimental-features.md).

## Add an experiment

- Register a named `ClientExperiment` in [experimentalFeatures.svelte.ts](../../../apps/frontend/src/lib/state/experimentalFeatures.svelte.ts). Give it a stable storage key of its own. The existing class starts disabled, exposes a reactive `enabled` property, and persists changes. The root layout already refreshes registered experiments when another tab writes, removes, or clears local storage.
- Bind a labelled checkbox to that property in [AppExperimentsSettings.svelte](../../../apps/frontend/src/lib/components/settings/AppExperimentsSettings.svelte). Add its label to the frontend translation catalogs.
- Read the flag where the two behaviors diverge. Keep the established behavior in the disabled branch. Put code specific to the experiment behind the enabled branch. For a layout experiment, share the existing actions, data, and accessibility behavior that both layouts need. [MessageView.svelte](../../../apps/frontend/src/lib/components/messages/MessageView.svelte) selects the layout once. [ExperimentalMessageHeader.svelte](../../../apps/frontend/src/lib/components/messages/ExperimentalMessageHeader.svelte) owns the alternate presentation.
- Exercise both branches and toggle the feature while its consumer is mounted. Check the surfaces that share that consumer. The [two-tab preference test](../../../apps/frontend/e2e/user-settings.test.ts) covers persistence and propagation. Extend that coverage when changing the storage contract. Record the user-visible choice in the relevant FDR.

## Adopt or retire an experiment

To adopt the behavior, make the enabled branch unconditional, give the retained implementation its permanent name, and remove the previous branch. To retire it, keep the disabled branch and remove the experimental implementation. In either case, remove the registry entry, checkbox, and obsolete translation keys. Keep tests for the retained behavior and update its FDR.

Removing the registry entry makes existing saved choices inert. Leave unrelated app preferences intact and keep the old storage key out of use so an earlier opt-in cannot enable a different experiment.
