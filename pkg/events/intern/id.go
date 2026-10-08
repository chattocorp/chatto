package intern

// ID is a dense, one-based identifier in a Table or ConcurrentTable.
// Kind distinguishes application-defined namespaces at compile time. IDs of
// different kinds cannot be assigned or passed to a table without an explicit
// conversion. Each ID occupies four bytes and contains no pointers.
//
// Zero means no ID. A nonzero ID belongs to the table that issued it, even
// when another table has the same Kind. Numeric order is insertion order,
// not lexical order. The numeric representation supports dense slice indexing;
// do not construct IDs from unrelated numbers or convert between kinds.
//
// Resolve IDs through their owning table at API and persistence boundaries.
// Never serialize the numeric value: handles are not stable across processes
// or restores. There is deliberately no global registry or String method.
type ID[Kind any] uint32
