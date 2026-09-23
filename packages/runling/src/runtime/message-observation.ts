// Associate a task read with an optional agent receipt without changing its value.
const receipts = new WeakMap<object, (consumed: boolean) => void>();

export function observeMessageReceipt(
  result: object,
  report: (consumed: boolean) => void,
): void {
  receipts.set(result, report);
}

export function reportMessageReceipt(result: object, consumed: boolean): void {
  receipts.get(result)?.(consumed);
  receipts.delete(result);
}
