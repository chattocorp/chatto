---
type: feature
---

Operators can rebuild the search index from the message history and keep the current index for diagnosis: stop the provider, run `chatto search-provider recover`, then start the provider again. The command refuses when the volume does not have free space for 1.5 times the index. A restart during the rebuild continues from its checkpoint, and the provider answers searches only after a real test query on the new index succeeds. Chatto keeps one previous index until the next recovery or until you delete it with `chatto search-provider discard-previous`. See [Rebuild the search index](https://docs.chatto.run/guides/operations/search/#rebuild-the-search-index).
