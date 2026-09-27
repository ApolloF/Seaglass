package owned

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ApolloF/Seaglass/internal/library"
	"github.com/ApolloF/Seaglass/internal/platform"
	"github.com/ApolloF/Seaglass/internal/scan"
	"github.com/ApolloF/Seaglass/internal/sqlite"
)

// GalaxyDB is GOG Galaxy's local database ("" when Galaxy isn't installed).
func GalaxyDB() string {
	p := filepath.Join(platform.ProgramData, "GOG.com", "Galaxy", "storage", "galaxy-2.0.db")
	if platform.IsFile(p) {
		return p
	}
	return ""
}

// GOG lists the GOG games in GOG Galaxy's library, with Galaxy's playtime.
// Galaxy keeps it up to date while it runs; nothing is sent anywhere.
func GOG(dbPath string) ([]library.Owned, error) {
	if dbPath == "" {
		return nil, errors.New("GOG Galaxy isn't installed")
	}
	db, err := sqlite.Open(dbPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, errors.New("GOG Galaxy's library wasn't found")
	}
	if err != nil {
		return nil, err
	}
	defer db.Close()
	if !db.HasTable("LibraryReleases") || !db.HasTable("GamePieces") || !db.HasTable("GamePieceTypes") {
		return nil, errors.New("GOG Galaxy's library has an unknown layout")
	}
	types := map[int64]string{}
	rows, err := db.Rows("GamePieceTypes")
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		id, _ := r["id"].(int64)
		t, _ := r["type"].(string)
		types[id] = t
	}
	// A release's title: "title" (what Galaxy shows), else "originalTitle".
	titles := map[string]string{}
	original := map[string]string{}
	pieces, err := db.Rows("GamePieces")
	if err != nil {
		return nil, err
	}
	for _, r := range pieces {
		key, _ := r["releaseKey"].(string)
		tid, _ := r["gamePieceTypeId"].(int64)
		val, _ := r["value"].(string)
		kind := types[tid]
		if kind != "title" && kind != "originalTitle" || !strings.HasPrefix(key, "gog_") {
			continue
		}
		var v struct {
			Title string `json:"title"`
		}
		if json.Unmarshal([]byte(val), &v) != nil || strings.TrimSpace(v.Title) == "" {
			continue
		}
		if kind == "title" {
			titles[key] = v.Title
		} else {
			original[key] = v.Title
		}
	}
	minutes := map[string]int64{}
	if db.HasTable("GameTimes") {
		rows, _ := db.Rows("GameTimes")
		for _, r := range rows {
			key, _ := r["releaseKey"].(string)
			m, _ := r["minutesInGame"].(int64)
			minutes[key] += m
		}
	}
	last := map[string]int64{}
	if db.HasTable("LastPlayedDates") {
		rows, _ := db.Rows("LastPlayedDates")
		for _, r := range rows {
			key, _ := r["gameReleaseKey"].(string)
			if s, ok := r["lastPlayedDate"].(string); ok {
				if t, err := time.Parse(time.DateTime, s); err == nil {
					last[key] = t.Unix()
				}
			}
		}
	}
	releases, err := db.Rows("LibraryReleases")
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []library.Owned
	for _, r := range releases {
		key, _ := r["releaseKey"].(string)
		id := strings.TrimPrefix(key, "gog_")
		if !strings.HasPrefix(key, "gog_") || id == "" || seen[key] {
			continue
		}
		seen[key] = true
		title := titles[key]
		if title == "" {
			title = original[key]
		}
		if title == "" {
			continue
		}
		out = append(out, library.Owned{
			Store: "gog", ID: id, Title: title, SortTitle: scan.SortTitle(title),
			InstallURI: "goggalaxy://openGameView/" + key, Playtime: minutes[key] * 60, LastPlayed: last[key],
		})
	}
	return out, nil
}
