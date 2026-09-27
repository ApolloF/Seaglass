package meta

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStoreImageAndHandler(t *testing.T) {
	dir := t.TempDir()
	img := image.NewNRGBA(image.Rect(0, 0, 256, 256))
	img.Set(3, 3, color.White)
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	name, err := StoreImage(dir, b.Bytes(), Icon)
	if err != nil || !reArtName.MatchString(name) || !strings.HasSuffix(name, ".png") {
		t.Fatalf("name %q err %v", name, err)
	}
	if _, err := StoreImage(dir, []byte("<svg/>"), Icon); err == nil {
		t.Error("not an image: stored")
	}
	h := ImageHandler("/ach/", dir)(http.NotFoundHandler())
	for path, want := range map[string]int{"/ach/" + name: 200, "/ach/../x.png": 404, "/ach/zz.png": 404, "/index.html": 404} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if rec.Code != want {
			t.Errorf("%s: %d, want %d", path, rec.Code, want)
		}
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/ach/"+name, nil))
	stored, _, err := image.DecodeConfig(rec.Body)
	if err != nil || stored.Width != maxWidth[Icon] {
		t.Errorf("served %+v %v (want scaled to %d)", stored, err, maxWidth[Icon])
	}
}
