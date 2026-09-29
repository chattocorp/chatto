package core

import (
	"math/bits"
	"time"
	"unsafe"
)

// Cold storage keeps old rows of large per-message projection structures in
// compact, read-only blocks in RAM (ADR-111). A read of a cold row decodes it
// synchronously, so projections keep their APIs, their results, and their
// snapshot formats. Freezing changes only the memory layout.

// coldBlockShift sets the number of rows in one frozen block to
// 1<<coldBlockShift. Tests lower it to freeze small projections.
var coldBlockShift = 8

// coldStorageTestWindow replaces every configured cold window when it is not
// zero. Only tests set it, to run projections with almost all rows frozen.
var coldStorageTestWindow time.Duration

// coldWindowOrTest returns the cold window that a projection uses.
func coldWindowOrTest(window time.Duration) time.Duration {
	if coldStorageTestWindow != 0 {
		return coldStorageTestWindow
	}
	return window
}

// coldBlockRows returns the number of rows in one frozen block.
func coldBlockRows() int {
	return 1 << coldBlockShift
}

// coldMaxColumns is the largest number of integer columns that a codec can
// use. Decoding uses a fixed array of this size, so it does not allocate.
const coldMaxColumns = 12

// coldFields holds the integer columns of one row.
type coldFields [coldMaxColumns]uint64

// coldCodec converts one row value to integer columns and back. Decode must
// return a value equal to the encoded value.
type coldCodec[T any] interface {
	columns() int
	encode(value T) coldFields
	decode(fields coldFields) T
}

// packedColumns stores the rows of one frozen block column by column. Each
// column keeps its smallest value and stores every value as an unsigned
// offset from it, with the smallest bit width that holds the largest offset.
// A column whose values are all equal uses no bits. The structure is
// immutable after construction, so concurrent readers need no lock.
type packedColumns struct {
	mins   []uint64
	widths []uint8
	// starts holds the bit position of each column in words.
	starts []uint32
	words  []uint64
}

// packColumns packs rows of the given column count. values[row][column]
// holds the input.
func packColumns(columns int, values []coldFields) packedColumns {
	packed := packedColumns{
		mins:   make([]uint64, columns),
		widths: make([]uint8, columns),
		starts: make([]uint32, columns),
	}
	var totalBits uint64
	for column := range columns {
		lowest, highest := ^uint64(0), uint64(0)
		for row := range values {
			value := values[row][column]
			lowest = min(lowest, value)
			highest = max(highest, value)
		}
		if len(values) == 0 {
			lowest = 0
		}
		packed.mins[column] = lowest
		packed.widths[column] = uint8(bits.Len64(highest - lowest))
		packed.starts[column] = uint32(totalBits)
		totalBits += uint64(packed.widths[column]) * uint64(len(values))
	}
	// One extra word lets a read of the last value span two words safely.
	packed.words = make([]uint64, (totalBits+63)/64+1)
	for column := range columns {
		width := uint64(packed.widths[column])
		if width == 0 {
			continue
		}
		position := uint64(packed.starts[column])
		for row := range values {
			value := values[row][column] - packed.mins[column]
			word, shift := position/64, position%64
			packed.words[word] |= value << shift
			if shift+width > 64 {
				packed.words[word+1] |= value >> (64 - shift)
			}
			position += width
		}
	}
	return packed
}

// get returns the value of one row and column.
func (p *packedColumns) get(row, column int) uint64 {
	width := uint64(p.widths[column])
	if width == 0 {
		return p.mins[column]
	}
	position := uint64(p.starts[column]) + uint64(row)*width
	word, shift := position/64, position%64
	value := p.words[word] >> shift
	if shift+width > 64 {
		value |= p.words[word+1] << (64 - shift)
	}
	if width < 64 {
		value &= 1<<width - 1
	}
	return p.mins[column] + value
}

// estimatedBytes approximates the heap size of the packed block.
func (p *packedColumns) estimatedBytes() int64 {
	return int64(cap(p.mins))*8 + int64(cap(p.widths)) + int64(cap(p.starts))*4 + int64(cap(p.words))*8 +
		3*int64(unsafe.Sizeof([]uint64(nil)))
}

// coldSlice is a dense slice whose leading rows can be frozen into packed
// blocks. Frozen rows stay readable through get. A set of a frozen row stores
// the new value in a sparse overlay, because blocks never change.
//
// Callers synchronize access. Reads do not change the slice, so concurrent
// readers under a shared lock are safe.
type coldSlice[T comparable] struct {
	codec  coldCodec[T]
	blocks []*packedColumns
	// hot holds the rows after the frozen blocks.
	hot []T
	// changed holds frozen rows that a set replaced after freezing.
	changed map[int]T
}

func newColdSlice[T comparable](codec coldCodec[T]) coldSlice[T] {
	return coldSlice[T]{codec: codec}
}

