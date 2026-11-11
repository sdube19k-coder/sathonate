package maintenance

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type dailyCompletion struct {
	ArtifactSHA256 string `json:"artifactSHA256"`
}

// RunDailyBackup serializes runs for an exact network and target, including across
// UTC dates. Incomplete markers require operator review before another capture.
// Retention removes only digest-verified artifacts with completed receipts.
func RunDailyBackup(directory, target string, hour, keep int, now time.Time, captureAndResume func(string) (string, error)) (bool, error) {
	if target == "" || hour < 0 || hour > 23 || keep < 1 || keep > 365 || !filepath.IsAbs(directory) || captureAndResume == nil {
		return false, errors.New("invalid daily backup configuration")
	}
	now = now.UTC()
	if now.Hour() != hour {
		return false, nil
	}
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return false, errors.New("daily backup requires an existing private directory")
	}
	id := HashBytes([]byte(target))
	lock := filepath.Join(directory, id+".lock")
	if err := os.Mkdir(lock, 0700); err != nil {
		return false, errors.New("backup lock is held or inaccessible; inspect the owner and checkpoint")
	}
	defer os.Remove(lock)
	date := now.Format("2006-01-02")
	prefix := filepath.Join(directory, date+"-"+id)
	entries, err := os.ReadDir(directory)
	if err != nil {
		return false, errors.New("backup directory inventory unavailable")
	}
	completed := []string{}
	for _, entry := range entries {
		day, ok := strings.CutSuffix(entry.Name(), "-"+id+".started")
		if !ok {
			continue
		}
		if _, err := time.Parse("2006-01-02", day); err != nil || day > date {
			return false, errors.New("invalid backup slot date; inspect the checkpoint")
		}
		slot := filepath.Join(directory, day+"-"+id)
		if _, err := readDailyCompletion(slot); err != nil {
			return false, errors.New("incomplete or unreadable backup slot; inspect the checkpoint")
		}
		completed = append(completed, slot)
	}
	ran := false
	if !containsSlot(completed, prefix) {
		if err := CreatePrivate(prefix+".started", []byte(now.Format(time.RFC3339))); err != nil {
			return false, errors.New("daily backup slot could not be claimed")
		}
		artifact := prefix + ".encrypted.json"
		digest, err := captureAndResume(artifact)
		if err != nil {
			return true, err
		}
		if !digestPattern.MatchString(digest) || !verifyDailyArtifact(artifact, digest) {
			return true, errors.New("captured artifact does not match its receipt")
		}
		receipt, _ := json.Marshal(dailyCompletion{ArtifactSHA256: digest})
		if err := CreatePrivate(prefix+".complete", receipt); err != nil {
			return true, errors.New("backup completed but acknowledgement failed; inspect the checkpoint")
		}
		completed = append(completed, prefix)
		ran = true
	}
	sort.Sort(sort.Reverse(sort.StringSlice(completed)))
	var prune []string
	for i, slot := range completed {
		artifact := slot + ".encrypted.json"
		if _, err := os.Lstat(artifact); os.IsNotExist(err) && i >= keep {
			continue
		}
		receipt, err := readDailyCompletion(slot)
		if err != nil || !verifyDailyArtifact(artifact, receipt.ArtifactSHA256) {
			return ran, errors.New("backup retention found a missing or changed artifact; pruning refused")
		}
		if i >= keep {
			prune = append(prune, artifact)
		}
	}
	for _, artifact := range prune {
		if err := os.Remove(artifact); err != nil {
			return ran, errors.New("completed backup could not be pruned")
		}
	}
	return ran, nil
}

func containsSlot(slots []string, target string) bool {
	for _, slot := range slots {
		if slot == target {
			return true
		}
	}
	return false
}

func readDailyCompletion(slot string) (dailyCompletion, error) {
	var receipt dailyCompletion
	data, err := readRegularBounded(slot+".complete", 1024)
	if err != nil || json.Unmarshal(data, &receipt) != nil || !digestPattern.MatchString(receipt.ArtifactSHA256) {
		return receipt, errors.New("invalid daily backup receipt")
	}
	return receipt, nil
}

func verifyDailyArtifact(path, digest string) bool {
	data, err := readRegularBounded(path, MaxBackupBytes*2)
	return err == nil && HashBytes(data) == digest
}

func readRegularBounded(path string, max int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > max {
		return nil, errors.New("invalid private backup file")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return nil, errors.New("backup file changed")
	}
	data, err := io.ReadAll(io.LimitReader(f, max+1))
	if err != nil || int64(len(data)) > max {
		return nil, errors.New("backup file exceeds limit")
	}
	return data, nil
}
