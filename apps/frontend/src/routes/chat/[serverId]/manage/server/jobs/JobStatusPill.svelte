<!-- @component Job execution status. A running job also shows an activity indicator. -->
<script lang="ts">
  import { JobStatus } from '$lib/api/adminJobs';
  import { ActivityIndicator, Pill } from '$lib/ui';
  import { jobStatusLabel, jobStatusTone } from './jobPresentation';

  let { status }: { status: JobStatus } = $props();
</script>

<Pill tone={jobStatusTone(status)}>
  <span class="inline-flex items-center gap-1.5">
    {#if status === JobStatus.RUNNING}
      <ActivityIndicator />
    {/if}
    <span>{jobStatusLabel(status)}</span>
  </span>
</Pill>
