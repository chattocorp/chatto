import { AsyncLocalStorage } from "node:async_hooks";

interface ExecutionServices {
  verbose: boolean;
}

const services = new AsyncLocalStorage<ExecutionServices>();
export const executionServices = () => services.getStore();
export const withExecutionServices = <T>(options: ExecutionServices, work: () => T): T =>
  services.run(options, work);
