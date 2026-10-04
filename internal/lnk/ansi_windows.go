package lnk

import (
	"strings"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const cpACP = 0 // the system's ANSI code page

// ansi decodes bytes in the system's ANSI code page (CP_ACP).
func ansi(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	ascii := true
	for _, c := range b {
		if c >= 0x80 {
			ascii = false
			break
		}
	}
	if ascii {
		return string(b)
	}
	n, err := windows.MultiByteToWideChar(cpACP, 0, &b[0], int32(len(b)), nil, 0)
	if err != nil || n <= 0 {
		return string(b)
	}
	u := make([]uint16, n)
	if _, err := windows.MultiByteToWideChar(cpACP, 0, &b[0], int32(len(b)), &u[0], n); err != nil {
		return string(b)
	}
	return windows.UTF16ToString(u)
}

// expand fills in %VARIABLES% the way Windows does.
func expand(s string) string {
	if !strings.Contains(s, "%") {
		return s
	}
	if e, err := registry.ExpandString(s); err == nil {
		return e
	}
	return s
}
