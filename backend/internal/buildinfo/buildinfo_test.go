package buildinfo

import "testing"

func TestCurrentReflectsInjectedValues(t *testing.T) {
	previousVersion, previousCommit, previousDate := Version, Commit, BuildDate
	t.Cleanup(func() { Version, Commit, BuildDate = previousVersion, previousCommit, previousDate })
	Version, Commit, BuildDate = "1.2.3", "abc123", "2026-09-06T12:00:00Z"

	got := Current()
	if got.Version != Version || got.Commit != Commit || got.BuildDate != BuildDate {
		t.Fatalf("Current() = %#v", got)
	}
}
