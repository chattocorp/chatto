import Ajv2020 from "ajv/dist/2020.js";
import type { Static, TSchema } from "typebox";
import { Check, Errors } from "typebox/value";
import type { StandardSchemaV1, StandardJSONSchemaV1 } from "@standard-schema/spec";

export type WorkflowSchema = StandardSchemaV1 | TSchema;
export type SchemaInput<S extends WorkflowSchema> = S extends StandardSchemaV1
  ? StandardSchemaV1.InferInput<S>
  : S extends TSchema ? Static<S> : never;
export type SchemaOutput<S extends WorkflowSchema> = S extends StandardSchemaV1
  ? StandardSchemaV1.InferOutput<S>
  : S extends TSchema ? Static<S> : never;

const isObject = (value: unknown): value is Record<string, unknown> =>
  value !== null && (typeof value === "object" || typeof value === "function");

export function isStandardSchema(value: unknown): value is StandardSchemaV1 {
  if (!isObject(value)) return false;
  const standard = value["~standard"];
  return isObject(standard) && standard.version === 1 &&
    typeof standard.vendor === "string" && typeof standard.validate === "function";
}

const validator = new Ajv2020({
  strict: false,
  validateFormats: false,
});

/** Check for Standard Schema validation or a valid JSON Schema object. */
export function isWorkflowSchema(value: unknown): value is WorkflowSchema {
  if (isObject(value) && "~standard" in value) return isStandardSchema(value);
  return isJsonSchema(value);
}

function isJsonSchema(value: unknown): value is TSchema {
  if (typeof value !== "object" || value === null || Array.isArray(value)) {
    return false;
  }

  try {
    return validator.validateSchema(value) === true;
  } catch {
    return false;
  }
}

export interface SchemaIssue {
  path: string;
  message: string;
}

export type SchemaResult =
  | { value: unknown; issues?: undefined }
  | { issues: SchemaIssue[] };

/** Validate a value and preserve the schema's parsed output. */
export function validateSchema(
  schema: WorkflowSchema,
  value: unknown,
): SchemaResult | Promise<SchemaResult> {
  if (isStandardSchema(schema)) {
    const normalize = (result: StandardSchemaV1.Result<unknown>): SchemaResult => {
      if (!result.issues) return { value: result.value };
      return {
        issues: result.issues.map(({ path, message }) => ({
          path: path?.length ? `/${path.map(segment => {
            const key = typeof segment === "object" ? segment.key : segment;
            return String(key).replaceAll("~", "~0").replaceAll("/", "~1");
          }).join("/")}` : "/",
          message,
        })),
      };
    };
    const result = schema["~standard"].validate(value);
    return Promise.resolve(result).then(normalize);
  }
  return Check(schema, value) ? { value } : {
    issues: Errors(schema, value).map(({ instancePath, message }) => ({
      path: instancePath || "/", message,
    })),
  };
}

/** Export a schema for the values received or returned by a task. */
export function toJsonSchema(
  schema: WorkflowSchema,
  boundary: "input" | "output",
): Record<string, unknown> {
  if (!isStandardSchema(schema)) return { ...schema };
  const standard = schema["~standard"] as StandardSchemaV1.Props & Partial<StandardJSONSchemaV1.Props>;
  if (typeof standard.jsonSchema?.[boundary] !== "function") {
    throw new TypeError(`Task ${boundary} schema must support Standard JSON Schema export for webhooks`);
  }
  const exported = standard.jsonSchema[boundary]({ target: "draft-2020-12" });
  if (!isJsonSchema(exported)) throw new TypeError(`Task ${boundary} JSON Schema is invalid`);
  return exported;
}
