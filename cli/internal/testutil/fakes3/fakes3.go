package fakes3

import (
	"bufio"
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type Server struct {
	*httptest.Server
}

// NewServer starts a small in-memory, path-style S3-compatible server for
// tests that exercise Chatto's AWS SDK-backed S3 client. It implements only the
// bucket and object operations the codebase uses in tests.
func NewServer(t testing.TB) *Server {
	t.Helper()

	store := &objectStore{
		buckets: make(map[string]map[string]object),
	}
	server := httptest.NewServer(store)
	t.Cleanup(server.Close)

	return &Server{Server: server}
}

func (s *Server) EndpointHost() string {
	return strings.TrimPrefix(s.URL, "http://")
}

type objectStore struct {
	mu      sync.RWMutex
	buckets map[string]map[string]object
}

type object struct {
	body        []byte
	contentType string
	etag        string
	modified    time.Time
}

func (s *objectStore) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	bucket, key, ok := splitPath(r.URL)
	if !ok {
		writeS3Error(w, http.StatusBadRequest, "InvalidURI", "invalid S3 path")
		return
	}

	if key == "" {
		s.handleBucket(w, r, bucket)
		return
	}
	s.handleObject(w, r, bucket, key)
}

func splitPath(u *url.URL) (bucket string, key string, ok bool) {
	path := strings.TrimPrefix(u.EscapedPath(), "/")
	if path == "" {
		return "", "", false
	}

	bucketPart, keyPart, found := strings.Cut(path, "/")
	bucket, err := url.PathUnescape(bucketPart)
	if err != nil || bucket == "" {
		return "", "", false
	}
	if !found {
		return bucket, "", true
	}
	key, err = url.PathUnescape(keyPart)
	if err != nil {
		return "", "", false
	}
	return bucket, key, true
}

func hasQueryKey(u *url.URL, key string) bool {
	for part := range strings.SplitSeq(u.RawQuery, "&") {
		name, _, _ := strings.Cut(part, "=")
		if name == key {
			return true
		}
	}
	return false
}

func (s *objectStore) handleBucket(w http.ResponseWriter, r *http.Request, bucket string) {
	switch r.Method {
	case http.MethodHead, http.MethodGet:
		s.mu.RLock()
		_, exists := s.buckets[bucket]
		s.mu.RUnlock()
		if !exists {
			writeS3Error(w, http.StatusNotFound, "NoSuchBucket", "bucket does not exist")
			return
		}
		if r.Method == http.MethodGet && hasQueryKey(r.URL, "location") {
			w.Header().Set("Content-Type", "application/xml")
			_, _ = io.WriteString(w, `<LocationConstraint xmlns="http://s3.amazonaws.com/doc/2006-03-01/"></LocationConstraint>`)
			return
		}
		if r.Method == http.MethodGet && r.URL.Query().Get("list-type") == "2" {
			s.writeListObjectsV2(w, bucket, r.URL.Query().Get("prefix"))
			return
		}
		if r.Method == http.MethodGet && hasQueryKey(r.URL, "versions") {
			s.writeListObjectVersions(w, bucket, r.URL.Query().Get("prefix"))
			return
		}
		w.WriteHeader(http.StatusOK)
	case http.MethodPut:
		s.mu.Lock()
		if _, exists := s.buckets[bucket]; !exists {
			s.buckets[bucket] = make(map[string]object)
		}
		s.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	default:
		w.Header().Set("Allow", "HEAD, GET, PUT")
		writeS3Error(w, http.StatusMethodNotAllowed, "MethodNotAllowed", "method not allowed")
	}
}

func (s *objectStore) handleObject(w http.ResponseWriter, r *http.Request, bucket, key string) {
	switch r.Method {
	case http.MethodPut:
		body, err := readPutBody(r)
		if err != nil {
			writeS3Error(w, http.StatusInternalServerError, "InternalError", "failed to read request body")
			return
		}
		sum := md5.Sum(body)
		obj := object{
			body:        body,
			contentType: r.Header.Get("Content-Type"),
			etag:        hex.EncodeToString(sum[:]),
			modified:    time.Now().UTC(),
		}

		s.mu.Lock()
		objects, exists := s.buckets[bucket]
		if !exists {
			s.mu.Unlock()
			writeS3Error(w, http.StatusNotFound, "NoSuchBucket", "bucket does not exist")
			return
		}
		objects[key] = obj
		s.mu.Unlock()

		w.Header().Set("ETag", `"`+obj.etag+`"`)
		w.WriteHeader(http.StatusOK)
	case http.MethodGet, http.MethodHead:
		obj, exists := s.getObject(bucket, key)
		if !exists {
			writeS3Error(w, http.StatusNotFound, "NoSuchKey", "object does not exist")
			return
		}
		setObjectHeaders(w, obj)
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return
		}
		_, _ = w.Write(obj.body)
	case http.MethodDelete:
		s.mu.Lock()
		if objects, exists := s.buckets[bucket]; exists {
			delete(objects, key)
		}
		s.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	default:
		w.Header().Set("Allow", "PUT, GET, HEAD, DELETE")
		writeS3Error(w, http.StatusMethodNotAllowed, "MethodNotAllowed", "method not allowed")
	}
}

