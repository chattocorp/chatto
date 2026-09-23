import { Type, type Static } from "runling";

/** A proposed change, not permission to implement it or proof that it works. */
export const planContentSchema = Type.Object({
  goal: Type.String({ minLength: 1, maxLength: 2000 }),
  steps: Type.Array(Type.Object({
    files: Type.Array(Type.String({ minLength: 1, maxLength: 500 }), { maxItems: 10 }),
    change: Type.String({ minLength: 1, maxLength: 2000 }),
  }), { minItems: 1, maxItems: 10 }),
  acceptanceCriteria: Type.Array(Type.String({ minLength: 1, maxLength: 1000 }), { minItems: 1, maxItems: 10 }),
  checks: Type.Array(Type.String({ minLength: 1, maxLength: 500 }), { minItems: 1, maxItems: 10 }),
  openQuestions: Type.Array(Type.String({ minLength: 1, maxLength: 1000 }), { maxItems: 10 }),
});
export const implementationPlanSchema = Type.Object({
  ...planContentSchema.properties,
  baseCommit: Type.String(),
});
export type ImplementationPlan = Static<typeof implementationPlanSchema>;
/** Owned by one conversation; never shared across users or persisted as a global cache. */
export type InvestigationPlans = Map<string, ImplementationPlan>;
