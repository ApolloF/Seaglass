package achievements

import (
	"encoding/binary"
	"hash/crc32"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// mk writes files under root; a name ending in / is a folder.
func mk(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if strings.HasSuffix(rel, "/") {
			if err := os.MkdirAll(p, 0o755); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func testEnv(root string) Env {
	return Env{
		Roaming: filepath.Join(root, "Roaming"), Local: filepath.Join(root, "Local"), Public: filepath.Join(root, "Public"),
		ProgramData: filepath.Join(root, "ProgramData"), Documents: filepath.Join(root, "Documents"),
	}
}

func sseFile(records ...[3]uint32) string { // crc, time, state
	b := make([]byte, 4+24*len(records))
	binary.LittleEndian.PutUint32(b, uint32(len(records)))
	for i, r := range records {
		o := 4 + 24*i
		binary.LittleEndian.PutUint32(b[o:], r[0])
		binary.LittleEndian.PutUint32(b[o+8:], r[1])
		binary.LittleEndian.PutUint32(b[o+20:], r[2])
	}
	return string(b)
}

func TestReadEmu(t *testing.T) {
	const at = 1_700_000_000
	cases := []struct {
		name   string
		emuDir string
		files  map[string]string // relative to the test root; game/ is the game folder
		source string
		want   map[string]Unlock
	}{
		{"goldberg", ".", map[string]string{
			"Roaming/Goldberg SteamEmu Saves/620/achievements.json": `{"ACH_WIN":{"earned":true,"earned_time":1700000000},"ACH_LOSE":{"earned":false,"earned_time":0,"progress":3,"max_progress":10}}`,
		}, "Goldberg", map[string]Unlock{"ACH_WIN": {Achieved: true, At: at}, "ACH_LOSE": {Progress: 3, Max: 10}}},
		{"gbe_fork", ".", map[string]string{
			"Roaming/GSE Saves/620/achievements.json": `{"ACH_WIN":{"earned":true,"earned_time":1700000000}}`,
		}, "Goldberg", map[string]Unlock{"ACH_WIN": {Achieved: true, At: at}}},
		{"goldberg local_save.txt", "bin", map[string]string{
			"game/bin/local_save.txt":                 "saves",
			"game/bin/saves/620/achievements.json":    `{"ACH_WIN":{"earned":true,"earned_time":1700000000}}`,
			"Roaming/GSE Saves/999/achievements.json": `{}`,
		}, "Goldberg", map[string]Unlock{"ACH_WIN": {Achieved: true, At: at}}},
		{"gbe_fork local_save_path", ".", map[string]string{
			"game/steam_settings/configs.user.ini": "[user::saves]\nlocal_save_path=./GSE\n",
			"game/GSE/620/achievements.json":       `{"ACH_WIN":{"earned":true,"earned_time":1700000000}}`,
		}, "Goldberg", map[string]Unlock{"ACH_WIN": {Achieved: true, At: at}}},
		{"empress public", ".", map[string]string{
			"Public/Documents/EMPRESS/620/remote/620/achievements.json": `{"ACH_WIN":{"earned":true,"earned_time":1700000000}}`,
		}, "EMPRESS", map[string]Unlock{"ACH_WIN": {Achieved: true, At: at}}},
		{"codex", ".", map[string]string{
			"Public/Documents/Steam/CODEX/620/achievements.ini": "[SteamAchievements]\nCount=2\n00000=ach_win\n[ach_win]\nAchieved=1\nCurProgress=0\nMaxProgress=0\nUnlockTime=1700000000\n[ach_lose]\nAchieved=0\nCurProgress=2\nMaxProgress=5\nUnlockTime=0\n",
		}, "CODEX", map[string]Unlock{"ach_win": {Achieved: true, At: at}, "ach_lose": {Progress: 2, Max: 5}}},
		{"rune", ".", map[string]string{
			"Public/Documents/Steam/RUNE/620/achievements.ini": "[ACH_WIN]\nAchieved=1\nUnlockTime=1700000000\n",
		}, "RUNE", map[string]Unlock{"ACH_WIN": {Achieved: true, At: at}}},
		{"onlinefix", ".", map[string]string{
			"Public/Documents/OnlineFix/620/Stats/Achievements.ini": "[ACH_WIN]\nachieved=true\ntimestamp=1700000000\n",
		}, "OnlineFix", map[string]Unlock{"ACH_WIN": {Achieved: true, At: at}}},
		{"creamapi seven digits", ".", map[string]string{
			"Roaming/CreamAPI/620/stats/CreamAPI.Achievements.cfg": "[ACH_WIN]\nachieved=true\nunlocktime=1700000\n",
		}, "CreamAPI", map[string]Unlock{"ACH_WIN": {Achieved: true, At: at}}},
		{"reloaded", ".", map[string]string{
			"ProgramData/Steam/RLD!/620/stats/achievements.ini": "[Steam]\nAppID=620\n[ACH_WIN]\nState=01000000\nTime=00F15365\nCurProgress=00000000\nMaxProgress=00000000\n",
		}, "Reloaded", nil},
		{"skidrow", ".", map[string]string{
			"Documents/SKIDROW/620/SteamEmu/UserStats/achiev.ini": "[ACHIEVE_DATA]\nACH_WIN=1@0@0@1700000000\nACH_LOSE=0@0@0@0\n",
		}, "SKIDROW", map[string]Unlock{"ach_win": {Achieved: true, At: at}, "ach_lose": {}}},
		{"tenoke", "game", map[string]string{
			"game/game/SteamData/user_stats.ini": "[STATS]\n[ACHIEVEMENTS]\nACH_WIN = {unlocked = true, time = 1700000000}\n\"ACH_LOSE\" = {unlocked = false, time = 0}\n",
		}, "TENOKE", map[string]Unlock{"ach_win": {Achieved: true, At: at}, "ach_lose": {}}},
		{"3dm", ".", map[string]string{
			"game/3DMGAME/Player/stats/achievements.ini": "[ACH_WIN]\nAchieved=1\nUnlockTime=1700000000\n",
		}, "3DM", map[string]Unlock{"ACH_WIN": {Achieved: true, At: at}}},
		{"ali213", ".", map[string]string{
			"game/Profile/Player/Stats/Achievements.Bin": "[ACH_WIN]\nHaveAchieved=1\nHaveAchievedTime=1700000000\n",
		}, "ALI213", map[string]Unlock{"ACH_WIN": {Achieved: true, At: at}}},
		{"razor1911", ".", map[string]string{
			"Roaming/.1911/620/achievement": "ACH_WIN 1 1700000000\nACH_LOSE 0 0\n",
		}, "Razor1911", map[string]Unlock{"ACH_WIN": {Achieved: true, At: at}, "ACH_LOSE": {}}},
		{"smartsteamemu", ".", map[string]string{
			"Roaming/SmartSteamEmu/620/stats.bin": sseFile([3]uint32{crc32.ChecksumIEEE([]byte("ACH_WIN")), at, 1}, [3]uint32{crc32.ChecksumIEEE([]byte("ACH_LOSE")), 0, 0}),
		}, "SmartSteamEmu", map[string]Unlock{SSEHash("ACH_WIN"): {Achieved: true, At: at}, SSEHash("ACH_LOSE"): {}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			mk(t, root, tc.files)
			g := EmuGame{Dir: filepath.Join(root, "game"), EmuDir: tc.emuDir, AppID: 620}
			res, _, ok := ReadEmu(g, testEnv(root))
			if !ok {
				t.Fatal("no unlock file found")
			}
			if res.Source != tc.source {
				t.Errorf("source %q, want %q", res.Source, tc.source)
			}
			if tc.want == nil { // Reloaded: checked below
				u := res.Unlocks["ACH_WIN"]
				if !u.Achieved || u.At != 0x6553F100 {
					t.Errorf("reloaded: %+v", res.Unlocks)
				}
				return
			}
			lower := map[string]Unlock{}
			for k, v := range res.Unlocks {
				lower[strings.ToLower(k)] = v
			}
			for id, w := range tc.want {
				if got := lower[strings.ToLower(id)]; got != w {
					t.Errorf("%s = %+v, want %+v", id, got, w)
				}
			}
			if len(res.Unlocks) != len(tc.want) {
				t.Errorf("got %d unlocks, want %d: %+v", len(res.Unlocks), len(tc.want), res.Unlocks)
			}
		})
	}
}

func TestReadEmuNewestWins(t *testing.T) {
	root := t.TempDir()
	mk(t, root, map[string]string{
		"Public/Documents/Steam/CODEX/620/achievements.ini":     "[OLD]\nAchieved=1\n",
		"Roaming/Goldberg SteamEmu Saves/620/achievements.json": `{"NEW":{"earned":true}}`,
	})
	old := time.Now().Add(-time.Hour)
	_ = os.Chtimes(filepath.Join(root, "Public/Documents/Steam/CODEX/620/achievements.ini"), old, old)
	res, files, ok := ReadEmu(EmuGame{Dir: filepath.Join(root, "game"), AppID: 620}, testEnv(root))
	if !ok || res.Source != "Goldberg" {
		t.Fatalf("got %+v", res)
	}
	if len(files) < 10 {
		t.Errorf("only %d candidate files", len(files))
	}
}

func TestLocalSaveStaysInGame(t *testing.T) {
	root := t.TempDir()
	mk(t, root, map[string]string{
		"game/steam_settings/configs.user.ini": "[user::saves]\nlocal_save_path=..\\..\\outside\n",
		"outside/620/achievements.json":        `{"X":{"earned":true}}`,
	})
	if _, _, ok := ReadEmu(EmuGame{Dir: filepath.Join(root, "game"), AppID: 620}, testEnv(root)); ok {
		t.Error("read unlocks outside the game folder")
	}
}

func TestNemirtinga(t *testing.T) {
	root := t.TempDir()
	mk(t, root, map[string]string{
		"game/nemirtingasepicemu.json":                                 `{"AppId":"sandbox1","EpicId":"x"}`,
		"Roaming/NemirtingasEpicEmu/Player/sandbox1/achievements.json": `[{"AchievementId":"WIN","Progress":1.0,"UnlockTime":12345},{"AchievementId":"LOSE","Progress":0.0}]`,
	})
	g := EmuGame{Dir: filepath.Join(root, "game")}
	g.EpicID = EpicEmuID(g)
	res, _, ok := ReadEmu(g, testEnv(root))
	if !ok || res.Source != "Epic emulator" || !res.Unlocks["WIN"].Achieved || res.Unlocks["WIN"].At != 0 || res.Unlocks["LOSE"].Achieved {
		t.Fatalf("got %+v ok=%v", res, ok)
	}
}

func TestGoldbergSchema(t *testing.T) {
	root := t.TempDir()
	mk(t, root, map[string]string{
		"game/steam_settings/achievements.json": `[
			{"name":"ACH_WIN","displayName":{"english":"Winner","german":"Gewinner"},"description":"Win","hidden":"0","icon":"win.jpg","icon_gray":"win_gray.jpg"},
			{"name":"ACH_SECRET","displayName":"Secret","description":"","hidden":1,"icon":"missing.jpg","icongray":"../../../x.jpg"}
		]`,
		"game/steam_settings/achievement_images/win.jpg":      "x",
		"game/steam_settings/achievement_images/win_gray.jpg": "x",
	})
	g := EmuGame{Dir: filepath.Join(root, "game")}
	defs, _, err := GoldbergSchema(g, "german")
	if err != nil || len(defs) != 2 {
		t.Fatalf("defs %+v err %v", defs, err)
	}
	if defs[0].Name != "Gewinner" || defs[0].Hidden || !strings.HasSuffix(defs[0].Icon, "win.jpg") || !strings.HasSuffix(defs[0].IconGray, "win_gray.jpg") {
		t.Errorf("first: %+v", defs[0])
	}
	if defs[1].Name != "Secret" || !defs[1].Hidden || defs[1].Icon != "" || defs[1].IconGray != "" {
		t.Errorf("second: %+v", defs[1])
	}
	if defs, _, _ := GoldbergSchema(g, "french"); defs[0].Name != "Winner" {
		t.Errorf("fallback to English: %q", defs[0].Name)
	}
}

func TestMerge(t *testing.T) {
	defs := []Def{{ID: "ACH_A", Name: "A"}, {ID: "ACH_B"}, {ID: "ACH_C", Name: "C"}}
	got := Merge(defs, map[string]Unlock{
		"ach_a": {Achieved: true, At: 5},
		"ACH_C": {Progress: 1, Max: 4},
		"extra": {Achieved: true},
	})
	if len(got) != 4 {
		t.Fatalf("got %d: %+v", len(got), got)
	}
	if !got[0].Unlocked || got[0].UnlockedAt != 5 || got[1].Name != "ACH_B" || got[2].Max != 4 || got[2].Unlocked || got[3].ID != "extra" || !got[3].Unlocked {
		t.Errorf("merge: %+v", got)
	}
	l := List{Items: got}
	l.Count()
	l.SetRarity(map[string]float64{"ACH_A": 12.5})
	if l.Total != 4 || l.Unlocked != 2 || l.Items[0].Percent == nil || *l.Items[0].Percent != 12.5 || l.Items[1].Percent != nil {
		t.Errorf("list: %+v", l)
	}
}

func TestUnknownEmus(t *testing.T) {
	for _, e := range []string{"PLAZA", "CPY", "FLT", "VOICES38"} {
		if !Unknown(e) {
			t.Errorf("%s should be unknown", e)
		}
	}
	if Unknown("CODEX") {
		t.Error("CODEX is known")
	}
}
