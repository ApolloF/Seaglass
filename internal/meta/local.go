package meta

import (
	"image"
	"os"
	"path/filepath"
	"strings"

	"github.com/ApolloF/Seaglass/internal/library"
)

// addLocal fills art the sources didn't have from the pictures a console
// game came with: a PS3 disc's 1920×1080 background (also the best
// backdrop: the game's own key art, without text), its icon, and covers
// an emulator downloaded. It reports whether any was used.
func (c *Client) addLocal(m *library.Meta, k *library.Console, coverImg, heroImg *image.Image) bool {
	used := false
	if img := loadLocal(k.Backdrop); img != nil {
		if b := crop16x9(img); b.Bounds().Dx() >= minBackdropWidth {
			if art, _, err := c.keepImage(b, Backdrop); err == nil {
				m.Backdrop, used = art, true
			}
		}
		if m.Hero == "" {
			if art, kept, err := c.keepImage(img, Hero); err == nil {
				m.Hero, *heroImg, used = art, kept, true
			}
		}
	}
	if m.Cover == "" {
		if img := loadLocal(k.Cover); img != nil {
			if art, kept, err := c.keepImage(img, Cover); err == nil {
				m.Cover, *coverImg, used = art, kept, true
			}
		}
	}
	if m.Icon == "" {
		if img := loadLocal(k.Icon); img != nil {
			if art, _, err := c.keepImage(img, Icon); err == nil {
				m.Icon, used = art, true
			}
		}
	}
	return used
}

// loadLocal reads a picture from this PC, within the same limits as a
// download. Only PNG and JPEG files are read; nil when it isn't one.
func loadLocal(p string) image.Image {
	if p == "" || !filepath.IsAbs(p) {
		return nil
	}
	switch strings.ToLower(filepath.Ext(p)) {
	case ".png", ".jpg", ".jpeg":
	default:
		return nil
	}
	fi, err := os.Stat(p)
	if err != nil || !fi.Mode().IsRegular() || fi.Size() > maxImageBytes {
		return nil
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	img, err := decodeChecked(b)
	if err != nil {
		return nil
	}
	return img
}
