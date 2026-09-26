<!--
@component

Tiny test double for ServerDirectory used by ModalContainer specs.
-->
<script lang="ts">
  let { inDialog = false }: { inDialog?: boolean } = $props();
</script>

<div data-testid="server-directory" data-in-dialog={inDialog}>Server Directory</div>
