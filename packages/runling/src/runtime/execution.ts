import { AsyncLocalStorage } from 'node:async_hooks';

interface ExecutionServices {
  verbose: boolean;
  /** The CLI prints the root result itself. Keep agent replies in events without printing them. */
  printAgentReplies?: boolean;
}

const services = new AsyncLocalStorage<ExecutionServices>();
export const executionServices = () => services.getStore();
export const withExecutionServices = <T>(options: ExecutionServices, work: () => T): T =>
  services.run(options, work);
