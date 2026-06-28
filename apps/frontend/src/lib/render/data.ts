import type { RenderDocument } from './types';

export type RenderType<TDocumentType> =
  TDocumentType extends RenderDocument<infer TType> ? TType : never;

export function useRenderData<TType>(
  _documentNode: RenderDocument<TType>,
  renderData: TType
): TType;
export function useRenderData<TType>(
  _documentNode: RenderDocument<TType>,
  renderData: TType | undefined
): TType | undefined;
export function useRenderData<TType>(
  _documentNode: RenderDocument<TType>,
  renderData: TType | null
): TType | null;
export function useRenderData<TType>(
  _documentNode: RenderDocument<TType>,
  renderData: TType | null | undefined
): TType | null | undefined;
export function useRenderData<TType>(
  _documentNode: RenderDocument<TType>,
  renderData: readonly TType[]
): readonly TType[];
export function useRenderData<TType>(
  _documentNode: RenderDocument<TType>,
  renderData: readonly TType[] | null | undefined
): readonly TType[] | null | undefined;
export function useRenderData<TType>(
  _documentNode: RenderDocument<TType>,
  renderData: unknown
): TType;
export function useRenderData<TType>(
  _documentNode: RenderDocument<TType>,
  renderData: TType | readonly TType[] | null | undefined
): TType | readonly TType[] | null | undefined {
  return renderData;
}

export function makeRenderData<TType>(
  data: TType,
  _document: RenderDocument<TType>
): TType {
  return data;
}
