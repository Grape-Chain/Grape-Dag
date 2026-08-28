package run

import (
	"os"
	"path/filepath"
	"runtime/debug"
	"testing"

	golog "github.com/ipfs/go-log/v2"
)

// The package logger is created by ProcessLogInit in the running node, which
// happens before the tuning step. These tests call the tuning code directly, so
// they have to stand it up themselves.
func init() {
	logger = golog.Logger("run-test")
}

func TestTheMemoryLimitLeavesHeadroomBelowWhatTheNodeMayUse(t *testing.T) {
	const available = 8 << 30
	got, why := memoryLimitFor(available, "system memory")
	if got <= 0 {
		t.Fatal("expected a limit to be chosen")
	}
	if got >= available {
		t.Fatalf("a soft limit of %d against %d available leaves no headroom", got, available)
	}
	want := int64(available) * defaultMemoryLimitPercent / 100
	if got != want {
		t.Fatalf("expected %d%% of the available memory (%d), got %d", defaultMemoryLimitPercent, want, got)
	}
	if why == "" {
		t.Fatal("the chosen limit should say where it came from; that line is how it gets diagnosed")
	}
}

func TestNoMemoryLimitIsSetWhenTheMachineCannotBeMeasured(t *testing.T) {
	// Better to run unlimited, as the node always has, than to invent a ceiling
	// from a number we do not have.
	if got, _ := memoryLimitFor(0, ""); got != 0 {
		t.Fatalf("expected no limit when available memory is unknown, got %d", got)
	}
	if got, _ := memoryLimitFor(-1, ""); got != 0 {
		t.Fatalf("expected no limit for a negative reading, got %d", got)
	}
}

func TestTheMemoryLimitAcceptsAByteCountOrAPercentage(t *testing.T) {
	const available = 10 << 30

	t.Setenv("GRAPE_MEMORY_LIMIT", "12345678")
	if got, _ := memoryLimitFor(available, "system memory"); got != 12345678 {
		t.Fatalf("expected an explicit byte count to be honoured, got %d", got)
	}

	t.Setenv("GRAPE_MEMORY_LIMIT", "50%")
	if got, want := mustLimit(t, available), int64(available)/2; got != want {
		t.Fatalf("expected 50%% of %d = %d, got %d", available, want, got)
	}

	for _, bad := range []string{"0", "-5", "101%", "0%", "half", "1GB", "%"} {
		t.Setenv("GRAPE_MEMORY_LIMIT", bad)
		got := mustLimit(t, available)
		want := int64(available) * defaultMemoryLimitPercent / 100
		if got != want {
			t.Fatalf("GRAPE_MEMORY_LIMIT=%q should have fallen back to the default, got %d", bad, got)
		}
	}
}

func mustLimit(t *testing.T, available int64) int64 {
	t.Helper()
	got, _ := memoryLimitFor(available, "system memory")
	if got <= 0 {
		t.Fatal("expected a limit to be chosen")
	}
	return got
}

// TestTheContainerLimitWinsOverTheHostsTotalMemory - sizing against the host
// inside a container produces a soft limit above the container's hard one, which
// is the same as having no limit at all.
func TestTheContainerLimitWinsOverTheHostsTotalMemory(t *testing.T) {
	dir := t.TempDir()
	v2 := filepath.Join(dir, "memory.max")
	if err := os.WriteFile(v2, []byte("2147483648\n"), 0o600); err != nil {
		t.Fatalf("writing the fake cgroup file: %s", err.Error())
	}
	got, ok := cgroupMemoryLimit(filepath.Join(dir, "absent"), v2)
	if !ok {
		t.Fatal("expected the cgroup limit to be read")
	}
	if got != 2147483648 {
		t.Fatalf("expected 2147483648, got %d", got)
	}
}

func TestAnUnlimitedCgroupIsNotMistakenForALimit(t *testing.T) {
	dir := t.TempDir()
	cases := map[string]string{
		// v2 says "max" when there is no limit.
		"v2-max": "max\n",
		// v1 reports no limit as a number near the top of int64.
		"v1-huge": "9223372036854771712\n",
		"empty":   "",
		"rubbish": "not a number\n",
		"zero":    "0\n",
	}
	for name, content := range cases {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("writing %s: %s", name, err.Error())
		}
		if got, ok := cgroupMemoryLimit(path); ok {
			t.Fatalf("%s should not read as a limit, got %d", name, got)
		}
	}
}

func TestNoCgroupFileMeansNoCgroupLimit(t *testing.T) {
	if _, ok := cgroupMemoryLimit(filepath.Join(t.TempDir(), "absent")); ok {
		t.Fatal("a missing cgroup file must not read as a limit")
	}
	if _, ok := cgroupMemoryLimit(); ok {
		t.Fatal("no paths at all must not read as a limit")
	}
}

// TestTuningSetsBothKnobsAndLeavesAnOperatorsOwnSettingsAlone - the runtime has
// already read GOGC and GOMEMLIMIT by the time this runs, so overriding an
// operator's explicit choice would be silently undoing an instruction.
func TestTuningSetsBothKnobsAndLeavesAnOperatorsOwnSettingsAlone(t *testing.T) {
	// Restore whatever the test binary was running with, so this test does not
	// change the collector's behaviour for everything after it.
	originalGC := debug.SetGCPercent(defaultGOGC)
	originalLimit := debug.SetMemoryLimit(-1)
	t.Cleanup(func() {
		debug.SetGCPercent(originalGC)
		debug.SetMemoryLimit(originalLimit)
	})

	t.Setenv("GRAPE_GOGC", "150")
	tuneGC()
	if got := debug.SetGCPercent(150); got != 150 {
		t.Fatalf("expected GOGC to have been set to 150, got %d", got)
	}
	if got := debug.SetMemoryLimit(-1); got <= 0 || got == originalLimit && originalLimit == 1<<63-1 {
		t.Fatalf("expected a soft memory limit to have been set, got %d", got)
	}

	// With the runtime's own variables set, nothing should be touched.
	debug.SetGCPercent(originalGC)
	debug.SetMemoryLimit(originalLimit)
	t.Setenv("GOGC", "77")
	t.Setenv("GOMEMLIMIT", "1GiB")
	tuneGC()
	if got := debug.SetGCPercent(originalGC); got != originalGC {
		t.Fatalf("GOGC was set to %d despite the operator having set GOGC in the environment", got)
	}
	if got := debug.SetMemoryLimit(originalLimit); got != originalLimit {
		t.Fatalf("the memory limit was changed to %d despite the operator having set GOMEMLIMIT", got)
	}
}

func TestTheDefaultsAreTheOnesTheCommentaryDescribes(t *testing.T) {
	if defaultGOGC <= 100 {
		t.Fatalf("GOGC of %d is not a relaxation of the language default of 100", defaultGOGC)
	}
	if defaultMemoryLimitPercent <= 0 || defaultMemoryLimitPercent >= 100 {
		t.Fatalf("a soft limit of %d%% of memory leaves no headroom", defaultMemoryLimitPercent)
	}
}
