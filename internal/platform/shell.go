package platform

import (
	"errors"
	"path/filepath"
	"strings"
)

// OpenURI hands a URI (steam://…, com.epicgames.launcher://…, shell:…) to
// Windows, the way Explorer would open it.
func OpenURI(uri string) error {
	if !safeURI(uri) {
		return errors.New("refusing to open this link")
	}
	return shellExecute("open", uri, "", "")
}

// allowedSchemes are the link types Seaglass itself ever builds.
var allowedSchemes = []string{"steam://", "com.epicgames.launcher://", "uplay://", "goggalaxy://", "origin2://", "battlenet://", "shell:appsfolder\\"}

func safeURI(uri string) bool {
	l := strings.ToLower(uri)
	for _, s := range allowedSchemes {
		if strings.HasPrefix(l, s) {
			return !strings.ContainsAny(uri, "\r\n\x00")
		}
	}
	return false
}

// OpenWebPage opens an https page in the default browser. Only pages
// Seaglass itself links to are passed here.
func OpenWebPage(url string) error {
	if !strings.HasPrefix(url, "https://") || strings.ContainsAny(url, " \"<>\r\n\x00") {
		return errors.New("refusing to open this link")
	}
	return shellExecute("open", url, "", "")
}

// OpenFile opens a text file Seaglass wrote in the user's editor.
func OpenFile(p string) error {
	if !filepath.IsAbs(p) || !IsFile(p) || !strings.EqualFold(filepath.Ext(p), ".txt") {
		return errors.New("refusing to open this file")
	}
	return shellExecute("open", p, "", "")
}
