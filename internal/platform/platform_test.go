package platform

import "testing"

// Within matches whole folder names, ignoring case and trailing slashes.
func TestWithin(t *testing.T) {
	for _, tc := range []struct {
		parent, child string
		want          bool
	}{
		{`D:\Games\Foo`, `D:\Games\FooBar\x.exe`, false},
		{`C:\`, `C:\x`, true},
		{`d:\games\foo\`, `D:\Games\Foo`, true},
		{`D:\Games\Foo`, `D:\Games\Foo\bin\a.exe`, true},
		{"", `D:\Games\Foo`, false},
		{`D:\Games\Foo`, "", false},
	} {
		if got := Within(tc.parent, tc.child); got != tc.want {
			t.Errorf("Within(%q, %q) = %v, want %v", tc.parent, tc.child, got, tc.want)
		}
	}
}

// Only the link types Seaglass builds reach ShellExecute.
func TestSafeURI(t *testing.T) {
	for uri, want := range map[string]bool{
		"steam://run/1":                   true,
		"com.epicgames.launcher://apps/x": true,
		`SHELL:AppsFolder\x`:              true,
		"https://x":                       false,
		"file:///c:/x":                    false,
		"steam://x\r\ny":                  false,
		"steam://run/1\x00":               false,
	} {
		if got := safeURI(uri); got != want {
			t.Errorf("safeURI(%q) = %v, want %v", uri, got, want)
		}
	}
}

// Crash codes are NTSTATUS errors and breakpoints, not exit(-1) or Ctrl+C.
func TestCrashed(t *testing.T) {
	for code, want := range map[uint32]bool{
		0xC0000005: true, 0x80000003: true, 0xC0000409: true,
		0: false, 1: false, 0xFFFFFFFF: false, 0xC000013A: false,
	} {
		if got := Crashed(code); got != want {
			t.Errorf("Crashed(%#x) = %v, want %v", code, got, want)
		}
	}
}
