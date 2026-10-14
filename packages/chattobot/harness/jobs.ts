/**
 * Background jobs of a conversation, such as investigations and actions that wait for approval.
 * A job starts from a tool call and returns at once, so the agent can keep talking. The job
 * reports progress while it runs, and its result reaches the agent in a later turn.
 */
import type { Destination } from '@chatto/client';

/** How a job ended. The text goes to the agent. */
export type JobOutcome = { result: string } | { error: string };

export interface Job {
  id: string;
  /** The kind of job, such as `investigation` or `approval`. Progress lines show it. */
  kind: string;
  /** What the job does, for the agent and the progress lines, such as "Investigation: …". */
  label: string;
  startedAt: number;
  /** The latest progress update. */
  latest?: string;
  /** Where results and progress go: a reply to the message that led to the job. */
  replyTo: Destination;
}

export interface JobEvents {
  /** Progress, in batches of updates that arrived close together. */
  progress(job: Job, updates: string[]): void;
  /** Called once per job, after all its progress. */
  finished(job: Job, outcome: JobOutcome): void;
}

/** Updates within this time go into one batch, such as the reads of one model step. */
const BATCH_MS = 1000;

export class Jobs {
  private running = new Map<string, Job>();

  constructor(private events: JobEvents) {}

  /** The number of jobs that run now. */
  get size() {
    return this.running.size;
  }

  has(id: string) {
    return this.running.has(id);
  }

  list(): Job[] {
    return [...this.running.values()];
  }

  /** Start a job. `work` reports progress and returns the result text for the agent. */
  start(
    {
      id = crypto.randomUUID(),
      kind,
      label,
      replyTo
    }: { id?: string; kind: string; label: string; replyTo: Destination },
    work: (report: (update: string) => void) => Promise<string>
  ): Job {
    if (this.running.has(id)) throw new Error(`Job ${id} is already running`);
    const job: Job = { id, kind, label, startedAt: Date.now(), replyTo };
    this.running.set(id, job);
    let batch: string[] = [];
    let timer: ReturnType<typeof setTimeout> | undefined;
    const flush = () => {
      clearTimeout(timer);
      timer = undefined;
      if (!batch.length) return;
      job.latest = batch.at(-1);
      this.events.progress(job, batch.splice(0));
    };
    const report = (update: string) => {
      batch.push(update);
      timer ??= setTimeout(flush, BATCH_MS);
    };
    void work(report).then(
      (result) => this.finish(job, flush, { result }),
      (error: unknown) =>
        this.finish(job, flush, { error: error instanceof Error ? error.message : String(error) })
    );
    return job;
  }

  private finish(job: Job, flush: () => void, outcome: JobOutcome) {
    flush();
    this.running.delete(job.id);
    this.events.finished(job, outcome);
  }
}
