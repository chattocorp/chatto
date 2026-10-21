export class TranslattoError extends Error {
  override readonly name: string = 'TranslattoError';
}

export class CatalogValidationError extends TranslattoError {
  override readonly name = 'CatalogValidationError';
}

export class InterpolationError extends TranslattoError {
  override readonly name = 'InterpolationError';
}

export class TranslationKindError extends TranslattoError {
  override readonly name = 'TranslationKindError';
}
