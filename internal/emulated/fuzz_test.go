package emulated

import (
	"testing"
)

// The emulators' files and the games' own are untrusted: none of their
// parsers may panic or loop on any input.

func FuzzSFO(f *testing.F) {
	f.Add(sfo(map[string]string{"CATEGORY": "DG", "TITLE": "A", "TITLE_ID": "BLES00001"}))
	f.Add([]byte("\x00PSF\x01\x01\x00\x00\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff"))
	f.Fuzz(func(t *testing.T, b []byte) { _, _ = parseSFO(b) })
}

func FuzzDisc(f *testing.F) {
	f.Add(gcDisc("GM4E01", "Mario Kart", false, 0)[:0x100])
	wbfs := make([]byte, 0x100)
	copy(wbfs, "WBFS")
	wbfs[8] = 9
	f.Add(wbfs)
	f.Add(append([]byte("RVZ\x01"), make([]byte, 0xFC)...))
	f.Fuzz(func(t *testing.T, b []byte) {
		_, _ = parseDisc(b, func(off int64) ([]byte, error) {
			if off < 0 || off+0x60 > int64(len(b)) {
				return make([]byte, 0x60), nil
			}
			return b[off : off+0x60], nil
		})
	})
}

func FuzzGamesYML(f *testing.F) {
	f.Add([]byte("BLES01689: \"R:/Games/PS3/A/\"\nX: 'it''s'\n"))
	f.Fuzz(func(t *testing.T, b []byte) {
		_ = parseGamesYML(b)
		_ = vfsHDD(b, `C:\Emu`)
	})
}

func FuzzTitleDB(f *testing.F) {
	f.Add([]byte("TITLES = x\nRMCE01 = Mario Kart Wii\n"))
	f.Fuzz(func(t *testing.T, b []byte) { _ = parseTitleDB(b) })
}

func FuzzINI(f *testing.F) {
	f.Add([]byte("\xef\xbb\xbf[General]\nISOPaths = 1\nISOPath0 = R:/Games\n"))
	f.Fuzz(func(t *testing.T, b []byte) { _ = parseINI(b) })
}

func FuzzCemuXML(f *testing.F) {
	f.Add([]byte(`<content><GamePaths><Entry>Z:\x</Entry></GamePaths><mlc_path>C:\m</mlc_path></content>`))
	f.Add([]byte(`<title_list><title titleId="0005000010101c00"><name>A</name><path>B</path></title></title_list>`))
	f.Add([]byte(`<menu><title_id>0005000010144F00</title_id><longname_en>Zelda</longname_en></menu>`))
	f.Fuzz(func(t *testing.T, b []byte) {
		_ = parseCemuSettings(b)
		_ = parseCemuTitles(b)
		_ = parseWiiUMeta(b)
	})
}
