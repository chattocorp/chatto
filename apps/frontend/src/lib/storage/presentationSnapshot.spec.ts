import { createHash } from 'node:crypto';
import { describe, expect, it } from 'vitest';
import { SAVED_RESOURCE_SCHEMA_VERSION } from './savedViews';
import { notificationSnapshotSchema, timelineSnapshotSchema } from './presentationSnapshot';

/**
 * Schema fingerprint for each released resource schema version. Do not change
 * an existing entry. When the schemas change, bump
 * `SAVED_RESOURCE_SCHEMA_VERSION` and add the new fingerprint here.
 */
const FINGERPRINTS: Record<number, string> = {
  1: 'e703e0cd89b17d35'
};

type Schema = { _zod: { def: Definition } };
type Definition = {
  type: string;
  shape?: Record<string, Schema>;
  element?: Schema;
  innerType?: Schema;
  options?: Schema[];
  entries?: Record<string, unknown>;
  values?: unknown[];
  keyType?: Schema;
  valueType?: Schema;
  getter?: () => Schema;
};

const LEAF_TYPES = new Set(['string', 'number', 'boolean', 'null', 'undefined', 'any', 'unknown']);

/**
 * Describe the accepted data shape from Zod's schema definitions. Unlike its
 * JSON Schema output, this description does not change when Zod changes its
 * output format.
 */
function describeSchema(schema: Schema, path: Schema[] = []): unknown {
  const cycle = path.indexOf(schema);
  if (cycle >= 0) return { recursive: path.length - cycle };
  const def = schema._zod.def;
  const child = (value: Schema) => describeSchema(value, [...path, schema]);
  switch (def.type) {
    case 'object': {
      const shape = def.shape ?? {};
      return {
        object: Object.fromEntries(
          Object.keys(shape)
            .sort()
            .map((key) => [key, child(shape[key])])
        )
      };
    }
    case 'array':
      return { array: child(def.element!) };
    case 'optional':
    case 'nullable':
      return { [def.type]: child(def.innerType!) };
    case 'union':
      return { union: def.options!.map(child) };
    case 'enum':
      return { enum: Object.values(def.entries ?? {}).map(String).sort() };
    case 'literal':
      return { literal: def.values };
    case 'record':
      return { record: [child(def.keyType!), child(def.valueType!)] };
    case 'lazy':
      return { lazy: child(def.getter!()) };
    default:
      if (LEAF_TYPES.has(def.type)) return def.type;
      throw new Error(`Describe the Zod schema type "${def.type}" in this fingerprint.`);
  }
}

function fingerprint(): string {
  const shapes = [timelineSnapshotSchema, notificationSnapshotSchema].map((schema) =>
    describeSchema(schema as unknown as Schema)
  );
  return createHash('sha256').update(JSON.stringify(shapes)).digest('hex').slice(0, 16);
}

describe('saved resource schemas', () => {
  it('bump the resource schema version when they change', () => {
    expect(
      fingerprint(),
      'The saved-view schemas changed. Bump SAVED_RESOURCE_SCHEMA_VERSION and add its fingerprint.'
    ).toBe(FINGERPRINTS[SAVED_RESOURCE_SCHEMA_VERSION]);
  });
});
