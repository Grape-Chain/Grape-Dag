package run

import (
	"bufio"
	"fmt"
	"os"
	"runtime/debug"
	"strconv"
	"strings"

	config "github.com/Grape-Chain/Grape-Dag/config"
	"github.com/pbnjay/memory"
)

/*
Garbage collection.

Nothing set GOGC or GOMEMLIMIT anywhere, so the node ran on the language
defaults: collect when the heap has doubled, and no memory ceiling at all. For a
process whose live heap is the ledger graph - hundreds of megabytes, and growing
with the graph between commit transactions - both halves of that are wrong in
opposite directions. Doubling a large live heap means collecting far more often
than the work justifies (GC was 40% of node CPU before unrelated fixes), and no
ceiling means the node's memory is whatever the arrival rate makes it.

The pair chosen here:

  - GOMEMLIMIT at 70% of the memory the process can actually use. This is a
    soft limit: the collector runs more often as the heap approaches it and
    never above it, which turns "the node grows until the kernel kills it" into
    "the node spends more CPU on GC under memory pressure". That is the right
    trade for a ledger, because a killed node loses its place in the network
    whereas a slow one does not.

  - GOGC raised to 200. With a limit in place the limit is the real trigger, so
    GOGC's job changes: it stops being the memory bound and becomes the
    frequency knob for the region below the limit. 200 means the heap grows to
    three times the live set before a cycle, which roughly halves the number of
    cycles compared with the default.

The alternative considered and rejected was GOGC=off with GOMEMLIMIT alone,
which the Go runtime guide offers for workloads with a steady live heap: it is
the most CPU-efficient setting, but if the live heap ever approaches the limit
the collector thrashes against it with no other trigger to fall back on. This
node's live heap grows with the graph and is trimmed in steps by slicing, so it
is exactly the shape that setting is not for.

Both are overridable. An operator who has set the runtime's own GOGC or
GOMEMLIMIT gets left alone entirely - the runtime has already read them, and
silently overriding an explicit instruction is worse than any default.
*/

const (
	// defaultGOGC - see the note above. Not a constant in the runtime's sense:
	// it is a starting point for a node whose live heap is large.
	defaultGOGC = 200
	// defaultMemoryLimitPercent - how much of the available memory the heap and
	// the rest of the Go runtime may use between them. The other 30% is for
	// everything GOMEMLIMIT does not account for (thread stacks, the pebble
	// store's own mappings, whatever else shares the container) plus the
	// headroom the collector needs to avoid thrashing at the limit.
	defaultMemoryLimitPercent = 70
)

// cgroupMemoryPaths - where a container's memory ceiling is published, v2 first.
// A parameter of cgroupMemoryLimit rather than a literal inside it so a test can
// point it at files it controls.
var cgroupMemoryPaths = []string{
	"/sys/fs/cgroup/memory.max",                   // v2
	"/sys/fs/cgroup/memory/memory.limit_in_bytes", // v1
}

type ProcessRuntimeTuning struct{}

func (p *ProcessRuntimeTuning) process(c *config.Grapepeer) error {
	tuneGC()
	return nil
}

// tuneGC - apply the GC settings, reporting what was chosen and why.
//
// Reporting matters more than usual here: a node that is slow because it is
// collecting, or dead because it was not limited, is diagnosed from this line.
func tuneGC() {
	if v, set := os.LookupEnv("GOGC"); set {
		logger.Infof("[gc] GOGC=%s is set; leaving the collector's target alone", v)
	} else {
		gogc := defaultGOGC
		if env, ok := os.LookupEnv("GRAPE_GOGC"); ok {
			if n, err := strconv.Atoi(env); err == nil && n > 0 {
				gogc = n
			} else {
				logger.Warnf("[gc] Ignoring GRAPE_GOGC=%q: expected a positive percentage", env)
			}
		}
		previous := debug.SetGCPercent(gogc)
		logger.Infof("[gc] GOGC set to %d (was %d): collect when the heap has grown %d%% beyond the live set",
			gogc, previous, gogc)
	}

	if v, set := os.LookupEnv("GOMEMLIMIT"); set {
		logger.Infof("[gc] GOMEMLIMIT=%s is set; leaving the soft memory limit alone", v)
		return
	}
	limit, why := memoryLimit()
	if limit <= 0 {
		logger.Warn("[gc] Cannot determine how much memory this node may use; running with no soft memory limit")
		return
	}
	debug.SetMemoryLimit(limit)
	logger.Infof("[gc] GOMEMLIMIT set to %d bytes (%d MiB): %s", limit, limit>>20, why)
}

