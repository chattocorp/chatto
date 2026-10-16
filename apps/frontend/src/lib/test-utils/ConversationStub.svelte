<!-- Isolates route resolution tests from the conversation's own component tests. -->
<script lang="ts">
  let { roomId }: { roomId: string } = $props();
</script>

<p>Conversation: {roomId}</p>
