package meta

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	_ "image/gif"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

// Kind is which picture of a game an image is.
type Kind string

const (
	Cover    Kind = "cover"    // portrait, 2:3
	Hero     Kind = "hero"     // wide banner behind the details
	Backdrop Kind = "backdrop" // 16:9, sharp enough to fill the big picture screen
	Tile     Kind = "tile"     // square: key art with the logo, for round tiles
	Logo     Kind = "logo"     // transparent title logo
	Icon     Kind = "icon"
)

const (
	maxImageBytes  = 24 << 20
	maxImagePixels = 24_000_000 // refuse decompression bombs (8K banners still fit)
)

// Largest stored size per kind; larger images are scaled down. Backdrops
// are kept up to 3840 wide: they fill the screen, and on a 4K screen
// anything smaller is visibly soft.
var maxWidth = map[Kind]int{Cover: 600, Hero: 1920, Backdrop: 3840, Tile: 384, Logo: 800, Icon: 128}

// minBackdropWidth is the least a backdrop may have after cropping to
// 16:9: anything smaller looks soft full screen, and the hero does as well.
const minBackdropWidth = 1280

var errTooSmall = errors.New("image too small for a backdrop")

// ArtURL is the path the interface loads a stored image from.
const artPrefix = "/art/"

var reArtName = regexp.MustCompile(`^[a-f0-9]{40}\.(jpg|png)$`)

// saveImage downloads an image, checks and re-encodes it, and stores it.
// It returns the art URL and the decoded image (for accent colours).
func (c *Client) saveImage(ctx context.Context, src string, kind Kind) (string, image.Image, error) {
	img, err := c.loadImage(ctx, src)
	if err != nil {
		return "", nil, err
	}
	switch kind {
	case Backdrop:
		if img = crop16x9(img); img.Bounds().Dx() < minBackdropWidth {
			return "", nil, errTooSmall
		}
	case Logo:
		// A title logo is wide; a square one is an icon (the Epic store
		// lists its icon as the logo for some games).
		if b := img.Bounds(); b.Dx()*10 < b.Dy()*13 {
			return "", nil, errors.New("not a title logo")
		}
	}
	return c.keepImage(img, kind)
}

// loadImage downloads and decodes an image, within the size limits.
func (c *Client) loadImage(ctx context.Context, src string) (image.Image, error) {
	b, err := c.get(ctx, src, maxImageBytes, nil)
	if err != nil {
		return nil, err
	}
	return decodeChecked(b)
}

// decodeChecked decodes image data within the size limits.
func decodeChecked(b []byte) (image.Image, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("not an image: %w", err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width*cfg.Height > maxImagePixels {
		return nil, errors.New("image too large")
	}
	img, _, err := image.Decode(bytes.NewReader(b))
	return img, err
}

// StoreImage checks image data (a local file, say), re-encodes it at the
// kind's size and stores it in dir, returning the stored file's name.
func StoreImage(dir string, data []byte, kind Kind) (string, error) {
	if len(data) > maxImageBytes {
		return "", errors.New("image too large")
	}
	img, err := decodeChecked(data)
	if err != nil {
		return "", err
	}
	return encodeAndStore(dir, fit(img, maxWidth[kind]), kind)
}

// FetchImage downloads an image from an allowlisted host and stores it
// like StoreImage.
func (c *Client) FetchImage(ctx context.Context, src, dir string, kind Kind) (string, error) {
	b, err := c.get(ctx, src, maxImageBytes, nil)
	if err != nil {
		return "", err
	}
	return StoreImage(dir, b, kind)
}

// keepImage re-encodes an image at the kind's size and stores it.
func (c *Client) keepImage(img image.Image, kind Kind) (string, image.Image, error) {
	img = fit(img, maxWidth[kind])
	name, err := encodeAndStore(c.artDir, img, kind)
	if err != nil {
		return "", nil, err
	}
	return artPrefix + name, img, nil
}

// encodeAndStore encodes an image (PNG for logos and icons, else JPEG) and
// stores it in dir under its content hash, returning the file name.
func encodeAndStore(dir string, img image.Image, kind Kind) (string, error) {
	var err error
	var out bytes.Buffer
	ext := "jpg"
	if kind == Logo || kind == Icon {
		ext = "png"
		err = (&png.Encoder{CompressionLevel: png.BestCompression}).Encode(&out, img)
	} else {
		err = jpeg.Encode(&out, flatten(img), &jpeg.Options{Quality: 88})
	}
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(out.Bytes())
	name := hex.EncodeToString(sum[:20]) + "." + ext
	p := filepath.Join(dir, name)
	if _, err := os.Stat(p); err != nil {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", err
		}
		// A temporary file of its own: the same picture can be fetched
		// twice at once, and both land on the same name.
		f, err := os.CreateTemp(dir, name+".*.tmp")
		if err != nil {
			return "", err
		}
		tmp := f.Name()
		_, err = f.Write(out.Bytes())
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err == nil {
			err = os.Rename(tmp, p)
		}
		if err != nil {
			_ = os.Remove(tmp)
			if _, serr := os.Stat(p); serr != nil {
				return "", err
			}
		}
	}
	return name, nil
}

