package achievements

import "testing"

// Unlock files are untrusted: no input may panic a parser.

func fuzzParser(f *testing.F, p parser, seeds ...string) {
	for _, s := range seeds {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, b []byte) { _, _ = p(b) })
}

func FuzzGoldbergJSON(f *testing.F) {
	fuzzParser(f, parseGoldbergJSON, `{"A":{"earned":true,"earned_time":1}}`, `{"A":{"earned":"1"}}`)
}

func FuzzINIUnlocks(f *testing.F) {
	fuzzParser(f, parseINIUnlocks, "[A]\nAchieved=1\nUnlockTime=5\n", "\xff\xfe[\x00A\x00]\x00")
}

func FuzzRLD(f *testing.F) {
	fuzzParser(f, parseRLD, "[Steam]\n[A]\nState=01000000\nTime=00F15365\n")
}

func FuzzSkidrow(f *testing.F) { fuzzParser(f, parseSkidrow, "[X]\nA=1@0@0@5\n") }

func FuzzTenoke(f *testing.F) {
	fuzzParser(f, parseTenoke, "[ACHIEVEMENTS]\nA = {unlocked = true, time = 5}\n")
}

func FuzzRazor(f *testing.F) { fuzzParser(f, parseRazor, "A 1 5\n") }

func FuzzSSE(f *testing.F) {
	fuzzParser(f, parseSSE, sseFile([3]uint32{1, 2, 1}), "\xff\xff\xff\x7f")
}

func FuzzNemirtinga(f *testing.F) {
	fuzzParser(f, parseNemirtinga, `[{"AchievementId":"A","Progress":1}]`, `{"x":[{"id":"A","unlocked":true}]}`)
}

func FuzzGoldbergSchema(f *testing.F) {
	f.Add([]byte(`[{"name":"A","displayName":{"english":"a"},"hidden":"1"}]`))
	f.Fuzz(func(t *testing.T, b []byte) { _, _ = parseGoldbergSchema(b, "english") })
}
