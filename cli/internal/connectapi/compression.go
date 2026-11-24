package connectapi

import (
	"compress/gzip"
	"io"

	"connectrpc.com/connect"
)

// idleGzipWriterLimit is the number of idle gzip writers that the process
// keeps for Connect responses. A writer at the default level holds about
// 1 MiB after its first write, so this limit is also the retained memory in
// MiB. A request that finds no idle writer makes a new one. If the free list
// is full when that writer is released, the garbage collector frees it.
const idleGzipWriterLimit = 4

// idleGzipWriters is the process-wide free list of gzip writers for Connect
// responses. Unlike a sync.Pool, it keeps its writers across garbage
// collections.
var idleGzipWriters = make(chan *gzip.Writer, idleGzipWriterLimit)

// sharedGzipCompression registers gzip for every Chatto Connect handler. The
// algorithm, level, and negotiation are the same as Connect's default gzip
// support. Only the reuse of writers is different.
//
// Connect keeps a sync.Pool of compressors for each procedure handler, and
// Chatto mounts more than 250 procedures. A sync.Pool keeps a private entry
// for each processor (GOMAXPROCS), which other processors cannot take, and it
// drops entries that stay unused for two garbage collections. With the
// default compressor, each procedure thus kept its own idle writers of about
// 1 MiB, often one or more for each processor that served it, and a procedure
// that was called less often than the collector ran made a new writer for
// each compressed response.
//
// The compressor that Connect pools here is a small pooledGzipWriter. It gets
// a real gzip writer from idleGzipWriters for each response and releases it
// in Close, so all procedures share at most idleGzipWriterLimit idle writers.
var sharedGzipCompression = connect.WithCompression(
	"gzip",
	func() connect.Decompressor { return &gzip.Reader{} },
	func() connect.Compressor { return &pooledGzipWriter{dst: io.Discard} },
)

// pooledGzipWriter is a connect.Compressor that holds a gzip writer only
// between its first Write and Close. Connect calls Reset with the response
// buffer, writes the message, and calls Close before it returns the
// pooledGzipWriter to its pool.
type pooledGzipWriter struct {
	dst    io.Writer
	writer *gzip.Writer
}

// Write compresses p to the current destination. The first Write after Reset
// or Close gets a gzip writer from the free list.
func (w *pooledGzipWriter) Write(p []byte) (int, error) {
	return w.acquire().Write(p)
}

// Close completes the gzip stream and releases the gzip writer. It does not
// close the destination. A Close without a preceding Write writes an empty
// gzip stream, as gzip.Writer does.
func (w *pooledGzipWriter) Close() error {
	writer := w.acquire()
	err := writer.Close()
	w.release()
	return err
}

// Reset sets the destination for the next stream and releases a gzip writer
// that an incomplete stream still holds.
func (w *pooledGzipWriter) Reset(dst io.Writer) {
	w.release()
	w.dst = dst
}

func (w *pooledGzipWriter) acquire() *gzip.Writer {
	if w.writer != nil {
		return w.writer
	}
	select {
	case writer := <-idleGzipWriters:
		writer.Reset(w.dst)
		w.writer = writer
	default:
		w.writer = gzip.NewWriter(w.dst)
	}
	return w.writer
}

func (w *pooledGzipWriter) release() {
	if w.writer == nil {
		return
	}
	// Do not keep a reference to the response buffer while idle.
	w.writer.Reset(io.Discard)
	select {
	case idleGzipWriters <- w.writer:
	default:
	}
	w.writer = nil
}