// fit scales img down to at most maxW pixels wide.
func fit(img image.Image, maxW int) image.Image {
	b := img.Bounds()
	if maxW <= 0 || b.Dx() <= maxW {
		return img
	}
	h := int(math.Round(float64(b.Dy()) * float64(maxW) / float64(b.Dx())))
	dst := image.NewNRGBA(image.Rect(0, 0, maxW, max(1, h)))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, b, draw.Src, nil)
	return dst
}

// flatten puts transparent pixels on black, since JPEG has no alpha.
func flatten(img image.Image) image.Image {
	b := img.Bounds()
	dst := image.NewRGBA(b)
	draw.Draw(dst, b, image.NewUniform(color.Black), image.Point{}, draw.Src)
	draw.Draw(dst, b, img, b.Min, draw.Over)
	return dst
}

// cropCover cuts a 2:3 portrait out of the middle of a wide image, for
// games that only have banner art.
func cropCover(img image.Image) image.Image {
	b := img.Bounds()
	w := b.Dy() * 2 / 3
	if w >= b.Dx() {
		return img
	}
	x := b.Min.X + (b.Dx()-w)/2
	return crop(img, image.Rect(x, b.Min.Y, x+w, b.Max.Y))
}

// crop returns r of img, sharing its pixels when the image type allows it.
func crop(img image.Image, r image.Rectangle) image.Image {
	if s, ok := img.(interface {
		SubImage(image.Rectangle) image.Image
	}); ok {
		return s.SubImage(r)
	}
	dst := image.NewNRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	draw.Draw(dst, dst.Bounds(), img, r.Min, draw.Src)
	return dst
}

// crop16x9 cuts the middle 16:9 out of an image: the sides of a wide
// banner, or the top and bottom of a 4:3 screenshot.
func crop16x9(img image.Image) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if cw := h * 16 / 9; cw < w {
		w = cw
	} else if ch := w * 9 / 16; ch < h {
		h = ch
	}
	if w == b.Dx() && h == b.Dy() {
		return img
	}
	x, y := b.Min.X+(b.Dx()-w)/2, b.Min.Y+(b.Dy()-h)/2
	return crop(img, image.Rect(x, y, x+w, y+h))
}

// makeTile draws a game's round tile: the middle of its key art (the hero,
// which has no text on it) with the logo over it, the way a console shows
// a game. Without a hero, the top of the cover, whose title is part of it.
// Nil when there's neither.
func makeTile(hero, logo, cover image.Image) image.Image {
	const side = 384
	dst := image.NewRGBA(image.Rect(0, 0, side, side))
	square := func(img image.Image, fromTop int) image.Rectangle {
		b := img.Bounds()
		s := min(b.Dx(), b.Dy())
		at := image.Pt(b.Min.X+(b.Dx()-s)/2, b.Min.Y+(b.Dy()-s)*fromTop/100)
		return image.Rectangle{Min: at, Max: at.Add(image.Pt(s, s))}
	}
	switch {
	case hero != nil:
		draw.CatmullRom.Scale(dst, dst.Bounds(), hero, square(hero, 50), draw.Src, nil)
	case cover != nil:
		draw.CatmullRom.Scale(dst, dst.Bounds(), cover, square(cover, 18), draw.Src, nil)
		return dst
	default:
		return nil
	}
	if logo == nil {
		return dst
	}
	// Darken behind the logo, so a light logo reads on light art.
	for y := 0; y < side; y++ {
		for x := 0; x < side; x++ {
			dx := (float64(x) - side/2) / (side * 0.42)
			dy := (float64(y) - side*0.56) / (side * 0.26)
			if d := dx*dx + dy*dy; d < 1 {
				i := dst.PixOffset(x, y)
				k := 1 - 0.38*(1-d)
				for j := 0; j < 3; j++ {
					dst.Pix[i+j] = uint8(float64(dst.Pix[i+j]) * k)
				}
			}
		}
	}
	lb := logo.Bounds()
	maxW, maxH := side*74/100, side*40/100
	w, h := maxW, lb.Dy()*maxW/max(1, lb.Dx())
	if h > maxH {
		w, h = lb.Dx()*maxH/max(1, lb.Dy()), maxH
	}
	at := image.Pt((side-w)/2, side*56/100-h/2)
	draw.CatmullRom.Scale(dst, image.Rectangle{Min: at, Max: at.Add(image.Pt(max(1, w), max(1, h)))}, logo, lb, draw.Over, nil)
	return dst
}

// storeImage encodes an image the pipeline made itself (a cropped cover).
func (c *Client) storeImage(img image.Image, kind Kind) (string, error) {
	art, _, err := c.keepImage(img, kind)
	return art, err
}

