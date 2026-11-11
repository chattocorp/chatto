---
type: feature
---

Add `TerminationReason`, which reports whether an error came from `TerminateDelivery`. Applications that run a `DurableDeliveryHandler` outside a `DurableWorker` can use it to keep the same non-retryable classification.
