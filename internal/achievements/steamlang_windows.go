package achievements

import (
	"strings"

	"golang.org/x/sys/windows/registry"
)

// SteamLanguage is Steam's interface language ("english", "german", …),
// the language achievement names are shown in.
func SteamLanguage() string {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Valve\Steam`, registry.QUERY_VALUE)
	if err != nil {
		return "english"
	}
	defer k.Close()
	if s, _, err := k.GetStringValue("Language"); err == nil && validLang(s) {
		return strings.ToLower(s)
	}
	return "english"
}