// memoryLimit - the soft limit to hand the runtime, and a sentence explaining
// where it came from.
//
// GRAPE_MEMORY_LIMIT takes either a byte count or a percentage ("70%"), the
// percentage being of whatever the process is actually allowed.
func memoryLimit() (int64, string) {
	return memoryLimitFor(availableMemory())
}

// memoryLimitFor - the arithmetic and the environment handling, separated from
// reading the machine so it can be tested without one.
func memoryLimitFor(available int64, source string) (int64, string) {
	if available <= 0 {
		return 0, ""
	}
	percent := int64(defaultMemoryLimitPercent)
	if env, ok := os.LookupEnv("GRAPE_MEMORY_LIMIT"); ok {
		env = strings.TrimSpace(env)
		if strings.HasSuffix(env, "%") {
			if n, err := strconv.ParseInt(strings.TrimSuffix(env, "%"), 10, 64); err == nil && n > 0 && n <= 100 {
				percent = n
			} else {
				logger.Warnf("[gc] Ignoring GRAPE_MEMORY_LIMIT=%q: expected 1-100 followed by %%", env)
			}
		} else if n, err := strconv.ParseInt(env, 10, 64); err == nil && n > 0 {
			return n, fmt.Sprintf("GRAPE_MEMORY_LIMIT=%s", env)
		} else {
			logger.Warnf("[gc] Ignoring GRAPE_MEMORY_LIMIT=%q: expected a byte count or a percentage", env)
		}
	}
	// available*percent/100 rather than available/100*percent: the second
	// truncates twice, and explaining a limit that is 20 bytes off the stated
	// percentage in a log line is not worth the arithmetic. Overflow would need
	// more than a hundred petabytes of memory.
	return available * percent / 100, fmt.Sprintf("%d%% of %d MiB %s", percent, available>>20, source)
}

// availableMemory - how much memory this process may actually use.
//
// The cgroup limit is checked first and wins when it is lower, because the node
// is deployed in containers: sizing against the host's total memory inside a
// container with a 2GB limit produces a soft limit above the hard one, which is
// the same as having no limit.
func availableMemory() (int64, string) {
	total := int64(memory.TotalMemory())
	if cg, ok := cgroupMemoryLimit(cgroupMemoryPaths...); ok && (total <= 0 || cg < total) {
		return cg, "cgroup memory limit"
	}
	if total <= 0 {
		return 0, ""
	}
	return total, "system memory"
}

// cgroupMemoryLimit - the container's memory ceiling, from cgroup v2 or v1.
//
// Returns false rather than an error for every failure: not running under a
// cgroup, "max" meaning unlimited, and an unreadable file are all the same
// answer to the only question being asked.
func cgroupMemoryLimit(paths ...string) (int64, bool) {
	for _, path := range paths {
		fd, err := os.Open(path)
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(fd)
		var line string
		if scanner.Scan() {
			line = strings.TrimSpace(scanner.Text())
		}
		fd.Close()
		if line == "" || line == "max" {
			continue
		}
		n, err := strconv.ParseInt(line, 10, 64)
		if err != nil || n <= 0 {
			continue
		}
		// v1 reports "no limit" as a number near the top of int64. Anything
		// above a petabyte is that, not a real limit.
		if n > 1<<50 {
			continue
		}
		return n, true
	}
	return 0, false
}