func readPutBody(r *http.Request) ([]byte, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	if !isAWSChunked(r) {
		return body, nil
	}
	return decodeAWSChunked(body)
}

func isAWSChunked(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Content-Encoding"), "aws-chunked") ||
		strings.Contains(r.Header.Get("X-Amz-Content-Sha256"), "STREAMING-")
}

func decodeAWSChunked(encoded []byte) ([]byte, error) {
	reader := bufio.NewReader(bytes.NewReader(encoded))
	var decoded bytes.Buffer
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		sizeText, _, _ := strings.Cut(line, ";")
		size, err := strconv.ParseInt(sizeText, 16, 64)
		if err != nil {
			return nil, err
		}
		if size == 0 {
			return decoded.Bytes(), nil
		}
		if _, err := io.CopyN(&decoded, reader, size); err != nil {
			return nil, err
		}
		if _, err := reader.Discard(2); err != nil {
			return nil, err
		}
	}
}

func (s *objectStore) writeListObjectsV2(w http.ResponseWriter, bucket, prefix string) {
	objects := s.listObjects(bucket, prefix)
	w.Header().Set("Content-Type", "application/xml")
	_ = xml.NewEncoder(w).Encode(listObjectsV2Result{
		Name:        bucket,
		Prefix:      prefix,
		IsTruncated: false,
		KeyCount:    len(objects),
		Contents:    objects,
	})
}

func (s *objectStore) writeListObjectVersions(w http.ResponseWriter, bucket, prefix string) {
	objects := s.listObjects(bucket, prefix)
	versions := make([]objectVersionResult, 0, len(objects))
	for _, object := range objects {
		versions = append(versions, objectVersionResult{
			Key:          object.Key,
			Size:         object.Size,
			IsLatest:     true,
			VersionID:    "null",
			LastModified: object.LastModified,
		})
	}

	w.Header().Set("Content-Type", "application/xml")
	_ = xml.NewEncoder(w).Encode(listObjectVersionsResult{
		Name:        bucket,
		Prefix:      prefix,
		IsTruncated: false,
		Versions:    versions,
	})
}

func (s *objectStore) listObjects(bucket, prefix string) []objectResult {
	s.mu.RLock()
	defer s.mu.RUnlock()

	objects := s.buckets[bucket]
	keys := make([]string, 0, len(objects))
	for key := range objects {
		if strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)

	results := make([]objectResult, 0, len(keys))
	for _, key := range keys {
		obj := objects[key]
		results = append(results, objectResult{
			Key:          key,
			Size:         len(obj.body),
			LastModified: obj.modified.Format(time.RFC3339),
			ETag:         `"` + obj.etag + `"`,
		})
	}
	return results
}

func (s *objectStore) getObject(bucket, key string) (object, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	objects, exists := s.buckets[bucket]
	if !exists {
		return object{}, false
	}
	obj, exists := objects[key]
	return obj, exists
}

func setObjectHeaders(w http.ResponseWriter, obj object) {
	if obj.contentType != "" {
		w.Header().Set("Content-Type", obj.contentType)
	}
	w.Header().Set("Content-Length", intToString(len(obj.body)))
	w.Header().Set("ETag", `"`+obj.etag+`"`)
	w.Header().Set("Last-Modified", obj.modified.Format(http.TimeFormat))
}

func intToString(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

type errorResponse struct {
	XMLName xml.Name `xml:"Error"`
	Code    string   `xml:"Code"`
	Message string   `xml:"Message"`
}

type listObjectsV2Result struct {
	XMLName     xml.Name       `xml:"ListBucketResult"`
	Name        string         `xml:"Name"`
	Prefix      string         `xml:"Prefix"`
	KeyCount    int            `xml:"KeyCount"`
	IsTruncated bool           `xml:"IsTruncated"`
	Contents    []objectResult `xml:"Contents"`
}

type listObjectVersionsResult struct {
	XMLName     xml.Name              `xml:"ListVersionsResult"`
	Name        string                `xml:"Name"`
	Prefix      string                `xml:"Prefix"`
	IsTruncated bool                  `xml:"IsTruncated"`
	Versions    []objectVersionResult `xml:"Version"`
}

type objectResult struct {
	Key          string `xml:"Key"`
	LastModified string `xml:"LastModified"`
	ETag         string `xml:"ETag"`
	Size         int    `xml:"Size"`
}

type objectVersionResult struct {
	Key          string `xml:"Key"`
	VersionID    string `xml:"VersionId"`
	IsLatest     bool   `xml:"IsLatest"`
	LastModified string `xml:"LastModified"`
	ETag         string `xml:"ETag,omitempty"`
	Size         int    `xml:"Size"`
}

func writeS3Error(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(status)
	if status != http.StatusNoContent && status != http.StatusNotModified {
		_ = xml.NewEncoder(w).Encode(errorResponse{
			Code:    code,
			Message: message,
		})
	}
}
