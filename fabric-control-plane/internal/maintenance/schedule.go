package maintenance

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"time"
)

// RunDailyBackup claims the current UTC slot before invoking cold maintenance.
// A failed or interrupted attempt leaves its claim in place for operator review.
// The caller supplies an operator-owned private directory and an exact target key.
func RunDailyBackup(directory, target string, hour int, now time.Time, captureAndResume func(string) error) (bool, error) {
	if target == "" || hour < 0 || hour > 23 || !filepath.IsAbs(directory) || captureAndResume == nil {
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
	digest := sha256.Sum256([]byte(target))
	prefix := filepath.Join(directory, now.Format("2006-01-02")+"-"+hex.EncodeToString(digest[:]))
	if _, err := os.Lstat(prefix + ".complete"); err == nil {
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, errors.New("cannot inspect daily backup completion")
	}
	if err := CreatePrivate(prefix+".started", []byte(now.Format(time.RFC3339))); err != nil {
		return false, errors.New("daily backup slot already claimed or inaccessible; inspect the checkpoint")
	}
	if err := captureAndResume(prefix + ".encrypted.json"); err != nil {
		return true, err
	}
	if err := CreatePrivate(prefix+".complete", []byte(time.Now().UTC().Format(time.RFC3339))); err != nil {
		return true, errors.New("backup completed but slot acknowledgement failed; inspect the checkpoint")
	}
	return true, nil
}
