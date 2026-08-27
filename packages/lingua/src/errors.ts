export class LinguaError extends Error {
  override readonly name: string = "LinguaError";
}

export class CatalogValidationError extends LinguaError {
  override readonly name = "CatalogValidationError";
}

export class InterpolationError extends LinguaError {
  override readonly name = "InterpolationError";
}

export class TranslationKindError extends LinguaError {
  override readonly name = "TranslationKindError";
}
