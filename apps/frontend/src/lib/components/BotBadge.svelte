<!-- SPDX-License-Identifier: Apache-2.0 -->
<!-- @component Compact, shared marker for visibly identifying bot accounts. -->
<script lang="ts">
  import { Pill } from '$lib/ui';
  import * as m from '$lib/i18n/messages';
</script>

<Pill tone="neutral">
  <span class="iconify uil--robot" aria-hidden="true"></span>
  {m['bots.badge.bot']()}
</Pill>
