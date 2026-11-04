//go:build !windows

package localfabric

import "os"

func protectPrivateDirectory(path string) error { return os.Chmod(path, 0700) }
