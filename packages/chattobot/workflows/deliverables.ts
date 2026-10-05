/** Structured investigation deliverables. An investigation always records checked findings; a
 * purpose composes the deliverables that it also requires or accepts. */
import { Type, type Static, type TSchema } from 'runling';
import { defineAgentExtension, type AgentExtension } from 'runling/agents';
import { planContentSchema, type ImplementationPlan } from './plan.ts';

const reasons = Type.Array(Type.String({ minLength: 1, maxLength: 1000 }), { maxItems: 10 });

/** A source-grounded answer to "can we do this, and what would it take?". Estimates are
 * hypotheses from read-only source inspection, not commitments. */
export const feasibilitySchema = Type.Object({
  verdict: Type.Union(
    [
      Type.Literal('already_supported'),
      Type.Literal('feasible'),
      Type.Literal('feasible_with_caveats'),
      Type.Literal('needs_product_decision'),
      Type.Literal('not_feasible')
    ],
    {
      description:
        'already_supported: existing behavior meets the request. needs_product_decision: the source allows it, but a product choice blocks a plan.'
    }
  ),
  summary: Type.String({ minLength: 1, maxLength: 2000 }),
  size: Type.Optional(
    Type.Union([Type.Literal('small'), Type.Literal('medium'), Type.Literal('large')], {
      description: 'Estimated size of the change. A hypothesis; omit it when nothing must change.'
    })
  ),
  affectedAreas: Type.Array(Type.String({ minLength: 1, maxLength: 200 }), {
    maxItems: 10,
    description:
      'Affected areas, for example the Chatto frontend, Chatto backend, public API, persisted protobufs, Authling, or Runling'
  }),
  compatibilityRisks: Type.Array(Type.String({ minLength: 1, maxLength: 1000 }), {
    maxItems: 10,
    description: 'Public API, persisted data, upgrade, and mixed-version risks'
  }),
  risks: reasons,
  openDecisions: reasons
});
export type FeasibilityAssessment = Static<typeof feasibilitySchema>;

/** Values that deliverables add to an investigation result. */
export interface DeliveredValues {
  plan?: ImplementationPlan;
  feasibility?: FeasibilityAssessment;
}

/** A deliverable that is required but missing blocks the result with this reason. */
export type MissingDeliverable = 'missing_plan' | 'missing_feasibility';

/** One structured result that an investigator returns through a tool call. */
export interface Deliverable {
  /** The tool that the investigator calls to deliver the value. */
  readonly tool: string;
  readonly extension: AgentExtension;
  /** The delivered value as result fields, or nothing when it was not delivered. */
  delivered(): DeliveredValues | undefined;
  /** Investigator instruction when this deliverable is required. */
  readonly requiredInstruction: string;
  /** Investigator instruction when this deliverable is optional. */
  readonly optionalInstruction: string;
  /** Sentence for the repair turn when a required deliverable is missing. */
  readonly repair: string;
  readonly missingReason: MissingDeliverable;
  /** Result summary when a required deliverable is missing. Describes facts for the supervisor. */
  readonly missingSummary: string;
}

/** What a deliverable needs from its investigation. */
export interface DeliverableContext {
  baseCommit: string;
  /** True after the host accepted at least one finding. Deliverables must rest on evidence. */
  hasEvidence(): boolean;
}

/** Create one deliverable instance for one investigation. */
export type DeliverableFactory = (context: DeliverableContext) => Deliverable;

/** Register a tool that stores one schema-checked value, once findings exist. */
function valueDeliverable<Schema extends TSchema>(
  context: DeliverableContext,
  tool: { name: string; label: string; description: string; parameters: Schema },
  maxLength: number
) {
  let value: Static<Schema> | undefined;
  const extension = defineAgentExtension((pi) => {
    pi.registerTool({
      ...tool,
      async execute(_id, content) {
        if (!context.hasEvidence())
          throw new Error('Record source evidence with recordFinding first');
        if (JSON.stringify(content).length > maxLength)
          throw new Error(`Keep this below ${maxLength.toLocaleString('en')} characters`);
        value = structuredClone(content) as Static<Schema>;
        return {
          content: [{ type: 'text', text: 'Retained. Finish with report_outcome.' }],
          details: {}
        };
      }
    });
  });
  return { extension, value: () => value };
}

/** A typed implementation plan, retained by the conversation for a later implementation. */
export const planDeliverable: DeliverableFactory = (context) => {
  const { extension, value } = valueDeliverable(
    context,
    {
      name: 'prepareImplementationPlan',
      label: 'Prepare implementation plan',
      description:
        'Return a concise implementation plan grounded in the checked findings. Separate open product decisions from required changes. This does not authorize implementation.',
      parameters: planContentSchema
    },
    16_000
  );
  return {
    tool: 'prepareImplementationPlan',
    extension,
    delivered() {
      const plan = value();
      return plan && { plan: { ...plan, baseCommit: context.baseCommit } };
    },
    requiredInstruction:
      'Call prepareImplementationPlan after collecting the necessary evidence. Include concrete changes, acceptance criteria, checks, and open questions. Stop researching when you can supply a useful plan. Do not invent required state or complexity: check whether existing behavior already meets the requirement. The implementation worker will verify your plan against its checkout, not repeat the entire investigation.',
    optionalInstruction:
      'You can also call prepareImplementationPlan when the evidence supports a concrete plan; it is optional here.',
    repair: 'Call prepareImplementationPlan using your checked findings.',
    missingReason: 'missing_plan',
    missingSummary:
      'The investigator did not supply the requested implementation plan. Checked findings remain available.'
  };
};

/** A feasibility verdict: whether a change is possible, how large it is, and what it risks. */
export const feasibilityDeliverable: DeliverableFactory = (context) => {
  const { extension, value } = valueDeliverable(
    context,
    {
      name: 'reportFeasibility',
      label: 'Report feasibility',
      description:
        'Return a feasibility verdict grounded in the checked findings: whether the requested fix or feature is possible, whether it already exists, its estimated size, the affected areas, and its risks and open decisions. This does not authorize implementation.',
      parameters: feasibilitySchema
    },
    12_000
  );
  return {
    tool: 'reportFeasibility',
    extension,
    delivered() {
      const feasibility = value();
      return feasibility && { feasibility };
    },
    requiredInstruction:
      'Call reportFeasibility after collecting the necessary evidence. First check whether existing behavior already meets the request. Name the affected areas and products, and check public API and persisted protobuf compatibility (ADR-045) when the change touches them. Label size as an estimate. Put unresolved product choices in openDecisions, not in risks. Stop researching when the evidence supports a verdict.',
    optionalInstruction: '',
    repair: 'Call reportFeasibility using your checked findings.',
    missingReason: 'missing_feasibility',
    missingSummary:
      'The investigator did not supply the requested feasibility assessment. Checked findings remain available.'
  };
};

/** A deliverable in a purpose. Required deliverables get one repair turn and block the result
 * when they are still missing. */
export interface PurposeDeliverable {
  deliverable: DeliverableFactory;
  required: boolean;
}

/** Deliverables by investigation purpose. Findings are always required. */
export const PURPOSE_DELIVERABLES = {
  assessment: [{ deliverable: planDeliverable, required: false }],
  implementation: [{ deliverable: planDeliverable, required: true }],
  feasibility: [
    { deliverable: feasibilityDeliverable, required: true },
    { deliverable: planDeliverable, required: false }
  ]
} satisfies Record<string, readonly PurposeDeliverable[]>;
export type InvestigationPurpose = keyof typeof PURPOSE_DELIVERABLES;
