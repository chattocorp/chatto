export interface ResourceOperationTarget {
  resourceId: string;
  generation: number;
}

export function isCurrentResourceOperation(
  target: ResourceOperationTarget,
  currentResourceId: string,
  currentGeneration: number
): boolean {
  return target.resourceId === currentResourceId && target.generation === currentGeneration;
}
