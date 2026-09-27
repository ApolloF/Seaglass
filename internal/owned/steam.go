package owned

import (
	"context"
	"errors"
	"net/url"
	"regexp"
	"strconv"

	"github.com/ApolloF/Seaglass/internal/library"
	"github.com/ApolloF/Seaglass/internal/scan"
	"github.com/ApolloF/gamekit/steam"
)

var reSteamKey = regexp.MustCompile(`^[0-9A-Fa-f]{32}$`)

// ValidSteamKey reports whether s looks like a Steam Web API key.
func ValidSteamKey(s string) bool { return reSteamKey.MatchString(s) }

// SteamID returns the steamID64 of the account Steam uses on this PC.
func SteamID() (string, error) {
	root := steam.Dir()
	if root == "" {
		return "", errors.New("Steam isn't installed")
	}
	accs := steam.Accounts(root, steam.HostInfo())
	if len(accs) == 0 {
		return "", errors.New("no Steam account has signed in on this PC")
	}
	acc, err := strconv.ParseUint(accs[0], 10, 64)
	if err != nil {
		return "", err
	}
	return strconv.FormatUint(acc+steam.IDBase, 10), nil
}

// ErrSteamPrivate means Steam answered without a game list.
var ErrSteamPrivate = errors.New("Steam didn't list any games. In Steam, set your profile's Game details to Public, or check the key")

// Steam lists the games a Steam account owns, with Steam's playtime.
func (c *Client) Steam(ctx context.Context, key, steamID string) ([]library.Owned, error) {
	if !ValidSteamKey(key) {
		return nil, errors.New("that doesn't look like a Steam Web API key (32 letters and digits)")
	}
	q := url.Values{
		"key": {key}, "steamid": {steamID}, "include_appinfo": {"1"},
		"include_played_free_games": {"1"}, "format": {"json"},
	}
	var resp struct {
		Response struct {
			Count int `json:"game_count"`
			Games []struct {
				AppID      int    `json:"appid"`
				Name       string `json:"name"`
				Playtime   int64  `json:"playtime_forever"` // minutes
				LastPlayed int64  `json:"rtime_last_played"`
			} `json:"games"`
		} `json:"response"`
	}
	err := c.do(ctx, "GET", "https://api.steampowered.com/IPlayerService/GetOwnedGames/v1/?"+q.Encode(), nil, nil, &resp)
	var he *HTTPError
	if errors.As(err, &he) && (he.Status == 401 || he.Status == 403) {
		return nil, errors.New("Steam didn't accept the key")
	}
	if err != nil {
		return nil, err
	}
	if resp.Response.Count == 0 && len(resp.Response.Games) == 0 {
		return nil, ErrSteamPrivate
	}
	out := make([]library.Owned, 0, len(resp.Response.Games))
	for _, g := range resp.Response.Games {
		if g.AppID <= 0 || g.Name == "" {
			continue
		}
		id := strconv.Itoa(g.AppID)
		out = append(out, library.Owned{
			Store: "steam", ID: id, Title: g.Name, SortTitle: scan.SortTitle(g.Name),
			InstallURI: "steam://install/" + id, Playtime: g.Playtime * 60, LastPlayed: g.LastPlayed,
		})
	}
	return out, nil
}
