// Command mockmeta fetches metadata and art for a list of games the way
// the app does (internal/meta), so the mock mode can show real games for
// screenshots. Art lands in <out>/art/ under the same /art/ URLs the app
// uses; the metadata goes to <out>/meta.json, keyed like the input.
//
//	mockmeta <games.json> <out>
//
// games.json: {"<key>": {"title": "…", "steamAppId": 123, "gogId": "…"}, …}
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/ApolloF/Seaglass/internal/library"
	"github.com/ApolloF/Seaglass/internal/meta"
)

type game struct {
	Title      string `json:"title"`
	SteamAppID int    `json:"steamAppId"`
	GogID      string `json:"gogId"`
}

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: mockmeta <games.json> <out>")
		os.Exit(2)
	}
	if err := run(os.Args[1], os.Args[2]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(in, out string) error {
	b, err := os.ReadFile(in)
	if err != nil {
		return err
	}
	var games map[string]game
	if err := json.Unmarshal(b, &games); err != nil {
		return fmt.Errorf("%s: %w", in, err)
	}
	artDir := filepath.Join(out, "art")
	if err := os.MkdirAll(artDir, 0o755); err != nil {
		return err
	}
	keys := make([]string, 0, len(games))
	for k := range games {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// No SteamGridDB key: what the app fetches out of the box.
	c := meta.NewClient(artDir, func() string { return "" })
	metas := map[string]*library.Meta{}
	for _, k := range keys {
		g := games[k]
		m, err := c.Fetch(context.Background(), meta.Request{Title: g.Title, SteamAppID: g.SteamAppID, GogID: g.GogID})
		if err != nil {
			return fmt.Errorf("%s: %w", g.Title, err)
		}
		metas[k] = m
		fmt.Printf("  %s: cover %t, hero %t, backdrop %t, logo %t\n", g.Title, m.Cover != "", m.Hero != "", m.Backdrop != "", m.Logo != "")
	}
	j, err := json.MarshalIndent(metas, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(out, "meta.json"), j, 0o644)
}
