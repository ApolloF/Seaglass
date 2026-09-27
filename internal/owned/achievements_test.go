package owned

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const testKey = "0123456789ABCDEF0123456789ABCDEF"

func TestSteamAchievements(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		switch {
		case strings.Contains(r.URL.Path, "GetSchemaForGame"):
			if r.URL.Query().Get("l") != "german" {
				t.Errorf("language %q", r.URL.Query().Get("l"))
			}
			w.Write([]byte(`{"game":{"availableGameStats":{"achievements":[
				{"name":"ACH_A","displayName":"Alfa","description":"a","hidden":0,"icon":"https://cdn.akamai.steamstatic.com/a.jpg","icongray":"http://evil/x.jpg"},
				{"name":"ACH_B","displayName":"Beta","hidden":1}]}}}`))
		case strings.Contains(r.URL.Path, "GetPlayerAchievements"):
			if r.URL.Query().Get("appid") == "404" {
				w.WriteHeader(403)
				w.Write([]byte(`{"playerstats":{"error":"Profile is not public","success":false}}`))
				return
			}
			w.Write([]byte(`{"playerstats":{"success":true,"achievements":[{"apiname":"ACH_A","achieved":1,"unlocktime":1700000000},{"apiname":"ACH_B","achieved":0,"unlocktime":0}]}}`))
		case strings.Contains(r.URL.Path, "GetGlobalAchievementPercentages"):
			if r.URL.Query().Get("gameid") == "429" {
				w.WriteHeader(429)
				return
			}
			w.Write([]byte(`{"achievementpercentages":{"achievements":[{"name":"ACH_A","percent":"12.5"},{"name":"ACH_B","percent":3.25}]}}`))
		}
	}))
	defer srv.Close()
	c := NewClient()
	c.base = map[string]string{"api.steampowered.com": srv.URL}
	ctx := context.Background()

	defs, err := c.SteamAchievementSchema(ctx, testKey, 620, "german")
	if err != nil || len(defs) != 2 {
		t.Fatalf("schema %+v %v", defs, err)
	}
	if defs[0].Name != "Alfa" || defs[0].Icon == "" || defs[0].IconGray != "" || !defs[1].Hidden {
		t.Errorf("schema: %+v", defs)
	}
	u, err := c.SteamPlayerAchievements(ctx, testKey, "7656", 620)
	if err != nil || !u["ACH_A"].Achieved || u["ACH_A"].At != 1_700_000_000 || u["ACH_B"].Achieved {
		t.Errorf("player %+v %v", u, err)
	}
	if _, err := c.SteamPlayerAchievements(ctx, testKey, "7656", 404); !errors.Is(err, ErrNoAchievements) {
		t.Errorf("private profile: %v", err)
	}
	r, err := c.SteamRarity(ctx, 620)
	if err != nil || r["ACH_A"] != 12.5 || r["ACH_B"] != 3.25 {
		t.Errorf("rarity %v %v", r, err)
	}
	// Too many requests: the next Steam call waits.
	if _, err := c.SteamRarity(ctx, 429); err == nil {
		t.Error("429 must fail")
	}
	start := time.Now()
	if _, err := c.SteamRarity(ctx, 620); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) < time.Second {
		t.Error("didn't wait after 429")
	}
	if _, err := c.SteamAchievementSchema(ctx, "nokey", 620, "english"); err == nil {
		t.Error("an invalid key must fail before calling Steam")
	}
}

func TestEpicAchievements(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Query     string            `json:"query"`
			Variables map[string]string `json:"variables"`
		}
		if r.Method != "POST" || r.Header.Get("Content-Type") != "application/json" || json.NewDecoder(r.Body).Decode(&req) != nil {
			t.Errorf("bad request %s %s", r.Method, r.Header.Get("Content-Type"))
		}
		switch {
		case strings.Contains(req.Query, "PlayerAchievement"):
			if r.Header.Get("Authorization") != "bearer tok" || req.Variables["epicAccountId"] != "acc" {
				w.WriteHeader(401)
				return
			}
			w.Write([]byte(`{"data":{"PlayerAchievement":{"playerAchievementGameRecordsBySandbox":{"records":[{"playerAchievements":[
				{"playerAchievement":{"achievementName":"a1","unlocked":true,"unlockDate":"2023-11-14T22:13:20.000Z","progress":1}},
				{"playerAchievement":{"achievementName":"a2","unlocked":false,"unlockDate":"0001-01-01T00:00:00.000Z","progress":0.5}}]}]}}}}`))
		case req.Variables["sandboxId"] == "none":
			w.Write([]byte(`{"data":{"Achievement":{"productAchievementsRecordBySandbox":null}}}`))
		default:
			if r.Header.Get("Authorization") != "" || req.Variables["locale"] != "de" {
				t.Errorf("definitions request: auth %q locale %q", r.Header.Get("Authorization"), req.Variables["locale"])
			}
			w.Write([]byte(`{"data":{"Achievement":{"productAchievementsRecordBySandbox":{"achievements":[
				{"achievement":{"name":"a1","hidden":false,"unlockedDisplayName":"Eins","lockedDisplayName":"?","unlockedDescription":"d","unlockedIconLink":"https://cdn1.epicgames.com/a1.png","lockedIconLink":"https://cdn1.epicgames.com/a1l.png","rarity":{"percent":12.5}}},
				{"achievement":{"name":"a2","hidden":true,"unlockedDisplayName":"","lockedDisplayName":"Zwei","rarity":{"percent":0}}}]}}}}`))
		}
	}))
	defer srv.Close()
	c := NewClient()
	c.base = map[string]string{"launcher.store.epicgames.com": srv.URL}
	ctx := context.Background()
	defs, rarity, err := c.EpicAchievements(ctx, "ns", EpicLocale("german"))
	if err != nil || len(defs) != 2 || defs[0].Name != "Eins" || defs[0].IconGray == "" || defs[1].Name != "Zwei" || !defs[1].Hidden || rarity["a1"] != 12.5 {
		t.Fatalf("defs %+v rarity %v err %v", defs, rarity, err)
	}
	if _, _, err := c.EpicAchievements(ctx, "none", "en-US"); !errors.Is(err, ErrNoAchievements) {
		t.Errorf("none: %v", err)
	}
	u, err := c.EpicPlayerAchievements(ctx, "tok", "acc", "ns")
	if err != nil || !u["a1"].Achieved || u["a1"].At != 1_700_000_000 || u["a2"].Achieved || u["a2"].At != 0 {
		t.Errorf("player %+v %v", u, err)
	}
	if _, err := c.EpicPlayerAchievements(ctx, "old", "acc", "ns"); err == nil || !strings.Contains(err.Error(), "sign in again") {
		t.Errorf("expired sign-in: %v", err)
	}
}

