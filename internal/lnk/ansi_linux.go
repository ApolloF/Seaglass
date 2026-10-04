package lnk

// ansi decodes bytes in the Windows ANSI code page; Linux has none, so
// they are taken as Latin-1 (close to Western Windows' code page 1252).
func ansi(b []byte) string {
	r := make([]rune, len(b))
	for i, c := range b {
		r[i] = rune(c)
	}
	return string(r)
}

// expand leaves %VARIABLES% alone: they name Windows folders.
func expand(s string) string { return s }
