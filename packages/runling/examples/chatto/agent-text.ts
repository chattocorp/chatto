import type { AgentOptions, RunlingAgent } from "runling/agents";
import type { WorkflowContext } from "runling";

export type ChattoAgentFactory = (options: AgentOptions) => Promise<
  Pick<RunlingAgent, "runOutcome" | "steer" | "dispose">
>;

export type SpecialistUpdate = { type: "text"; text: string };

export type SpecialistContext = WorkflowContext<string, SpecialistUpdate>;
export const progressInstructions = [
  "Your plain text messages are posted directly to the user while you work. Write brief, user-facing progress updates.",
  "Before starting tool work, say what you will check or change. When you learn something material or change approach, explain the finding and next action in one or two sentences.",
  "Describe actions and concise conclusions, not private reasoning. Avoid narrating every tool call. Do not claim progress you have not made.",
  "Put the complete final result in report_outcome for the coordinator. Include all findings and paths in that report even if you already reported them before incoming steering. Do not repeat that result in a plain text message or ask the user questions yourself.",
];
