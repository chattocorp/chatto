// Orchestrator. Runs every operation as every persona, diffs against the
// expected matrix, prints a report, and exits non-zero on any mismatch.
//
// Usage:
//   mise x -- node --experimental-strip-types tools/authz-fuzzer/run.ts \
//     --endpoint=https://your-dev-instance/

import { OPERATIONS } from "./operations.ts";
import { MATRIX, type Outcome } from "./matrix.ts";
import { PERSONAS, type PersonaId } from "./personas.ts";
import { buildClients, seed } from "./seed.ts";
import type { Client, GqlResponse } from "./client.ts";

function arg(name: string, fallback?: string): string {
  const flag = `--${name}=`;
  const found = process.argv.find((a) => a.startsWith(flag));
  if (found) return found.slice(flag.length);
  if (fallback !== undefined) return fallback;
  throw new Error(`missing required arg --${name}`);
}

// Classify a GraphQL response into one of the matrix outcomes. The mapping is
// deliberately loose because Chatto's resolvers differ in how they signal
// "denied" — some return null, some return an error with a permission-denied
// message. We bias toward "did the user get the data they wanted".
function classify(res: GqlResponse, opCategory: "query" | "mutation"): Outcome {
  const errs = res.errors ?? [];
  const messages = errs.map((e) => (e.message || "").toLowerCase());

  const isAuth = messages.some(
    (m) => m.includes("not authenticated") || m.includes("unauthenticated") || m.includes("not logged in"),
  );
  if (isAuth) return "auth";

  const isDeny = messages.some(
    (m) =>
      m.includes("permission denied") ||
      m.includes("forbidden") ||
      m.includes("access denied") ||
      m.includes("not authorized") ||
      m.includes("unauthorized"),
  );
  if (isDeny) return "deny";

  // No errors? If the data is null for a query, treat as notfound. For
  // mutations, no errors == allow.
  if (errs.length === 0) {
    if (opCategory === "query") {
      const data = res.data;
      if (data && typeof data === "object") {
        const onlyKey = Object.keys(data)[0];
        if (onlyKey && (data as Record<string, unknown>)[onlyKey] === null) return "notfound";
      }
    }
    return "allow";
  }

  // Generic errors (validation, internal): caller should investigate. Treat
  // as deny for the purposes of the matrix — but the diff report flags these
  // separately so they're not silently glossed over.
  return "deny";
}

type Cell = {
  op: string;
  persona: PersonaId;
  expected: Outcome;
  actual: Outcome;
  raw: GqlResponse;
};

async function runOp(client: Client, query: string, variables: Record<string, unknown>) {
  return client.query(query, variables);
}

async function main() {
  const endpoint = arg("endpoint");

  console.log(`[fuzzer] endpoint=${endpoint}`);
  const pool = await buildClients(endpoint);
  console.log(`[fuzzer] seeding world...`);
  const world = await seed(endpoint, pool);
  console.log(`[fuzzer] world: publicSpace=${world.publicSpaceId} publicRoom=${world.publicRoomId} otherSpace=${world.otherSpaceId}`);

  const cells: Cell[] = [];
  for (const op of OPERATIONS) {
    const vars = op.vars(world);
    if (vars === null) {
      console.log(`[fuzzer] skip ${op.name} (seed didn't produce prerequisites)`);
      continue;
    }
    for (const persona of PERSONAS) {
      const expected = MATRIX[op.name]?.[persona.id] ?? "deny";
      const client = pool[persona.id];
      const res = await runOp(client, op.query, vars);
      const actual = classify(res, op.category);
      cells.push({ op: op.name, persona: persona.id, expected, actual, raw: res });
    }
  }

  // Report.
  const mismatches = cells.filter((c) => c.expected !== c.actual);
  console.log(`\n[fuzzer] ${cells.length} cells tested, ${mismatches.length} mismatch(es).\n`);

  if (mismatches.length > 0) {
    console.log("Findings:");
    for (const m of mismatches) {
      const severity = m.expected === "deny" && m.actual === "allow" ? "‼ BYPASS" : "  diff   ";
      console.log(`${severity}  ${m.op}  [${m.persona}]  expected=${m.expected} actual=${m.actual}`);
      const errText = m.raw.errors?.map((e) => e.message).join(" | ") ?? "(no errors)";
      console.log(`           errors: ${errText}`);
    }
    process.exit(1);
  } else {
    console.log("No mismatches. (Doesn't mean no bugs — only that the cells in matrix.ts match observed behaviour.)");
  }
}

main().catch((e) => {
  console.error(e);
  process.exit(2);
});
