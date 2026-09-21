import { randomInt } from "node:crypto";
import { adjectives, nouns } from "human-id";

export function randomId() {
  const adjective = adjectives[randomInt(adjectives.length)];
  const noun = nouns[randomInt(nouns.length)];
  return `${adjective}-${noun}-${randomInt(1_000, 10_000)}`;
}