// frozenLen returns the number of frozen rows.
func (s *coldSlice[T]) frozenLen() int {
	return len(s.blocks) << coldBlockShift
}

// len returns the number of rows.
func (s *coldSlice[T]) len() int {
	return s.frozenLen() + len(s.hot)
}

// get returns row i, or the zero value when i is out of range.
func (s *coldSlice[T]) get(i int) T {
	var zero T
	if i < 0 || i >= s.len() {
		return zero
	}
	frozen := s.frozenLen()
	if i >= frozen {
		return s.hot[i-frozen]
	}
	if value, ok := s.changed[i]; ok {
		return value
	}
	var fields coldFields
	block := s.blocks[i>>coldBlockShift]
	row := i & (coldBlockRows() - 1)
	for column := range s.codec.columns() {
		fields[column] = block.get(row, column)
	}
	return s.codec.decode(fields)
}

// set stores row i and extends the slice with zero values when necessary.
func (s *coldSlice[T]) set(i int, value T) {
	if i < 0 {
		return
	}
	frozen := s.frozenLen()
	if i < frozen {
		if s.changed == nil {
			s.changed = make(map[int]T)
		}
		s.changed[i] = value
		return
	}
	if missing := i - s.len() + 1; missing > 0 {
		s.hot = append(s.hot, make([]T, missing)...)
	}
	s.hot[i-frozen] = value
}

// append adds a row at the end.
func (s *coldSlice[T]) append(value T) {
	s.hot = append(s.hot, value)
}

// freezeBefore packs every complete block of hot rows below row n. Rows at
// or after the last complete block below n stay hot.
func (s *coldSlice[T]) freezeBefore(n int) {
	rows := coldBlockRows()
	target := min(n, s.len()) >> coldBlockShift
	if target <= len(s.blocks) {
		return
	}
	values := make([]coldFields, rows)
	columns := s.codec.columns()
	for len(s.blocks) < target {
		for row := range rows {
			values[row] = s.codec.encode(s.hot[row])
		}
		block := packColumns(columns, values)
		s.blocks = append(s.blocks, &block)
		s.hot = s.hot[rows:]
	}
	// Copy the hot rows so the frozen rows' backing array can be released.
	s.hot = append(make([]T, 0, len(s.hot)+len(s.hot)/4+1), s.hot...)
}

// estimatedBytes approximates the heap size of the slice.
func (s *coldSlice[T]) estimatedBytes() int64 {
	var zero T
	bytes := int64(cap(s.hot))*int64(unsafe.Sizeof(zero)) + int64(cap(s.blocks))*8
	for _, block := range s.blocks {
		bytes += block.estimatedBytes()
	}
	bytes += int64(len(s.changed)) * (projectionCompactMapEntryOverhead + 8 + int64(unsafe.Sizeof(zero)))
	return bytes
}

// coldHandleSlice stores one value per ID-table handle, like handleSlice, and
// can freeze the values of low handles. The zero value of T means "no value".
type coldHandleSlice[T comparable] struct {
	rows coldSlice[T]
}

func newColdHandleSlice[T comparable](codec coldCodec[T]) coldHandleSlice[T] {
	return coldHandleSlice[T]{rows: newColdSlice[T](codec)}
}

// get returns the value for handle and whether it is set.
func (s *coldHandleSlice[T]) get(handle uint32) (T, bool) {
	var zero T
	if handle == 0 {
		return zero, false
	}
	value := s.rows.get(int(handle) - 1)
	return value, value != zero
}

// set stores value for handle. Handle zero is ignored.
func (s *coldHandleSlice[T]) set(handle uint32, value T) {
	if handle == 0 {
		return
	}
	s.rows.set(int(handle)-1, value)
}

// len returns one more than the highest handle with a stored slot.
func (s *coldHandleSlice[T]) len() int {
	return s.rows.len()
}

// at returns the value of handle index+1, for iteration over all slots.
func (s *coldHandleSlice[T]) at(index int) T {
	return s.rows.get(index)
}

// freezeBelow freezes the complete blocks of handles below handle.
func (s *coldHandleSlice[T]) freezeBelow(handle uint32) {
	if handle > 1 {
		s.rows.freezeBefore(int(handle) - 1)
	}
}

// estimatedBytes approximates the heap size of the slice.
func (s *coldHandleSlice[T]) estimatedBytes() int64 {
	return s.rows.estimatedBytes()
}

// coldHandleBoundary returns the handle below which a component that follows
// watermark can freeze its state. In the cold test mode, a component without
// a watermark freezes all complete blocks.
func coldHandleBoundary(watermark *coldWatermark, slots int) uint32 {
	if watermark == nil && coldStorageTestWindow != 0 {
		return uint32(slots + 1)
	}
	return watermark.below()
}
