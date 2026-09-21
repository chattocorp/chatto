import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { lookup } from "node:dns/promises";
import { isIP } from "node:net";
import { Type } from "typebox";
import { Agent } from "undici";

const MAX_RESPONSE_BYTES = 100_000;
const REQUEST_TIMEOUT_MS = 30_000;
const MAX_REDIRECTS = 5;

const parameters = Type.Object({
  url: Type.String({
    description: "The absolute HTTP or HTTPS URL to fetch",
    minLength: 1,
  }),
});

interface WebFetchDependencies {
  fetch(input: URL, init: RequestInit, addresses: readonly string[]): Promise<Response>;
  resolveAddresses(hostname: string): Promise<readonly string[]>;
}

const defaultDependencies: WebFetchDependencies = {
  fetch: fetchWithPinnedAddresses,
  async resolveAddresses(hostname) {
    if (isIP(hostname) !== 0) return [hostname];
    return (await lookup(hostname, { all: true, verbatim: true })).map(
      ({ address }) => address,
    );
  },
};

/** Connect only to checked addresses while retaining the URL's Host and TLS name. */
export async function fetchWithPinnedAddresses(
  input: URL,
  init: RequestInit,
  addresses: readonly string[],
): Promise<Response> {
  const resolved = addresses.map((address) => ({ address, family: isIP(address) }));
  const dispatcher = new Agent({
    autoSelectFamily: true,
    connect: {
      lookup(_hostname, options, callback) {
        const matches = options.family
          ? resolved.filter(({ family }) => family === options.family)
          : resolved;
        const first = matches[0];
        if (!first) {
          callback(new Error("No checked address for the requested IP family"), "", 0);
        } else if (options.all) {
          callback(null, matches);
        } else {
          callback(null, first.address, first.family);
        }
      },
    },
  });
  try {
    // Redirects are checked separately, with a new dispatcher for each hop.
    const options = { ...init, redirect: "manual" as const, dispatcher };
    return await globalThis.fetch(input, options);
  } finally {
    // Close after the response body is consumed or cancelled, not before returning it.
    dispatcher.close(() => {});
  }
}

export function createWebFetchExtension(
  dependencies: WebFetchDependencies = defaultDependencies,
) {
  return function webFetchExtension(pi: ExtensionAPI) {
    pi.registerTool({
      name: "web_fetch",
      label: "Fetch URL",
      description:
        "Fetch textual content from an HTTP or HTTPS URL. Returns the final URL, HTTP status, content type, and a size-limited response body.",
      promptSnippet: "Fetch textual content from an HTTP or HTTPS URL",
      parameters,

      async execute(_toolCallId, { url }, signal) {
        const parsedUrl = new URL(url);
        if (parsedUrl.protocol !== "http:" && parsedUrl.protocol !== "https:") {
          throw new Error("web_fetch only supports HTTP and HTTPS URLs");
        }
        if (parsedUrl.username !== "" || parsedUrl.password !== "") {
          throw new Error("web_fetch does not accept credentials in URLs");
        }

        const timeoutSignal = AbortSignal.timeout(REQUEST_TIMEOUT_MS);
        const requestSignal = signal
          ? AbortSignal.any([signal, timeoutSignal])
          : timeoutSignal;
        const response = await fetchPublicUrl(
          parsedUrl,
          requestSignal,
          dependencies,
        );

        const contentType = response.headers.get("content-type") ?? "unknown";
        if (!isTextualContentType(contentType)) {
          await response.body?.cancel();
          throw new Error(`web_fetch cannot return content type ${contentType}`);
        }

        const { text, truncated } = await readLimitedText(
          response.body,
          MAX_RESPONSE_BYTES,
        );
        const metadata = [
          `URL: ${response.url}`,
          `Status: ${response.status} ${response.statusText}`,
          `Content-Type: ${contentType}`,
        ];

        return {
          content: [
            {
              type: "text" as const,
              text: [
                ...metadata,
                "",
                text,
                ...(truncated
                  ? [`\n[Response truncated after ${MAX_RESPONSE_BYTES} bytes]`]
                  : []),
              ].join("\n"),
            },
          ],
          details: {
            url: response.url,
            status: response.status,
            contentType,
            truncated,
          },
        };
      },
    });
  };
}

const webFetchExtension = createWebFetchExtension();
export default webFetchExtension;

async function fetchPublicUrl(
  initialUrl: URL,
  signal: AbortSignal,
  dependencies: WebFetchDependencies,
): Promise<Response> {
  let url = initialUrl;

  for (let redirects = 0; redirects <= MAX_REDIRECTS; redirects++) {
    const addresses = await assertPublicDestination(url, dependencies.resolveAddresses);
    const response = await dependencies.fetch(url, {
      headers: {
        accept:
          "text/plain, text/html, text/markdown, application/json, application/xml;q=0.9, text/xml;q=0.9",
        "user-agent": "runling-web-fetch/1.0",
      },
      redirect: "manual",
      signal,
    }, addresses);

    if (!isRedirect(response.status)) return response;

    const location = response.headers.get("location");
    if (location === null) return response;
    await response.body?.cancel();
    if (redirects === MAX_REDIRECTS) {
      throw new Error(`web_fetch stopped after ${MAX_REDIRECTS} redirects`);
    }

    url = new URL(location, url);
    if (url.protocol !== "http:" && url.protocol !== "https:") {
      throw new Error("web_fetch redirects must use HTTP or HTTPS");
    }
    if (url.username !== "" || url.password !== "") {
      throw new Error("web_fetch redirects cannot contain credentials");
    }
  }

  throw new Error("web_fetch redirect limit exceeded");
}

