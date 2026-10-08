// Package intern maps opaque strings to compact process-local handles.
//
// Table is for a single owner that supplies synchronization. ConcurrentTable
// can be shared by models that use independent locks. Neither type owns domain
// state, event-log positions, or persistence. Both return ID[Kind] values, reserve
// zero for the empty string, and retain IDs until the table is released.
//
// A handle is valid only in the table that issued it. Applications must keep
// tables and their handle-indexed state together. Persist strings and intern
// them again on restore; insertion order can differ between processes. Use
// Lookup for read-only queries so unknown IDs do not grow the table.
//
// These tables are intended for opaque identifiers. They are not secure stores
// and cannot erase individual entries. Do not intern secrets or personal data
// that must be removed during the table's lifetime.
package intern
