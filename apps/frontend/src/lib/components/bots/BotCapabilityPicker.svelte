<!--
@component

Lets the creator of a new bot select its capabilities as large tiles. A
capability that the creator cannot give to a bot is disabled. The owner
applies the selection after it creates the bot.
-->
<script lang="ts">
  import { m } from '$lib/i18n/messages';
  import { SelectableTile } from '$lib/ui';
  import {
    BOT_CAPABILITIES,
    botCapabilityAvailable,
    type BotCapabilityId
  } from './botCapabilities';

  let {
    selected = $bindable([]),
    serverScope,
    disabled = false
  }: {
    /** The selected capabilities, in catalogue order. */
    selected?: BotCapabilityId[];
    /** The creator's effective server-scope permissions. */
    serverScope: Readonly<Record<string, boolean>>;
    /** Locks every tile, for example while the bot is created. */
    disabled?: boolean;
  } = $props();

  function toggle(id: BotCapabilityId, checked: boolean) {
    selected = BOT_CAPABILITIES.map((capability) => capability.id).filter((capabilityId) =>
      capabilityId === id ? checked : selected.includes(capabilityId)
    );
  }
</script>

<fieldset class="flex flex-col gap-3" {disabled}>
  <legend class="mb-1 font-medium text-text-top">{m('settings.bots.capabilities.legend')}</legend>
  <p class="text-sm text-muted">{m('settings.bots.capabilities.hint')}</p>
  <div class="grid gap-3 sm:grid-cols-2">
    {#each BOT_CAPABILITIES as capability (capability.id)}
      <SelectableTile
        checked={selected.includes(capability.id)}
        icon={capability.icon}
        title={m(`settings.bots.capabilities.${capability.id}.title`)}
        description={m(`settings.bots.capabilities.${capability.id}.description`)}
        disabled={!botCapabilityAvailable(capability.id, serverScope)}
        disabledReason={m('settings.bots.capabilities.unavailable')}
        onchange={(checked) => toggle(capability.id, checked)}
      />
    {/each}
  </div>
</fieldset>
