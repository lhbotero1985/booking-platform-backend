package app

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/geography/application/geotest"
)

// pngOf is an opaque w×h PNG (the server returns it as JPEG).
func pngOf(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := range img.Pix {
		img.Pix[i] = 0xff
	}
	img.Set(0, 0, color.RGBA{10, 20, 30, 255})
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// multipartBody is a form with the file field "image" and optional text fields.
func multipartBody(t *testing.T, data []byte, fields map[string]string) (string, map[string]string) {
	t.Helper()
	var b bytes.Buffer
	mw := multipart.NewWriter(&b)
	for k, v := range fields {
		_ = mw.WriteField(k, v)
	}
	fw, err := mw.CreateFormFile("image", "foto.png")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fw.Write(data)
	_ = mw.Close()
	return b.String(), map[string]string{"Content-Type": mw.FormDataContentType()}
}

// TestContractImages (2026-11-27): logo and cover of a business (OWNER) and the portfolio of a
// professional (itself or the OWNER, 10 at most), against api/openapi.yaml.
func TestContractImages(t *testing.T) {
	c := loadContract(t)
	e := newGeoServer(t)
	owner := e.signIn(t, "dueno@example.com")
	barber := e.signIn(t, "juan@example.com")
	outsider := e.signIn(t, "ana@example.com")
	idOf := func(s string) string { return regexp.MustCompile(`"id":"([^"]+)"`).FindStringSubmatch(s)[1] }
	biz := idOf(c.callAuth(t, e.srv, http.MethodPost, "/api/v1/businesses", `{"name":"Barbería Imágenes","contactPhone":"3001234567","cityId":"`+geotest.CaliID.String()+`","addressLine":"Calle 1"}`, owner, http.StatusCreated))
	base := "/api/v1/business/" + biz
	c.callAuth(t, e.srv, http.MethodPost, base+"/members", `{"email":"juan@example.com","role":"PROFESSIONAL"}`, owner, http.StatusCreated)
	juan := idOf(c.callAuth(t, e.srv, http.MethodPost, base+"/professionals", `{"email":"juan@example.com","displayName":"Juan"}`, owner, http.StatusCreated))
	boss := idOf(c.callAuth(t, e.srv, http.MethodPost, base+"/professionals", `{"email":"dueno@example.com","displayName":"El Dueño"}`, owner, http.StatusCreated))
	svc := idOf(c.callAuth(t, e.srv, http.MethodPost, base+"/services", `{"name":"Corte","durationMinutes":30,"priceMinor":2500000}`, owner, http.StatusCreated))
	c.callAuth(t, e.srv, http.MethodPut, base+"/professionals/"+juan+"/services", `{"serviceIds":["`+svc+`"]}`, owner, http.StatusOK)
	type fetched struct {
		StatusCode int
		Header     http.Header
	}
	get := func(url string) (fetched, []byte) {
		t.Helper()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := e.srv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return fetched{resp.StatusCode, resp.Header}, b
	}

	// Logo: the OWNER uploads it; it is served as JPEG, cross-origin, at most 512 px.
	body, h := multipartBody(t, pngOf(t, 1200, 600), nil)
	got := c.callWith(t, e.srv, http.MethodPut, base+"/logo", body, owner, h, http.StatusOK)
	logo := regexp.MustCompile(`"logoUrl":"([^"]+)"`).FindStringSubmatch(got)
	if logo == nil || !strings.Contains(got, `"coverUrl":null`) {
		t.Fatalf("logo: %s", got)
	}
	resp, img := get(e.srv.URL + logo[1])
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "image/jpeg" || resp.Header.Get("Cross-Origin-Resource-Policy") != "cross-origin" {
		t.Fatalf("serving the logo: %d %v", resp.StatusCode, resp.Header)
	}
	if cfg, _, err := image.DecodeConfig(bytes.NewReader(img)); err != nil || cfg.Width != 512 || cfg.Height != 256 {
		t.Errorf("logo size: %+v %v", cfg, err)
	}
	// Replacing it deletes the old file; the business shows the new one everywhere.
	body, h = multipartBody(t, pngOf(t, 300, 300), nil)
	got = c.callWith(t, e.srv, http.MethodPut, base+"/logo", body, owner, h, http.StatusOK)
	if r, _ := get(e.srv.URL + logo[1]); r.StatusCode != http.StatusNotFound || strings.Contains(got, logo[1]) {
		t.Errorf("old logo still served: %d", r.StatusCode)
	}
	body, h = multipartBody(t, pngOf(t, 2000, 1000), nil)
	c.callWith(t, e.srv, http.MethodPut, base+"/cover", body, owner, h, http.StatusOK)
	slug := regexp.MustCompile(`"slug":"([^"]+)"`).FindStringSubmatch(got)[1]
	if pub := c.callAuth(t, e.srv, http.MethodGet, "/api/v1/businesses/"+slug, "", "", http.StatusOK); !strings.Contains(pub, `"logoUrl":"/media/logos/`+biz) || !strings.Contains(pub, `"coverUrl":"/media/covers/`+biz) {
		t.Errorf("public detail: %s", pub)
	}
	if m := c.callAuth(t, e.srv, http.MethodGet, base, "", owner, http.StatusOK); !strings.Contains(m, `"coverUrl":"/media/covers/`) {
		t.Errorf("managed business: %s", m)
	}
	// Only the OWNER; invalid files; another business is a 404 whatever the body.
	body, h = multipartBody(t, pngOf(t, 10, 10), nil)
	c.callWith(t, e.srv, http.MethodPut, base+"/logo", body, barber, h, http.StatusForbidden)
	c.callWith(t, e.srv, http.MethodPut, base+"/logo", body, outsider, h, http.StatusNotFound)
	c.callWith(t, e.srv, http.MethodPut, base+"/logo", `{"image":"x"}`, outsider, nil, http.StatusNotFound)
	for name, data := range map[string][]byte{"pdf": []byte("%PDF-1.7 fake"), "script": []byte("<script>alert(1)</script>")} {
		body, h = multipartBody(t, data, nil)
		if p := c.callWith(t, e.srv, http.MethodPut, base+"/logo", body, owner, h, http.StatusBadRequest); !strings.Contains(p, `"UNSUPPORTED_IMAGE"`) {
			t.Errorf("%s: %s", name, p)
		}
	}
	c.callWith(t, e.srv, http.MethodPut, base+"/logo", `{"image":"x"}`, owner, nil, http.StatusUnsupportedMediaType)
	body, h = multipartBody(t, bytes.Repeat([]byte{0xff}, 5<<20+10), nil)
	c.callWith(t, e.srv, http.MethodPut, base+"/cover", body, owner, h, http.StatusRequestEntityTooLarge)
	c.callAuth(t, e.srv, http.MethodDelete, base+"/cover", "", owner, http.StatusNoContent)
	c.callAuth(t, e.srv, http.MethodDelete, base+"/cover", "", owner, http.StatusNoContent) // already gone
	c.callAuth(t, e.srv, http.MethodDelete, base+"/logo", "", barber, http.StatusForbidden)

	// Portfolio: Juan uploads to his own, not to the owner's; the owner manages anyone's.
	port := base + "/professionals/" + juan + "/portfolio"
	body, h = multipartBody(t, pngOf(t, 3000, 2000), map[string]string{"caption": "Degradado clásico"})
	first := c.callWith(t, e.srv, http.MethodPost, port, body, barber, h, http.StatusCreated)
	if !strings.Contains(first, `"caption":"Degradado clásico"`) || !strings.Contains(first, `"width":1600,"height":1066`) || !strings.Contains(first, `"sortOrder":0`) {
		t.Errorf("first image: %s", first)
	}
	body, h = multipartBody(t, pngOf(t, 10, 10), nil)
	c.callWith(t, e.srv, http.MethodPost, base+"/professionals/"+boss+"/portfolio", body, barber, h, http.StatusForbidden)
	c.callWith(t, e.srv, http.MethodPost, port, body, outsider, h, http.StatusNotFound)
	body, h = multipartBody(t, pngOf(t, 10, 10), map[string]string{"caption": strings.Repeat("x", 141)})
	c.callWith(t, e.srv, http.MethodPost, port, body, owner, h, http.StatusBadRequest)
	for i := range 9 { // up to 10
		body, h = multipartBody(t, pngOf(t, 20+i, 20), nil)
		c.callWith(t, e.srv, http.MethodPost, port, body, owner, h, http.StatusCreated)
	}
	body, h = multipartBody(t, pngOf(t, 10, 10), nil)
	if p := c.callWith(t, e.srv, http.MethodPost, port, body, barber, h, http.StatusConflict); !strings.Contains(p, `"PORTFOLIO_FULL"`) {
		t.Errorf("11th image: %s", p)
	}
	list := c.callAuth(t, e.srv, http.MethodGet, port, "", barber, http.StatusOK)
	if strings.Count(list, `"url":"/media/portfolio/`) != 10 || !strings.Contains(list, `"max":10`) {
		t.Errorf("list: %s", list)
	}
	firstID := idOf(first)
	if p := c.callAuth(t, e.srv, http.MethodPatch, port+"/"+firstID, `{"caption":"Fade","sortOrder":20}`, barber, http.StatusOK); !strings.Contains(p, `"sortOrder":20`) {
		t.Errorf("patch: %s", p)
	}
	c.callAuth(t, e.srv, http.MethodPatch, port+"/"+firstID, `{"sortOrder":-1}`, barber, http.StatusBadRequest)
	if cat := c.callAuth(t, e.srv, http.MethodGet, "/api/v1/businesses/"+slug+"/catalog", "", "", http.StatusOK); strings.Count(cat, `"url":"/media/portfolio/`) != 10 || !strings.Contains(cat, `"caption":"Fade"`) {
		t.Errorf("public catalog: %s", cat)
	}
	firstURL := regexp.MustCompile(`"url":"([^"]+)"`).FindStringSubmatch(first)[1]
	c.callAuth(t, e.srv, http.MethodDelete, port+"/"+firstID, "", outsider, http.StatusNotFound)
	c.callAuth(t, e.srv, http.MethodDelete, port+"/"+firstID, "", barber, http.StatusNoContent)
	c.callAuth(t, e.srv, http.MethodDelete, port+"/"+firstID, "", barber, http.StatusNotFound)
	if r, _ := get(e.srv.URL + firstURL); r.StatusCode != http.StatusNotFound {
		t.Errorf("deleted image still served: %d", r.StatusCode)
	}
	body, h = multipartBody(t, pngOf(t, 10, 10), nil)
	c.callWith(t, e.srv, http.MethodPost, port, body, barber, h, http.StatusCreated) // room again
	if r, _ := get(e.srv.URL + "/media/../../etc/passwd"); r.StatusCode == http.StatusOK {
		t.Error("path traversal served a file")
	}
}
