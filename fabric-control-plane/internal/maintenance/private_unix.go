//go:build !windows

package maintenance

import "os"

func protectPrivateFile(path string) error { return os.Chmod(path, 0600) }
