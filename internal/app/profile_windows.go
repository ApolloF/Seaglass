package app

import "golang.org/x/sys/windows/registry"

// machineID is Windows' machine id (MachineGuid), "" when unreadable.
func machineID() string {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Cryptography`, registry.QUERY_VALUE|registry.WOW64_64KEY)
	if err != nil {
		return ""
	}
	defer k.Close()
	raw, _, _ := k.GetStringValue("MachineGuid")
	return raw
}
