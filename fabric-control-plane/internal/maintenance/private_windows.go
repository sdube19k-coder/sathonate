//go:build windows

package maintenance

import (
	"golang.org/x/sys/windows"
	"os/user"
)

func protectPrivateFile(path string) error {
	current, e := user.Current()
	if e != nil {
		return e
	}
	descriptor, e := windows.SecurityDescriptorFromString("D:P(A;;FA;;;" + current.Uid + ")")
	if e != nil {
		return e
	}
	acl, _, e := descriptor.DACL()
	if e != nil {
		return e
	}
	return windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil)
}