// Accent picks a lively colour from an image: the most prominent hue among
// saturated, bright-enough pixels, brought to a lightness that reads on a
// dark background. "" when the image is too grey.
func Accent(img image.Image) string {
	if img == nil {
		return ""
	}
	b := img.Bounds()
	const bins = 24
	var weight [bins]float64
	var sumR, sumG, sumB [bins]float64
	step := max(1, min(b.Dx(), b.Dy())/64)
	for y := b.Min.Y; y < b.Max.Y; y += step {
		for x := b.Min.X; x < b.Max.X; x += step {
			r, g, bl, _ := img.At(x, y).RGBA()
			rf, gf, bf := float64(r)/65535, float64(g)/65535, float64(bl)/65535
			h, s, v := hsv(rf, gf, bf)
			if v < 0.25 || s < 0.25 {
				continue
			}
			w := s * s * v
			i := int(h/360*bins) % bins
			weight[i] += w
			sumR[i] += rf * w
			sumG[i] += gf * w
			sumB[i] += bf * w
		}
	}
	best, total := 0, 0.0
	for i := range weight {
		total += weight[i]
		if weight[i] > weight[best] {
			best = i
		}
	}
	if weight[best] == 0 || weight[best] < total*0.08 {
		return ""
	}
	r, g, bl := sumR[best]/weight[best], sumG[best]/weight[best], sumB[best]/weight[best]
	h, s, _ := hsv(r, g, bl)
	r, g, bl = hsvToRGB(h, math.Max(0.45, math.Min(0.75, s)), 0.92)
	return fmt.Sprintf("#%02x%02x%02x", int(r*255+0.5), int(g*255+0.5), int(bl*255+0.5))
}

func hsv(r, g, b float64) (h, s, v float64) {
	mx := math.Max(r, math.Max(g, b))
	mn := math.Min(r, math.Min(g, b))
	v = mx
	d := mx - mn
	if mx > 0 {
		s = d / mx
	}
	switch {
	case d == 0:
		h = 0
	case mx == r:
		h = math.Mod((g-b)/d, 6)
	case mx == g:
		h = (b-r)/d + 2
	default:
		h = (r-g)/d + 4
	}
	h *= 60
	if h < 0 {
		h += 360
	}
	return
}

func hsvToRGB(h, s, v float64) (float64, float64, float64) {
	c := v * s
	x := c * (1 - math.Abs(math.Mod(h/60, 2)-1))
	m := v - c
	var r, g, b float64
	switch {
	case h < 60:
		r, g, b = c, x, 0
	case h < 120:
		r, g, b = x, c, 0
	case h < 180:
		r, g, b = 0, c, x
	case h < 240:
		r, g, b = 0, x, c
	case h < 300:
		r, g, b = x, 0, c
	default:
		r, g, b = c, 0, x
	}
	return r + m, g + m, b + m
}

// ArtHandler serves stored art under /art/ to the interface, and passes
// every other request on.
func ArtHandler(artDir string) func(http.Handler) http.Handler {
	return ImageHandler(artPrefix, artDir)
}

// ImageHandler serves the images stored in dir (content-addressed, as
// StoreImage names them) under prefix, and passes every other request on.
func ImageHandler(prefix, dir string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasPrefix(r.URL.Path, prefix) {
				next.ServeHTTP(w, r)
				return
			}
			name := strings.TrimPrefix(r.URL.Path, prefix)
			if !reArtName.MatchString(name) {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			if strings.HasSuffix(name, ".png") {
				w.Header().Set("Content-Type", "image/png")
			} else {
				w.Header().Set("Content-Type", "image/jpeg")
			}
			http.ServeFile(w, r, filepath.Join(dir, name))
		})
	}
}

// PruneArt deletes stored images no game uses anymore (art replaced by a
// refresh, games removed) and leftovers of cut-short writes. keep holds the
// art URLs in use. Files younger than grace stay: a fetch may be about to
// use them.
func PruneArt(artDir string, keep map[string]bool, grace time.Duration) (removed int, freed int64) {
	entries, err := os.ReadDir(artDir)
	if err != nil {
		return 0, 0
	}
	cutoff := time.Now().Add(-grace)
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || keep[artPrefix+name] || !(reArtName.MatchString(name) || strings.HasSuffix(name, ".tmp")) {
			continue
		}
		fi, err := e.Info()
		if err != nil || fi.ModTime().After(cutoff) {
			continue
		}
		if os.Remove(filepath.Join(artDir, name)) == nil {
			removed++
			freed += fi.Size()
		}
	}
	return removed, freed
}

// IsStoredArt reports whether url names an image stored in artDir.
func IsStoredArt(artDir, url string) bool {
	name, ok := strings.CutPrefix(url, artPrefix)
	return ok && reArtName.MatchString(name) && func() bool {
		fi, err := os.Stat(filepath.Join(artDir, name))
		return err == nil && fi.Mode().IsRegular()
	}()
}
