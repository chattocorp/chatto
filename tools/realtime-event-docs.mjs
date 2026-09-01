import { execFile } from "node:child_process";
import { readFile } from "node:fs/promises";
import { createRequire } from "node:module";
import path from "node:path";
import { promisify } from "node:util";

const execFileAsync = promisify(execFile);
const eventFieldSurfaceOptionNumber = 50001;

const scalarTypes = new Map([
  [1, "double"],
  [2, "float"],
  [3, "int64"],
  [4, "uint64"],
  [5, "int32"],
  [6, "fixed64"],
  [7, "fixed32"],
  [8, "bool"],
  [9, "string"],
  [12, "bytes"],
  [13, "uint32"],
  [15, "sfixed32"],
  [16, "sfixed64"],
  [17, "sint32"],
  [18, "sint64"],
]);

function anchor(fullName) {
  return fullName.replaceAll(".", "-");
}

function pascalCase(name) {
  return name
    .split("_")
    .map((part) => `${part.slice(0, 1).toUpperCase()}${part.slice(1)}`)
    .join("");
}

function normalizeComment(value) {
  return value
    .trim()
    .split("\n")
    .map((line) => line.trim())
    .join(" ")
    .replaceAll("|", "\\|");
}

function publicComment(value) {
  const sentences = normalizeComment(value).split(/(?<=[.!?])\s+/);
  return sentences
    .filter(
      (sentence) =>
        !/\b(?:EVT|JetStream|NATS|aggregate|migration|projector|projection|publisher|subscriber)\b/i.test(
          sentence,
        ),
    )
    .join(" ");
}

function fallbackFieldDescription(field) {
  const sentence = (value) =>
    `${value.slice(0, 1).toUpperCase()}${value.slice(1)}.`;
  if (field.name.endsWith("_plaintext")) {
    return "Plaintext value for authorized realtime delivery.";
  }
  if (field.name.endsWith("_ids")) {
    return sentence(
      `${field.name.slice(0, -4).replaceAll("_", " ")} identifiers`,
    );
  }
  if (field.name.endsWith("_id")) {
    return sentence(
      `${field.name.slice(0, -3).replaceAll("_", " ")} identifier`,
    );
  }
  return "—";
}

function sourceComment(file, descriptorPath) {
  const location = file.sourceCodeInfo?.location.find(
    (candidate) => candidate.path.join(",") === descriptorPath.join(","),
  );
  return normalizeComment(
    location?.leadingComments ?? location?.trailingComments ?? "",
  );
}

function fieldSurface(field) {
  if (!field.options) return 0;
  const unknownFields = field.options
    .getType()
    .runtime.bin.listUnknownFields(field.options)
    .filter((unknown) => unknown.no === eventFieldSurfaceOptionNumber);
  if (unknownFields.length === 0 || unknownFields[0].data.length === 0)
    return 0;
  return unknownFields[0].data[0];
}

function publicField(field, inheritUnspecified) {
  const surface = fieldSurface(field);
  return (
    surface === 1 || surface === 3 || (surface === 0 && inheritUnspecified)
  );
}

function buildDescriptorIndex(descriptorSet) {
  const messages = new Map();
  const enums = new Map();

  function addMessage(file, message, descriptorPath, parentName) {
    const fullName = [file.package, parentName, message.name]
      .filter(Boolean)
      .join(".");
    messages.set(fullName, { file, message, descriptorPath, fullName });
    message.nestedType.forEach((nested, index) =>
      addMessage(
        file,
        nested,
        [...descriptorPath, 3, index],
        [parentName, message.name].filter(Boolean).join("."),
      ),
    );
    message.enumType.forEach((nestedEnum, index) => {
      const enumFullName = `${fullName}.${nestedEnum.name}`;
      enums.set(enumFullName, {
        file,
        enum: nestedEnum,
        descriptorPath: [...descriptorPath, 4, index],
        fullName: enumFullName,
      });
    });
  }

  for (const file of descriptorSet.file) {
    file.messageType.forEach((message, index) =>
      addMessage(file, message, [4, index], ""),
    );
    file.enumType.forEach((enumDescriptor, index) => {
      const fullName = `${file.package}.${enumDescriptor.name}`;
      enums.set(fullName, {
        file,
        enum: enumDescriptor,
        descriptorPath: [5, index],
        fullName,
      });
    });
  }
  return { messages, enums };
}

