import { Command, InvalidArgumentError } from "commander";

export interface RunOptions {
  input?: string;
  json: boolean;
  log: boolean;
  verbose: boolean;
}

export interface ServeOptions {
  config: string;
  host: string;
  port: number;
  open: boolean;
}

function parsePort(value: string): number {
  const port = Number(value);
  if (!/^\d+$/.test(value) || !Number.isInteger(port) || port < 1 || port > 65535) {
    throw new InvalidArgumentError("Port must be an integer from 1 through 65535");
  }
  return port;
}

/** Share server options between the installed CLI and the development server. */
export function createServeCommand(): Command {
  return new Command("serve")
    .exitOverride()
    .description("Start the web server")
    .option("--config <path>", "Configuration file", "runling.config.ts")
    .option("--host <host>", "Hostname to listen on", "localhost")
    .option("--port <port>", "Port to listen on", parsePort, 5173)
    .option("--open", "Open the app in a browser", false);
}

export interface CliActions {
  run: (file: string, prompt: string, options: RunOptions) => Promise<void>;
  serve: (options: ServeOptions) => Promise<void>;
}

/** Parse commands without loading a workflow or starting a server for help. */
export function createCli(version: string, actions: CliActions): Command {
  const program = new Command("runling")
    .description("Run TypeScript workflows and serve the web console")
    .version(version)
    .addHelpCommand()
    .showHelpAfterError()
    .exitOverride();

  program.command("run")
    .description("Run a workflow file")
    .argument("<file>", "TypeScript workflow file")
    .argument("[prompt]", "Input passed to the workflow", "")
    .option("--input <json>", "Task input as JSON (instead of prompt)")
    .option("--json", "Write the result as JSON", false)
    .option("--log", "Use append-only logs instead of the TUI", false)
    .option("-v, --verbose", "Show debug logs", false)
    .action(async (file: string, prompt: string, options: RunOptions, command: Command) => {
      if (options.input !== undefined && command.args.length > 1) {
        command.error("Use either a prompt or --input, not both");
      }
      await actions.run(file, prompt, options);
    });

  program.addCommand(createServeCommand().action(async (options: ServeOptions) => {
    await actions.serve(options);
  }));
  program.action(() => { program.outputHelp(); });
  return program;
}
