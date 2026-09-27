package owned

import (
	"context"
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
