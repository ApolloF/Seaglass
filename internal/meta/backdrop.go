package meta

import (
	"context"
	"image"
	"math"
)

// A backdrop fills the whole big picture screen behind a game's name,
// details and buttons, which sit on its left. The best one is sharp at 4K
// and calm where the text goes: key art or a quiet screenshot rather than
// a busy scene full of interface. Several candidates are loaded and
// scored; the first that scores well enough wins.

// maxBackdropTries caps how many candidates are downloaded per game.
const maxBackdropTries = 6

// goodBackdrop is a score that's not worth looking further than.
const goodBackdrop = 0.85

// backdropCandidate is one picture that could be the backdrop, with
// alternatives (the original screenshot, then Steam's 1920-wide copy of
// it: the first that loads counts). Key art, made to sit behind text,
// gets a bonus over gameplay screenshots, whose interface and action make
// busy backgrounds.
type backdropCandidate struct {
	alts  []string
	bonus float64 // keyArt or epicArt
}

// traceBackdrop sees every candidate and its score (tests).
var traceBackdrop func(src string, img image.Image, score float64)

// How much key art is preferred: Steam's library hero has no text by
// Steam's rules; the Epic store's often carries the title.
const (
	keyArt  = 0.18
	epicArt = 0.08
)

// pickBackdrop stores the best of the candidate images as the backdrop.
func (c *Client) pickBackdrop(ctx context.Context, candidates []backdropCandidate) string {
	var best image.Image
	bestScore := -1.0
	tries := 0
	seen := map[string]bool{}
	for i, cand := range candidates {
		if seen[cand.alts[0]] {
			continue
		}
		seen[cand.alts[0]] = true
		if tries >= c.backdropTries() || bestScore >= goodBackdrop && c.maxTries == 0 {
			break
		}
		var img image.Image
		for _, u := range cand.alts {
			tries++
			if im, err := c.loadImage(ctx, u); err == nil {
				img = im
				break
			}
		}
		if img == nil {
			continue
		}
		img = crop16x9(img)
		if img.Bounds().Dx() < minBackdropWidth {
			continue
		}
		// The order the stores give (the developer's own picks first) counts
		// for a little.
		s := backdropScore(img) - 0.02*float64(i)
		s += cand.bonus
		if traceBackdrop != nil {
			traceBackdrop(cand.alts[0], img, s)
		}
		if s > bestScore {
			best, bestScore = img, s
		}
	}
	if best == nil {
		return ""
	}
	art, _, err := c.keepImage(best, Backdrop)
	if err != nil {
		return ""
	}
	return art
}

// backdropScore rates a 16:9 image as a backdrop from 0 to 1: its
// resolution (3840 wide is best), how calm the left side is, where the
// title and text go (little detail, not too bright), and whether there's
// something to look at on the right (not an empty sky). A near-uniform image
// (a black loading screen) scores 0.
func backdropScore(img image.Image) float64 {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	res := math.Min(1, float64(w)/3840)

	step := max(1, w/240)
	lum := func(x, y int) float64 {
		r, g, bl, _ := img.At(x, y).RGBA()
		return (0.2126*float64(r) + 0.7152*float64(g) + 0.0722*float64(bl)) / 65535
	}
	// The whole picture: is there anything in it?
	var sum, sq float64
	n := 0
	for y := b.Min.Y; y < b.Max.Y; y += step * 2 {
		for x := b.Min.X; x < b.Max.X; x += step * 2 {
			l := lum(x, y)
			sum += l
			sq += l * l
			n++
		}
	}
	if n == 0 {
		return 0
	}
	mean := sum / float64(n)
	if sd := math.Sqrt(math.Max(0, sq/float64(n)-mean*mean)); sd < 0.04 {
		return 0
	}
	// The text side (left 55 %, below the top fifth) and the rest.
	var light, edges, rest float64
	m, k := 0, 0
	for y := b.Min.Y + h/5; y < b.Min.Y+h*95/100-step; y += step {
		for x := b.Min.X; x < b.Max.X-step; x += step {
			l := lum(x, y)
			e := math.Abs(l-lum(x+step, y)) + math.Abs(l-lum(x, y+step))
			if x < b.Min.X+w*55/100 {
				light += l
				edges += e
				m++
			} else {
				rest += e
				k++
			}
		}
	}
	if m == 0 || k == 0 {
		return res
	}
	light /= float64(m)
	edges /= float64(m)
	calm := math.Max(0, 1-edges/0.16)
	calm *= 1 - 0.5*math.Min(1, math.Max(0, (light-0.35)/0.4))
	interest := math.Min(1, rest/float64(k)/0.04)
	return 0.4*res + 0.45*calm + 0.15*interest
}

func (c *Client) backdropTries() int {
	if c.maxTries > 0 {
		return c.maxTries
	}
	return maxBackdropTries
}
