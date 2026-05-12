// Personas the fuzzer impersonates. Each gets its own Client (cookie jar).
//
// IMPORTANT: keep this list ordered from least to most privileged so the seed
// step can build relationships incrementally (e.g., spaceAdmin needs to be
// promoted from spaceMember).

export type PersonaId =
  | "anon"
  | "randomUser"
  | "spaceMember"
  | "roomMember"
  | "moderator"
  | "spaceAdmin"
  | "otherSpaceOwner"
  | "instanceAdmin";

export type Persona = {
  id: PersonaId;
  // Login/password used to create + authenticate the persona. instanceAdmin's
  // email must match an entry in chatto.toml [admin].emails for the test
  // instance — otherwise it degrades to a regular user and admin checks
  // appear "to pass" while actually never running.
  login: string;
  email: string;
  password: string;
  // Human-readable description for diff output.
  description: string;
};

export const PERSONAS: Persona[] = [
  {
    id: "anon",
    login: "",
    email: "",
    password: "",
    description: "Unauthenticated network attacker (no session, no token)",
  },
  {
    id: "randomUser",
    login: "fuzz_random",
    email: "fuzz_random@example.test",
    password: "fuzz-random-pw-1",
    description: "Authenticated user, member of nothing",
  },
  {
    id: "spaceMember",
    login: "fuzz_smember",
    email: "fuzz_smember@example.test",
    password: "fuzz-smember-pw-1",
    description: "Member of seed.publicSpace, default role",
  },
  {
    id: "roomMember",
    login: "fuzz_rmember",
    email: "fuzz_rmember@example.test",
    password: "fuzz-rmember-pw-1",
    description: "spaceMember + joined seed.publicRoom",
  },
  {
    id: "moderator",
    login: "fuzz_mod",
    email: "fuzz_mod@example.test",
    password: "fuzz-mod-pw-1",
    description:
      "Holds the system `moderator` role — outranks members but lacks role.assign. " +
      "Exists to catch the #435 rank-only authorization bug: any mutation that " +
      "gates on rank alone (without permission) will mis-allow this persona.",
  },
  {
    id: "spaceAdmin",
    login: "fuzz_sadmin",
    email: "fuzz_sadmin@example.test",
    password: "fuzz-sadmin-pw-1",
    description: "Admin role on seed.publicSpace",
  },
  {
    id: "otherSpaceOwner",
    login: "fuzz_other",
    email: "fuzz_other@example.test",
    password: "fuzz-other-pw-1",
    description: "Owner of a different space (cross-tenant probe)",
  },
  {
    id: "instanceAdmin",
    login: "fuzz_iadmin",
    // MUST be present in owners.emails on the test instance.
    email: "fuzz_iadmin@example.test",
    password: "fuzz-iadmin-pw-1",
    description: "Email matches owners.emails",
  },
];
