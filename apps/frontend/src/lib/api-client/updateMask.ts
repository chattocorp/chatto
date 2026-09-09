/** Build a protobuf update mask from explicitly supplied editable fields.
 * Null, false, zero, empty strings, and empty lists are deliberate updates.
 * Generated clients expect protobuf snake_case paths; ProtoJSON serializes
 * these as a comma-separated camelCase string on the wire.
 */
export function updateMask<T extends object>(values: T, fields: readonly (keyof T & string)[]) {
  return {
    paths: fields
      .filter((field) => values[field] !== undefined)
      .map((field) => field.replace(/[A-Z]/g, (letter) => `_${letter.toLowerCase()}`))
  };
}
