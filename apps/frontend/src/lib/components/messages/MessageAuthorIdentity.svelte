<!-- @component The display name gets first use of the header width. Show a truncated
login only if its prefix is long enough to distinguish the account. -->
<script lang="ts">
  import AccountName from '$lib/components/users/AccountName.svelte';
  import type { UserAvatarUserView } from '@chatto/client/timeline/users';

  let { name, actor }: { name: string; actor: UserAvatarUserView } = $props();
  const login = $derived(actor.login ?? '');
  const prefix = $derived(login.length > 3 ? login.slice(0, 3) + '…' : login);

  /** Measure the prefix in the actual font, including the gap and the @ sign. */
  function observeFit(slot: HTMLElement) {
    const probe = slot.querySelector<HTMLElement>('[aria-hidden]')!;
    const label = slot.querySelector<HTMLElement>('bdi')!;
    function update() {
      const fits = slot.clientWidth >= Math.min(probe.offsetWidth, label.scrollWidth);
      label.style.visibility = fits ? 'visible' : 'hidden';
    }
    const observer = new ResizeObserver(update);
    observer.observe(slot);
    observer.observe(probe);
    update();
    return () => observer.disconnect();
  }
</script>

<span class="inline-flex max-w-full min-w-0 items-baseline">
  <AccountName {name} identity={actor} badgeSize="md" />
  {#if login}
    <span
      class="relative min-w-0 flex-[1_1_0px] overflow-hidden font-normal text-muted"
      {@attach observeFit}
    >
      <bdi dir="ltr" class="block truncate ps-1.5">@{login}</bdi>
      <span aria-hidden="true" class="invisible absolute start-0 top-0 w-max ps-1.5">@{prefix}</span
      >
    </span>
  {/if}
</span>