function isRedirect(status: number): boolean {
  return [301, 302, 303, 307, 308].includes(status);
}

async function assertPublicDestination(
  url: URL,
  resolveAddresses: WebFetchDependencies["resolveAddresses"],
): Promise<readonly string[]> {
  const hostname = url.hostname.replace(/^\[|\]$/g, "");
  const addresses = await resolveAddresses(hostname);
  if (addresses.length === 0) {
    throw new Error(`web_fetch could not resolve ${url.hostname}`);
  }

  const blockedAddress = addresses.find((address) => !isPublicIpAddress(address));
  if (blockedAddress !== undefined) {
    throw new Error(
      `web_fetch blocked non-public destination ${url.hostname} (${blockedAddress})`,
    );
  }
  return addresses;
}

function isPublicIpAddress(address: string): boolean {
  if (isIP(address) === 4) return isPublicIpv4(parseIpv4(address));
  if (isIP(address) !== 6) return false;

  const bytes = parseIpv6(address);
  if (bytes === undefined) return false;

  // Unspecified, loopback, IPv4-compatible, and IPv4-mapped addresses.
  if (bytes.slice(0, 12).every((byte) => byte === 0)) {
    return isPublicIpv4(bytes.slice(12));
  }
  if (
    bytes.slice(0, 10).every((byte) => byte === 0) &&
    bytes[10] === 0xff &&
    bytes[11] === 0xff
  ) {
    return isPublicIpv4(bytes.slice(12));
  }

  // Only globally routable unicast space is eligible.
  if ((bytes[0]! & 0xe0) !== 0x20) return false;
  if (
    bytes[0] === 0x20 &&
    bytes[1] === 0x01 &&
    ((bytes[2] === 0x0d && bytes[3] === 0xb8) || bytes[2]! <= 0x01)
  ) {
    return false;
  }

  // 6to4 and NAT64 can otherwise tunnel requests to blocked IPv4 targets.
  if (bytes[0] === 0x20 && bytes[1] === 0x02) return false;
  return true;
}

function isPublicIpv4(bytes: readonly number[]): boolean {
  const [a = 0, b = 0, c = 0] = bytes;
  return !(
    a === 0 ||
    a === 10 ||
    (a === 100 && b >= 64 && b <= 127) ||
    a === 127 ||
    (a === 169 && b === 254) ||
    (a === 172 && b >= 16 && b <= 31) ||
    (a === 192 && b === 0 && (c === 0 || c === 2)) ||
    (a === 192 && b === 88 && c === 99) ||
    (a === 192 && b === 168) ||
    (a === 198 && (b === 18 || b === 19)) ||
    (a === 198 && b === 51 && c === 100) ||
    (a === 203 && b === 0 && c === 113) ||
    a >= 224
  );
}

function parseIpv4(address: string): number[] {
  return address.split(".").map(Number);
}

function parseIpv6(address: string): number[] | undefined {
  const sections = address.toLowerCase().split("::");
  if (sections.length > 2) return undefined;

  const parseSection = (section: string): number[] | undefined => {
    if (section === "") return [];
    const groups: number[] = [];
    for (const part of section.split(":")) {
      if (part.includes(".")) {
        const ipv4 = parseIpv4(part);
        if (ipv4.length !== 4) return undefined;
        groups.push((ipv4[0]! << 8) | ipv4[1]!, (ipv4[2]! << 8) | ipv4[3]!);
        continue;
      }
      const group = Number.parseInt(part, 16);
      if (!Number.isInteger(group) || group < 0 || group > 0xffff) {
        return undefined;
      }
      groups.push(group);
    }
    return groups;
  };

  const left = parseSection(sections[0]!);
  const right = parseSection(sections[1] ?? "");
  if (left === undefined || right === undefined) return undefined;

  const missing = 8 - left.length - right.length;
  if ((sections.length === 1 && missing !== 0) || missing < 0) return undefined;
  const groups = [...left, ...Array(missing).fill(0), ...right];
  if (groups.length !== 8) return undefined;
  return groups.flatMap((group) => [group >> 8, group & 0xff]);
}

function isTextualContentType(contentType: string): boolean {
  const mediaType = contentType.split(";", 1)[0]?.trim().toLowerCase();
  return (
    mediaType?.startsWith("text/") === true ||
    mediaType?.endsWith("+json") === true ||
    mediaType?.endsWith("+xml") === true ||
    mediaType === "application/json" ||
    mediaType === "application/xml" ||
    mediaType === "application/javascript" ||
    mediaType === "application/x-www-form-urlencoded"
  );
}

async function readLimitedText(
  body: ReadableStream<Uint8Array> | null,
  maxBytes: number,
): Promise<{ text: string; truncated: boolean }> {
  if (body === null) return { text: "", truncated: false };

  const reader = body.getReader();
  const decoder = new TextDecoder();
  let bytesRead = 0;
  let text = "";

  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) return { text: text + decoder.decode(), truncated: false };

      const remaining = maxBytes - bytesRead;
      if (value.byteLength > remaining) {
        text += decoder.decode(value.subarray(0, remaining), { stream: true });
        await reader.cancel();
        return { text: text + decoder.decode(), truncated: true };
      }

      bytesRead += value.byteLength;
      text += decoder.decode(value, { stream: true });
    }
  } finally {
    reader.releaseLock();
  }
}
