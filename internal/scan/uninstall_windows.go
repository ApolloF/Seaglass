package scan

import "golang.org/x/sys/windows/registry"

func uninstallEntries() []uninstallEntry {
	var out []uninstallEntry
	for _, src := range []struct {
		root registry.Key
		path string
	}{
		{registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`},
		{registry.LOCAL_MACHINE, `SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall`},
		{registry.CURRENT_USER, `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`},
	} {
		registryEach(src.root, src.path, func(k registry.Key, key string) {
			if regInt(k, "SystemComponent") == 1 || regString(k, "ParentKeyName") != "" {
				return
			}
			e := uninstallEntry{
				Key: key, Name: regString(k, "DisplayName"), Publisher: regString(k, "Publisher"),
				Dir: regString(k, "InstallLocation"), Icon: regString(k, "DisplayIcon"), SizeKB: regInt(k, "EstimatedSize"),
			}
			if e.Name != "" {
				out = append(out, e)
			}
		})
	}
	return out
}
