package maintenance

import (
	"errors"
	"testing"
	"time"
)

func TestDailyBackupRunsOnceAndFencesInterruptedSlot(t *testing.T) {
	directory := t.TempDir()
	now := time.Date(2026, 11, 11, 3, 20, 0, 0, time.UTC)
	calls := 0
	callback := func(path string) error {
		calls++
		return CreatePrivate(path, []byte("fixture artifact"))
	}
	if ran, err := RunDailyBackup(directory, "fixture-a", 4, now, callback); ran || err != nil {
		t.Fatal("outside schedule", ran, err)
	}
	if ran, err := RunDailyBackup(directory, "fixture-a", 3, now, callback); !ran || err != nil {
		t.Fatal("expected a capture", ran, err)
	}
	if ran, err := RunDailyBackup(directory, "fixture-a", 3, now, callback); ran || err != nil || calls != 1 {
		t.Fatal("completed slot repeated", ran, err, calls)
	}
	if ran, err := RunDailyBackup(directory, "fixture-b", 3, now, func(string) error { return errors.New("interrupted") }); !ran || err == nil {
		t.Fatal("interruption lost", ran, err)
	}
	if ran, err := RunDailyBackup(directory, "fixture-b", 3, now, callback); ran || err == nil || calls != 1 {
		t.Fatal("interrupted slot was repeated", ran, err, calls)
	}
}
