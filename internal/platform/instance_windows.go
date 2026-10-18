package platform

import (
	"time"

	"golang.org/x/sys/windows"
)

// instanceMutex is the mutex Wails' single-instance lock creates for
// uniqueID (pkg/application/single_instance_windows.go, beta.26).
func instanceMutex(uniqueID string) string { return "wails-app-" + uniqueID + "-sim" }

// InstanceRunning reports whether a Seaglass with this id already runs
// in this Windows session.
func InstanceRunning(uniqueID string) bool {
	name, err := windows.UTF16PtrFromString(instanceMutex(uniqueID))
	if err != nil {
		return false
	}
	h, err := windows.OpenMutex(windows.SYNCHRONIZE, false, name)
	if err != nil {
		return false
	}
	windows.CloseHandle(h)
	return true
}

// WaitInstanceGone waits until the running instance has exited, at most d.
func WaitInstanceGone(uniqueID string, d time.Duration) bool {
	end := time.Now().Add(d)
	for InstanceRunning(uniqueID) {
		if time.Now().After(end) {
			return false
		}
		time.Sleep(150 * time.Millisecond)
	}
	return true
}
