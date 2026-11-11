package maintenance

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDailyBackupRunsOnceAndFencesInterruptedSlot(t *testing.T) {
	directory := t.TempDir()
	now := time.Date(2026, 11, 11, 3, 20, 0, 0, time.UTC)
	calls := 0
	callback := func(path string) (string, error) {
		calls++
		payload := []byte("fixture artifact")
		return HashBytes(payload), CreatePrivate(path, payload)
	}
	if ran, err := RunDailyBackup(directory, "fixture-a", 4, 2, now, callback); ran || err != nil {
		t.Fatal("outside schedule", ran, err)
	}
	if ran, err := RunDailyBackup(directory, "fixture-a", 3, 2, now, callback); !ran || err != nil {
		t.Fatal("expected a capture", ran, err)
	}
	if ran, err := RunDailyBackup(directory, "fixture-a", 3, 2, now, callback); ran || err != nil || calls != 1 {
		t.Fatal("completed slot repeated", ran, err, calls)
	}
	if ran, err := RunDailyBackup(directory, "fixture-b", 3, 2, now, func(string) (string, error) { return "", errors.New("interrupted") }); !ran || err == nil {
		t.Fatal("interruption lost", ran, err)
	}
	if ran, err := RunDailyBackup(directory, "fixture-b", 3, 2, now, callback); ran || err == nil || calls != 1 {
		t.Fatal("interrupted slot was repeated", ran, err, calls)
	}
}

func TestDailyRetentionKeepsVerifiedArtifactsAndBlocksUnfinishedEarlierSlot(t *testing.T) {
	directory := t.TempDir()
	day := time.Date(2026, 11, 11, 3, 0, 0, 0, time.UTC)
	capture := func(path string) (string, error) {
		data := []byte("encrypted fixture")
		return HashBytes(data), CreatePrivate(path, data)
	}
	for i := 0; i < 3; i++ {
		if ran, err := RunDailyBackup(directory, "network", 3, 2, day.AddDate(0, 0, i), capture); !ran || err != nil {
			t.Fatal(ran, err)
		}
	}
	artifacts, _ := filepath.Glob(filepath.Join(directory, "*.encrypted.json"))
	if len(artifacts) != 2 {
		t.Fatalf("expected two artifacts, got %d", len(artifacts))
	}
	if err := os.WriteFile(artifacts[0], []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := RunDailyBackup(directory, "network", 3, 2, day.AddDate(0, 0, 2), capture); err == nil {
		t.Fatal("tampered artifact accepted")
	}
	if _, err := os.Stat(artifacts[0]); err != nil {
		t.Fatal("tampered artifact was removed")
	}
	fail := func(string) (string, error) { return "", errors.New("capture interrupted") }
	_, _ = RunDailyBackup(directory, "interrupted", 3, 2, day, fail)
	if ran, err := RunDailyBackup(directory, "interrupted", 3, 2, day.AddDate(0, 0, 1), capture); ran || err == nil {
		t.Fatal("earlier interrupted slot bypassed")
	}
}

func TestDailyScheduleRejectsConcurrentCaptureAcrossDates(t *testing.T) {
	directory := t.TempDir()
	day := time.Date(2026, 11, 11, 3, 0, 0, 0, time.UTC)
	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		_, err := RunDailyBackup(directory, "network", 3, 2, day, func(path string) (string, error) {
			close(entered)
			<-release
			data := []byte("fixture artifact")
			return HashBytes(data), CreatePrivate(path, data)
		})
		done <- err
	}()
	select {
	case <-entered:
	case err := <-done:
		t.Fatalf("first capture failed to start: %v", err)
	}
	ran, err := RunDailyBackup(directory, "network", 3, 2, day.AddDate(0, 0, 1), func(string) (string, error) {
		t.Error("concurrent capture reached the executor")
		return "", errors.New("unexpected callback")
	})
	close(release)
	if firstErr := <-done; ran || err == nil || firstErr != nil {
		t.Fatalf("exclusive backup lock failed: ran=%v second=%v first=%v", ran, err, firstErr)
	}
}
