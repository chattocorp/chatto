export type JsonValue =
  null | boolean | number | string | JsonValue[] | { [key: string]: JsonValue };

export interface WorkflowResult {
  summary: string;
  /** Human-readable Markdown shown after the summary in interactive mode. */
  details?: string;
  outputs?: Record<string, JsonValue>;
}

export type WorkflowReturn = WorkflowResult | JsonValue | undefined | void;
