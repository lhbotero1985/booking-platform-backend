package media

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func encodePNG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func solid(w, h int, c color.Color) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, c)
		}
	}
	return img
}

// withOrientation inserts an EXIF APP1 segment (orientation o, and a fake GPS tag) after the
// SOI marker of a JPEG.
func withOrientation(jpg []byte, o uint16) []byte {
	tiff := []byte("MM\x00\x2a\x00\x00\x00\x08")
	ifd := make([]byte, 2+2*12+4)
	binary.BigEndian.PutUint16(ifd, 2)
	e := ifd[2:]
	binary.BigEndian.PutUint16(e, 0x0112)
	binary.BigEndian.PutUint16(e[2:], 3)
	binary.BigEndian.PutUint32(e[4:], 1)
	binary.BigEndian.PutUint16(e[8:], o)
	binary.BigEndian.PutUint16(e[12:], 0x8825) // GPS IFD pointer: must not survive
	binary.BigEndian.PutUint16(e[14:], 4)
	binary.BigEndian.PutUint32(e[16:], 1)
	seg := append([]byte("Exif\x00\x00"), append(tiff, ifd...)...)
	app1 := []byte{0xff, 0xe1, 0, 0}
	binary.BigEndian.PutUint16(app1[2:], uint16(len(seg)+2)) //nolint:gosec // tiny test segment
	out := append([]byte{}, jpg[:2]...)
	out = append(out, app1...)
	out = append(out, seg...)
	return append(out, jpg[2:]...)
}

func TestProcess(t *testing.T) {
	// A transparent PNG stays PNG; scaled down to the side, never up.
	img, err := Process(encodePNG(t, solid(800, 400, color.NRGBA{255, 0, 0, 128})), 200)
	if err != nil || img.ContentType != "image/png" || img.Ext != "png" || img.Width != 200 || img.Height != 100 {
		t.Fatalf("transparent png: %+v %v", img, err)
	}
	small, err := Process(encodePNG(t, solid(50, 30, color.White)), 200)
	if err != nil || small.ContentType != "image/jpeg" || small.Width != 50 || small.Height != 30 {
		t.Errorf("opaque png → jpeg, not scaled up: %+v %v", small, err)
	}
	// A JPEG with EXIF orientation 6 (rotated 90°) comes out upright and without metadata.
	var jb bytes.Buffer
	if err := jpeg.Encode(&jb, solid(40, 20, color.Black), nil); err != nil {
		t.Fatal(err)
	}
	rotated, err := Process(withOrientation(jb.Bytes(), 6), 1600)
	if err != nil || rotated.Width != 20 || rotated.Height != 40 || bytes.Contains(rotated.Data, []byte("Exif")) {
		t.Errorf("orientation: %dx%d exif=%v %v", rotated.Width, rotated.Height, bytes.Contains(rotated.Data, []byte("Exif")), err)
	}
	if cfg, err := jpeg.DecodeConfig(bytes.NewReader(rotated.Data)); err != nil || cfg.Width != 20 {
		t.Errorf("output is not a valid JPEG: %v %v", cfg, err)
	}
	for name, c := range map[string]struct {
		data []byte
		want error
	}{
		"pdf":             {[]byte("%PDF-1.7\n1 0 obj"), ErrUnsupported},
		"script as .jpg":  {[]byte("<script>alert(1)</script>"), ErrUnsupported},
		"gif":             {[]byte("GIF89a\x01\x00\x01\x00\x00\x00\x00;"), ErrUnsupported},
		"truncated jpeg":  {jb.Bytes()[:40], ErrInvalid},
		"too wide":        {encodePNG(t, image.NewGray(image.Rect(0, 0, 12_001, 1))), ErrTooBig},
		"png header only": {encodePNG(t, solid(10, 10, color.White))[:33], ErrInvalid},
	} {
		if _, err := Process(c.data, 1600); !errors.Is(err, c.want) {
			t.Errorf("%s: %v, want %v", name, err, c.want)
		}
	}
}

func TestLocalStore(t *testing.T) {
	ctx := context.Background()
	st, err := NewLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	biz := uuid.Must(uuid.NewV7())
	key := NewKey(FolderLogos, biz, Image{Ext: "png"})
	if !ValidKey(key) || !strings.HasPrefix(key, "logos/"+biz.String()+"/") {
		t.Fatalf("key %q", key)
	}
	data := encodePNG(t, solid(4, 4, color.White))
	if err := st.Put(ctx, key, data); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.Handle("GET /media/{key...}", st.Handler())
	get := func(path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil))
		return rec
	}
	rec := get("/media/" + key)
	if rec.Code != http.StatusOK || !bytes.Equal(rec.Body.Bytes(), data) || rec.Header().Get("Content-Type") != "image/png" ||
		rec.Header().Get("Cross-Origin-Resource-Policy") != "cross-origin" || !strings.Contains(rec.Header().Get("Cache-Control"), "immutable") {
		t.Errorf("serve: %d %v", rec.Code, rec.Header())
	}
	for _, bad := range []string{"/media/../../etc/passwd", "/media/logos/x.png", "/media/" + strings.Replace(key, ".png", ".svg", 1), "/media/" + NewKey(FolderCovers, biz, Image{Ext: "jpg"})} {
		if rec := get(bad); rec.Code != http.StatusNotFound && rec.Code != http.StatusTemporaryRedirect { // the router redirects "../" itself
			t.Errorf("%s: %d", bad, rec.Code)
		}
	}
	if err := st.Put(ctx, "../escape.png", data); err == nil {
		t.Error("invalid key stored")
	}
	if err := st.Delete(ctx, key); err != nil || get("/media/"+key).Code != http.StatusNotFound {
		t.Errorf("delete: %v", err)
	}
	if err := st.Delete(ctx, key); err != nil {
		t.Errorf("deleting twice: %v", err)
	}
	SetBaseURL("https://api.example.com/")
	defer SetBaseURL("")
	if u := URL(key); u != "https://api.example.com/media/"+key || URL("") != "" {
		t.Errorf("URL = %q", u)
	}
}

func TestFromRequest(t *testing.T) {
	form := func(field string, data []byte, extra map[string]string) *http.Request {
		var b bytes.Buffer
		mw := multipart.NewWriter(&b)
		for k, v := range extra {
			_ = mw.WriteField(k, v)
		}
		if field != "" {
			fw, _ := mw.CreateFormFile(field, "foto.jpg")
			_, _ = fw.Write(data)
		}
		_ = mw.Close()
		r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", &b)
		r.Header.Set("Content-Type", mw.FormDataContentType())
		return r
	}
	u := FromRequest(form("image", []byte("abc"), map[string]string{"caption": "Degradado"}))
	if data, err := u.Read(); err != nil || string(data) != "abc" || u.Field("caption") != "Degradado" {
		t.Errorf("read: %q %v %q", data, err, u.Field("caption"))
	}
	if _, err := FromRequest(form("otro", []byte("abc"), nil)).Read(); !errors.Is(err, ErrMissingFile) {
		t.Errorf("missing field: %v", err)
	}
	if _, err := FromRequest(form("image", bytes.Repeat([]byte{1}, MaxUploadBytes+1), nil)).Read(); !errors.Is(err, ErrTooLarge) {
		t.Errorf("too large: %v", err)
	}
	j := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", strings.NewReader("{}"))
	j.Header.Set("Content-Type", "application/json")
	if _, err := FromRequest(j).Read(); !errors.Is(err, ErrNotMultipart) {
		t.Errorf("json body: %v", err)
	}
}