function publicWrapperNames(source) {
  const functionBody = source.match(
    /func isRealtimePublicEvent\(event \*evtv1\.Event\) bool \{[\s\S]*?switch event\.GetEvent\(\)\.\(type\) \{([\s\S]*?)\n\tdefault:/,
  );
  if (!functionBody) {
    throw new Error("Cannot find the runtime public realtime event catalogue.");
  }
  const names = [
    ...functionBody[1].matchAll(/\*evtv1\.Event_([A-Za-z0-9]+)/g),
  ].map((match) => match[1]);
  if (names.length === 0) {
    throw new Error("The runtime public realtime event catalogue is empty.");
  }
  return new Set(names);
}

function fieldType(field, linkedTypes) {
  const fullType = (field.typeName ?? "").replace(/^\./, "");
  let rendered;
  if (fullType) {
    const shortName = fullType.split(".").at(-1);
    rendered = linkedTypes.has(fullType)
      ? `[\`${shortName}\`](#${anchor(fullType)})`
      : `\`${fullType}\``;
  } else {
    rendered = `\`${scalarTypes.get(field.type) ?? "unknown"}\``;
  }
  if (field.label === 3) return `repeated ${rendered}`;
  if (field.proto3Optional) return `optional ${rendered}`;
  return rendered;
}

function renderMessage(entry, inheritUnspecified, linkedTypes) {
  const { file, message, descriptorPath, fullName } = entry;
  const rows = message.field
    .map((field, index) => ({ field, index }))
    .filter(({ field }) => publicField(field, inheritUnspecified))
    .map(({ field, index }) => {
      const oneof =
        field.oneofIndex === undefined
          ? undefined
          : message.oneofDecl[field.oneofIndex];
      const fieldName =
        oneof && !oneof.name.startsWith("_")
          ? `${oneof.name}.${field.name}`
          : field.name;
      const description =
        publicComment(sourceComment(file, [...descriptorPath, 2, index])) ||
        fallbackFieldDescription(field);
      return `| \`${fieldName}\` | ${fieldType(field, linkedTypes)} | ${description} |`;
    });

  return [
    `<a id="${anchor(fullName)}"></a>`,
    "",
    `### ${message.name}`,
    "",
    `\`${fullName}\``,
    "",
    "| Field | Type | Description |",
    "| --- | --- | --- |",
    ...rows,
    "",
  ].join("\n");
}

function renderEnum(entry) {
  const { file, enum: enumDescriptor, descriptorPath, fullName } = entry;
  const description = publicComment(sourceComment(file, descriptorPath));
  return [
    `<a id="${anchor(fullName)}"></a>`,
    "",
    `### ${enumDescriptor.name}`,
    "",
    `\`${fullName}\``,
    "",
    ...(description ? [description, ""] : []),
    "| Name | Number | Description |",
    "| --- | --- | --- |",
    ...enumDescriptor.value.map((value, index) => {
      const valueDescription =
        publicComment(sourceComment(file, [...descriptorPath, 2, index])) ||
        "—";
      return `| \`${value.name}\` | \`${value.number}\` | ${valueDescription} |`;
    }),
    "",
  ].join("\n");
}

export async function renderRealtimeEventCatalogue({
  repoRoot,
  generatedNotice,
}) {
  const require = createRequire(
    path.join(repoRoot, "packages/api-types/package.json"),
  );
  const { FileDescriptorSet } = require("@bufbuild/protobuf");
  const { stdout } = await execFileAsync(
    "buf",
    [
      "build",
      path.join(repoRoot, "proto"),
      "--as-file-descriptor-set",
      "-o",
      "-",
    ],
    { cwd: repoRoot, encoding: "buffer", maxBuffer: 16 * 1024 * 1024 },
  );
  const descriptorSet = FileDescriptorSet.fromBinary(stdout);
  const index = buildDescriptorIndex(descriptorSet);
  const eventEntry = index.messages.get("chatto.core.evt.v1.Event");
  if (!eventEntry) throw new Error("Cannot find chatto.core.evt.v1.Event.");

  const catalogueSource = await readFile(
    path.join(repoRoot, "cli/internal/http_server/realtime_event_surface.go"),
    "utf8",
  );
  const wrapperNames = publicWrapperNames(catalogueSource);
  const eventOneofIndex = eventEntry.message.oneofDecl.findIndex(
    (oneof) => oneof.name === "event",
  );
  const publicVariants = eventEntry.message.field.filter(
    (field) =>
      field.oneofIndex === eventOneofIndex &&
      wrapperNames.has(pascalCase(field.name)),
  );
  if (publicVariants.length !== wrapperNames.size) {
    const found = new Set(
      publicVariants.map((field) => pascalCase(field.name)),
    );
    const missing = [...wrapperNames].filter((name) => !found.has(name));
    throw new Error(
      `Public realtime event variants are missing from Event: ${missing.join(", ")}`,
    );
  }

  const messageModes = new Map();
  const enumNames = new Set();
  const queue = publicVariants.map((field) => ({
    fullName: field.typeName.replace(/^\./, ""),
    inherit: false,
  }));
  while (queue.length > 0) {
    const { fullName, inherit } = queue.shift();
    const previous = messageModes.get(fullName);
    if (previous === true || previous === inherit) continue;
    messageModes.set(fullName, previous === true || inherit);
    const entry = index.messages.get(fullName);
    if (!entry) continue;
    for (const field of entry.message.field) {
      if (!publicField(field, inherit)) continue;
      const referencedType = (field.typeName ?? "").replace(/^\./, "");
      if (index.messages.has(referencedType)) {
        queue.push({ fullName: referencedType, inherit: true });
      } else if (index.enums.has(referencedType)) {
        enumNames.add(referencedType);
      }
    }
  }

  const linkedTypes = new Set([...messageModes.keys(), ...enumNames]);
  const durableVariants = publicVariants.filter(
    (field) => field.number < 20000,
  );
  const transientVariants = publicVariants.filter(
    (field) => field.number >= 20000,
  );
  const variantRows = (variants, recovery) =>
    variants.map((field) => {
      const fullType = field.typeName.replace(/^\./, "");
      const shortType = fullType.split(".").at(-1);
      return `| \`${field.name}\` | \`${field.number}\` | [\`${shortType}\`](#${anchor(fullType)}) | ${recovery} |`;
    });

  const body = [
    '<a id="chatto-core-evt-v1-Event"></a>',
    "",
    "## Event",
    "",
    "This catalogue contains the canonical event variants that Chatto can send through `RealtimeEvent.event`. It is a public subset of `chatto.core.evt.v1.Event`. Internal variants and storage-only fields are excluded. The page is generated from the runtime public-event catalogue and the protobuf field-surface rules.",
    "",
    "The canonical event package is public because protocol 4 puts this message directly on the wire. The package name still reflects the persisted compatibility contract. It does not mean that the server sends stored bytes.",
    "",
    "Every canonical event also contains these common fields:",
    "",
    "| Field | Type | Description |",
    "| --- | --- | --- |",
    "| `id` | `string` | Stable event identifier. |",
    "| `created_at` | `google.protobuf.Timestamp` | Time when Chatto created the event. |",
    "| `actor_id` | `string` | Visible user that caused the event, when applicable. |",
    "",
    "The opaque resume cursor is part of the surrounding `RealtimeEvent`, not the canonical event.",
    "",
    "## Durable variants",
    "",
    "Durable variants can have a resume cursor and can appear during catch-up.",
    "",
    "| Event oneof field | Number | Payload | Recovery |",
    "| --- | --- | --- | --- |",
    ...variantRows(durableVariants, "Bounded replay"),
    "",
    "## Transient variants",
    "",
    "Transient variants are live-only. They do not have a resume cursor and do not appear during catch-up.",
    "",
    "| Event oneof field | Number | Payload | Recovery |",
    "| --- | --- | --- | --- |",
    ...variantRows(transientVariants, "Live only"),
    "",
    "## Event payloads",
    "",
    ...[...messageModes.entries()].map(([fullName, inherit]) =>
      renderMessage(index.messages.get(fullName), inherit, linkedTypes),
    ),
    ...(enumNames.size > 0 ? ["## Event enums", ""] : []),
    ...[...enumNames].map((fullName) => renderEnum(index.enums.get(fullName))),
  ];

  return [
    "---",
    "title: Realtime Event Catalogue",
    "description: Public canonical event variants and client-visible payload fields for realtime protocol 4.",
    "editUrl: false",
    "---",
    "",
    generatedNotice().trim(),
    "",
    body.join("\n").trim(),
    "",
  ].join("\n");
}
