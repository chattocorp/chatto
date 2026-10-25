package connectapi

import (
	"errors"
	"net/http"
	"sync"
	"time"

	"connectrpc.com/authn"
	"connectrpc.com/connect"
	"github.com/charmbracelet/log"
	"github.com/chattocorp/chatto/cli/internal/pb/chatto/api/v1/apiv1connect"
)

const (
	// maxConcurrentUploadChunks limits the UploadChunk requests that one
	// process reads at the same time. Connect reads a complete request into
	// memory before the handler runs, and storage makes more copies. Thus, one
	// request can use several times core.DefaultAssetUploadChunkSize. Memory is
	// per process, so the limit is per process too. The trusted local operator
	// API has no limit.
	maxConcurrentUploadChunks = 4
	// maxConcurrentUploadChunksPerUser keeps one user from holding all slots.
	// Two slots let a user upload two files in parallel. Each bot is a
	// separate user.
	maxConcurrentUploadChunksPerUser = 2
	// uploadChunkReadTimeout is the time that a request can hold a slot while
	// its body arrives. A 10 MiB chunk needs approximately 0.3 Mbit/s.
	uploadChunkReadTimeout = 5 * time.Minute
)

// uploadChunkLimiter makes UploadChunk requests wait for a slot before
// Connect reads their body. Other procedures pass through.
type uploadChunkLimiter struct {
	next        http.Handler
	slots       chan struct{}
	perUser     int
	readTimeout time.Duration
	errorWriter *connect.ErrorWriter

	warnDeadlineOnce sync.Once

	mu    sync.Mutex
	users map[string]*userUploadSlots
}

// userUploadSlots holds the slots of one user. refs counts the requests that
// hold or wait for a slot, so that the limiter can delete unused entries.
type userUploadSlots struct {
	slots chan struct{}
	refs  int
}

func newUploadChunkLimiter(next http.Handler, options ...connect.HandlerOption) *uploadChunkLimiter {
	return &uploadChunkLimiter{
		next:        next,
		slots:       make(chan struct{}, maxConcurrentUploadChunks),
		perUser:     maxConcurrentUploadChunksPerUser,
		readTimeout: uploadChunkReadTimeout,
		errorWriter: connect.NewErrorWriter(options...),
		users:       make(map[string]*userUploadSlots),
	}
}

// ServeHTTP waits for a slot of the user and then for a slot of the process.
// A request that ends while it waits gets an Unavailable error, and the client
// can send the chunk again. A request that gets a slot must send its body
// within readTimeout.
func (l *uploadChunkLimiter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != apiv1connect.AssetUploadServiceUploadChunkProcedure {
		l.next.ServeHTTP(w, r)
		return
	}
	// The authentication middleware runs first. Unauthenticated requests
	// share one entry, and the handler rejects them.
	caller, _ := authn.GetInfo(r.Context()).(Caller)
	user := l.acquireUser(caller.UserID)
	defer l.releaseUser(caller.UserID, user)

	select {
	case user.slots <- struct{}{}:
		defer func() { <-user.slots }()
	case <-r.Context().Done():
		l.writeUnavailable(w, r)
		return
	}
	select {
	case l.slots <- struct{}{}:
		defer func() { <-l.slots }()
	case <-r.Context().Done():
		l.writeUnavailable(w, r)
		return
	}
	// Without a deadline, a slow client could hold its slot without limit.
	// net/http removes the deadline when the body is complete, so the deadline
	// does not limit the time that the handler needs to store the chunk.
	if err := http.NewResponseController(w).SetReadDeadline(time.Now().Add(l.readTimeout)); err != nil {
		l.warnDeadlineOnce.Do(func() {
			log.Warn("Upload chunk requests have no read deadline", "error", err)
		})
	}
	l.next.ServeHTTP(w, r)
}

func (l *uploadChunkLimiter) acquireUser(userID string) *userUploadSlots {
	l.mu.Lock()
	defer l.mu.Unlock()
	user, ok := l.users[userID]
	if !ok {
		user = &userUploadSlots{slots: make(chan struct{}, l.perUser)}
		l.users[userID] = user
	}
	user.refs++
	return user
}

func (l *uploadChunkLimiter) releaseUser(userID string, user *userUploadSlots) {
	l.mu.Lock()
	defer l.mu.Unlock()
	user.refs--
	if user.refs == 0 {
		delete(l.users, userID)
	}
}

func (l *uploadChunkLimiter) writeUnavailable(w http.ResponseWriter, r *http.Request) {
	_ = l.errorWriter.Write(w, r, connect.NewError(connect.CodeUnavailable, errors.New("upload chunk was not accepted before the request ended")))
}
