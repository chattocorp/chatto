// Package memlimit sets the Go soft memory limit from the memory limit of the
// container.
//
// Go does not read the cgroup memory limit. With the default GOGC, the heap
// can grow to about twice the live data before a garbage collection. In a
// small container, the kernel can stop the process before Go frees memory
// that it does not need. A Go memory limit below the container limit makes
// Go collect garbage earlier.
package memlimit

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"runtime/debug"
	"strconv"
	"strings"
)

// containerShare is the part of the container memory limit that Go can use.
// The remainder is for memory that the container limit counts but the Go limit
// does not, such as the program code and the file cache of the process.
const containerShare = 0.9

// unlimited is the lowest value that counts as no limit. cgroup v1 reports a
// missing limit as a very large number, not as "max".
const unlimited = 1 << 62

// limitFiles are the cgroup v2 and cgroup v1 memory limit files of the
// process's own cgroup, as a container sees them.
var limitFiles = []string{
	"/sys/fs/cgroup/memory.max",
	"/sys/fs/cgroup/memory/memory.limit_in_bytes",
}

// FromContainer sets the Go soft memory limit to 90% of the container memory
// limit. It makes no change when the GOMEMLIMIT environment variable is set or
// when no container memory limit is found. It returns the limit that it set,
// or 0 when it made no change.
func FromContainer() (int64, error) {
	if os.Getenv("GOMEMLIMIT") != "" {
		return 0, nil
	}
	containerLimit, err := containerMemoryLimit(limitFiles)
	if err != nil || containerLimit == 0 {
		return 0, err
	}
	goLimit := goMemoryLimit(containerLimit)
	debug.SetMemoryLimit(goLimit)
	return goLimit, nil
}

// containerMemoryLimit returns the limit from the first file in files that
// exists, or 0 when no file exists or the limit is "max" or unlimited.
func containerMemoryLimit(files []string) (int64, error) {
	for _, file := range files {
		raw, err := os.ReadFile(file)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return 0, fmt.Errorf("read container memory limit: %w", err)
		}
		value := strings.TrimSpace(string(raw))
		if value == "max" {
			return 0, nil
		}
		limit, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("parse container memory limit %q: %w", value, err)
		}
		if limit <= 0 || limit >= unlimited {
			return 0, nil
		}
		return limit, nil
	}
	return 0, nil
}

func goMemoryLimit(containerLimit int64) int64 {
	return int64(float64(containerLimit) * containerShare)
}
