/** Convert live execution events into the server's terminal output and safe log file records. */
import type { RunlingEvent } from './events.ts';
import { serverLog, terminalRunLog } from './server-log.ts';

/** Log one live server run. The log file gets safe summaries: task numbers are local to the run,
 * and prompts, task labels, commands, arguments, results, and arbitrary logs are never written.
 * Agent labels and explicit task activity are public host-owned metadata. Successful tool calls
 * are summarized at most every 30 seconds per agent.
 *
 * With `detailed` (the default unless `RUNLING_QUIET_LOG` is `1`), the terminal shows the run's
 * full log, like `runling run`, including failed task errors; routine summaries then go only to
 * the file, and warnings and errors go to both. Otherwise the terminal shows the summaries. */
export function createServerActivityLog(
  runId: string,
  runReference?: string,
  { detailed = process.env.RUNLING_QUIET_LOG !== '1' }: { detailed?: boolean } = {}
) {
  const tasks = new Map<string, string>();
  const agents = new Map<string, number>();
  const labels = new Map<string, string>();
  const taskLabels = new Map<string, string>();
  const channels = new Map<string, string>();
  const counts = new Map<string, Map<string, number>>();
  const lastAgentEvent = new Map<string, number>();
  const lastReminder = new Map<string, number>();
  const agentTasks = new Map<string, string>();
  let reminder: ReturnType<typeof setInterval> | undefined;
  let sequence = 0;
  const taskReference = (id: string) => {
    let reference = tasks.get(id);
    if (!reference) {
      reference = `task-${++sequence}`;
      tasks.set(id, reference);
    }
    return reference;
  };
  const remind = () => {
    const now = Date.now();
    for (const [agentId, last] of lastAgentEvent) {
      const remindedAt = lastReminder.get(agentId);
      if (now - last < 120_000 || (remindedAt !== undefined && now - remindedAt < 300_000))
        continue;
      lastReminder.set(agentId, now);
      const taskId = agentTasks.get(agentId);
      serverLog('warn', 'run.activity', {
        runId,
        runReference,
        activity: `No recorded agent activity for ${Math.floor((now - last) / 60_000)} min; agent still running`,
        ...(taskId ? { taskId, taskReference: taskReference(taskId) } : {}),
        ...(labels.has(agentId) ? { agentLabel: labels.get(agentId) } : {}),
        agentId
      });
    }
  };
  const record = (event: RunlingEvent) => {
    if ('agentId' in event && lastAgentEvent.has(event.agentId))
      lastAgentEvent.set(event.agentId, Date.now());
    let activity: string;
    let level: 'info' | 'warn' | 'error' = 'info';
    let taskId = event.activityId;
    let agentId: string | undefined;
    const context = (task = taskId, agent?: string) => ({
      runReference: runReference ?? runId.slice(0, 8),
      ...(task && taskLabels.has(task) ? { agentLabel: taskLabels.get(task) } : {}),
      ...(task ? { taskReference: taskReference(task) } : {}),
      ...(agent && labels.has(agent) ? { agentLabel: labels.get(agent) } : {})
    });
    if (event.type === 'log') {
      if (detailed)
        terminalRunLog(
          event.level,
          context(taskId, event.source === 'agent' ? event.sourceId : undefined),
          event.message
        );
      return;
    }
    if (detailed && event.type === 'step.finished' && event.status === 'failed')
      terminalRunLog(
        'error',
        context(event.id),
        `Task failed · ${Math.round(event.durationMs)} ms${event.error ? ` · ${event.error}` : ''}`
      );
    switch (event.type) {
      case 'task.linked':
        channels.set(event.channelId, event.taskId);
        return;
      case 'task.activity':
        taskId = channels.get(event.channelId);
        activity = event.message;
        if (event.level === 'error') level = 'error';
        break;
      case 'step.started':
        taskId = event.id;
        activity = 'Task started';
        break;
      case 'step.finished':
        taskId = event.id;
        activity = `Task ${event.status} · ${Math.round(event.durationMs)} ms`;
        if (event.status === 'failed') level = 'error';
        break;
      case 'agent.started':
        agentId = event.agentId;
        agents.set(agentId, event.timestamp);
        lastAgentEvent.set(agentId, Date.now());
        if (taskId) agentTasks.set(agentId, taskId);
        reminder ??= setInterval(remind, 60_000);
        reminder.unref?.();
        if (event.label) {
          labels.set(agentId, event.label);
          if (taskId) taskLabels.set(taskId, event.label);
        }
        activity = `Agent started · ${event.model}`;
        break;
      case 'agent.finished':
        agentId = event.agentId;
        agents.delete(agentId);
        lastAgentEvent.delete(agentId);
        lastReminder.delete(agentId);
        agentTasks.delete(agentId);
        if (!lastAgentEvent.size) {
          clearInterval(reminder);
          reminder = undefined;
        }
        activity = `Agent ${event.outcome}${counts.get(agentId)?.size ? ` · ${[...counts.get(agentId)!].map(([name, count]) => `${count} ${name}`).join(', ')}` : ''}`;
        counts.delete(agentId);
        if (event.outcome !== 'completed') level = event.outcome === 'failed' ? 'error' : 'warn';
        break;
      case 'agent.action':
        return;
      case 'agent.tool':
        agentId = event.agentId;
        if (event.phase === 'started') return;
        if (event.phase === 'succeeded') {
          const summary = counts.get(agentId) ?? new Map<string, number>();
          summary.set(event.operation, (summary.get(event.operation) ?? 0) + 1);
          counts.set(agentId, summary);
          if (event.timestamp - (agents.get(agentId) ?? -Infinity) < 30_000) return;
          activity = `Activity · ${[...summary].map(([name, count]) => `${count} ${name}`).join(', ')}`;
          counts.delete(agentId);
        } else activity = `Tool ${event.toolName ?? event.operation} failed`;
        if (event.phase === 'failed') level = 'error';
        agents.set(agentId, event.timestamp);
        break;
      case 'command.started':
        activity = 'Command started';
        break;
      case 'command.finished':
        activity = `Command ${event.status} · ${Math.round(event.durationMs)} ms`;
        if (event.status === 'failed') level = 'error';
        break;
      case 'input.requested':
        activity = 'Waiting for input';
        break;
      case 'input.finished':
        activity =
          event.status === 'answered' ? 'Input received' : `Input ${event.reason ?? 'failed'}`;
        if (event.status === 'failed') level = 'warn';
        break;
      case 'conversation.started':
        activity = 'Conversation ready';
        break;
      default:
        return;
    }
    serverLog(
      level,
      'run.activity',
      {
        runId,
        runReference,
        activity,
        ...(event.type === 'task.activity' && event.level ? { activityLevel: event.level } : {}),
        ...(taskId && taskLabels.has(taskId) ? { agentLabel: taskLabels.get(taskId) } : {}),
        ...(taskId ? { taskId, taskReference: taskReference(taskId) } : {}),
        ...(agentId
          ? { agentId, ...(labels.has(agentId) ? { agentLabel: labels.get(agentId) } : {}) }
          : {})
      },
      { terminal: !detailed || (level !== 'info' && event.type !== 'step.finished') }
    );
  };
  return Object.assign(record, {
    dispose() {
      clearInterval(reminder);
      reminder = undefined;
    }
  });
}