func TestGOGUnlocks(t *testing.T) {
	u, err := GOGUnlocks("testdata/galaxy-2.0.db", "1453375253")
	if err != nil || len(u) != 3 {
		t.Fatalf("%+v %v", u, err)
	}
	if !u["ACH_WIN"].Achieved || u["ACH_WIN"].At != 1_700_000_000 || u["ACH_LOSE"].Achieved || u["ACH_EPOCH"].At != 1_700_000_100 {
		t.Errorf("%+v", u)
	}
	if _, err := GOGUnlocks("testdata/missing.db", "1"); err == nil {
		t.Error("missing database: no error")
	}
}

func TestGOGCode(t *testing.T) {
	for in, want := range map[string]string{
		"https://embed.gog.com/on_login_success?origin=client&code=AbC_def-123456789012345": "AbC_def-123456789012345",
		"  AbC_def-123456789012345XYZ  ":                       "AbC_def-123456789012345XYZ",
		"https://embed.gog.com/on_login_success?origin=client": "",
	} {
		if got, _ := GOGCode(in); got != want {
			t.Errorf("GOGCode(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestGOGAchievementsAPI(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/token":
			q := r.URL.Query()
			if q.Get("client_id") != gogClientID || q.Get("grant_type") != "refresh_token" || q.Get("refresh_token") != "r1" {
				w.WriteHeader(400)
				return
			}
			w.Write([]byte(`{"access_token":"a2","refresh_token":"r2","user_id":"u1","expires_in":3600}`))
		case r.URL.Path == "/clients/5134/users/u1/achievements":
			if r.Header.Get("Authorization") != "Bearer a2" {
				w.WriteHeader(401)
				return
			}
			w.Write([]byte(`{"total_count":2,"items":[
				{"achievement_key":"W","visible":true,"name":"Win","description":"d","image_url_unlocked":"https://images.gog-statics.com/w.png","image_url_locked":"https://images.gog-statics.com/wl.png","date_unlocked":"2023-11-14T22:13:20+0000","rarity":12.5},
				{"achievement_key":"S","visible":false,"name":"Secret","date_unlocked":null,"rarity":1}]}`))
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	c := NewClient()
	c.base = map[string]string{"auth.gog.com": srv.URL, "gameplay.gog.com": srv.URL}
	ctx := context.Background()
	tok, err := c.GOGRefresh(ctx, "r1")
	if err != nil || tok.AccessToken != "a2" || tok.RefreshToken != "r2" || tok.UserID != "u1" {
		t.Fatalf("refresh %+v %v", tok, err)
	}
	if _, err := c.GOGRefresh(ctx, "bad"); err == nil || !strings.Contains(err.Error(), "sign in again") {
		t.Errorf("bad refresh: %v", err)
	}
	defs, u, rarity, err := c.GOGAchievements(ctx, "a2", "5134", "u1")
	if err != nil || len(defs) != 2 || defs[0].Name != "Win" || defs[0].Hidden || !defs[1].Hidden || !u["W"].Achieved || u["W"].At != 1_700_000_000 || u["S"].Achieved || rarity["W"] != 12.5 {
		t.Fatalf("defs %+v unlocks %+v rarity %v err %v", defs, u, rarity, err)
	}
	if _, _, _, err := c.GOGAchievements(ctx, "a2", "999", "u1"); !errors.Is(err, ErrNoAchievements) {
		t.Errorf("unknown game: %v", err)
	}
}
