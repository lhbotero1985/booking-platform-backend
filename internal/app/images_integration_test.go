//go:build integration

package app

import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"sync"
	"testing"

	"github.com/google/uuid"

	bizapp "github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/application"
	profapp "github.com/lhbotero1985/booking-platform-backend/internal/modules/professionals/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/media"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/pgtest"
)

// TestImagesOnPostgres (2026-11-27): business logo and cover keys in every read of a business,
// the portfolio queries, the limit of 10 under concurrent uploads and the schema constraints.
func TestImagesOnPostgres(t *testing.T) {
	ctx := t.Context()
	w := newBookingWorld(t)
	store, err := media.NewLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	w.bizSvc.SetMedia(store)
	w.profs.SetMedia(store)
	pic := func(side int) media.Upload {
		img := image.NewRGBA(image.Rect(0, 0, side, side))
		for i := range img.Pix {
			img.Pix[i] = 0xff
		}
		var b bytes.Buffer
		if err := png.Encode(&b, img); err != nil {
			t.Fatal(err)
		}
		return media.Bytes(b.Bytes(), map[string]string{"caption": "Corte"})
	}

	b, err := w.bizSvc.SetImage(ctx, w.owner, w.biz, bizapp.ImageLogo, pic(40))
	if err != nil || b.Images().Logo == "" {
		t.Fatalf("logo: %v %v", b, err)
	}
	if _, err := w.bizSvc.SetImage(ctx, w.owner, w.biz, bizapp.ImageCover, pic(40)); err != nil {
		t.Fatal(err)
	}
	// The keys come back in every read: by id, by slug (public) and in the public search.
	m, err := w.bizSvc.GetManaged(ctx, w.owner, w.biz)
	if err != nil || m.Business.Images().Logo != b.Images().Logo || m.Business.Images().Cover == "" {
		t.Errorf("managed: %+v %v", m.Business.Images(), err)
	}
	if pub, err := w.bizSvc.GetPublic(ctx, w.slug); err != nil || pub.Images().Cover != m.Business.Images().Cover {
		t.Errorf("public: %v", err)
	}
	if hits, _, err := w.bizSvc.SearchPublic(ctx, bizapp.SearchQuery{CityID: pgtest.SeedCaliID, Text: "Reservas"}); err != nil || len(hits) == 0 || hits[0].Images().Logo != b.Images().Logo {
		t.Errorf("search: %d %v", len(hits), err)
	}
	if _, err := w.bizSvc.RemoveImage(ctx, w.owner, w.biz, bizapp.ImageCover); err != nil {
		t.Fatal(err)
	}
	if n := count(t, w.pool, `SELECT count(*) FROM businesses WHERE id = $1 AND cover_key IS NULL AND logo_key IS NOT NULL`, w.biz); n != 1 {
		t.Errorf("cover removed, logo kept: %d", n)
	}
	if _, err := w.pool.Exec(ctx, `UPDATE businesses SET logo_key = '../../etc/passwd' WHERE id = $1`, w.biz); err == nil {
		t.Error("an invalid key passed the schema")
	}

	// Twelve simultaneous uploads to Juan's portfolio: exactly ten are kept.
	var wg sync.WaitGroup
	errs := make([]error, 12)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = w.profs.AddPortfolioImage(ctx, w.barber, w.biz, w.pro, pic(30+i))
		}(i)
	}
	wg.Wait()
	full := 0
	for _, err := range errs {
		switch {
		case errors.Is(err, profapp.ErrPortfolioFull):
			full++
		case err != nil:
			t.Fatalf("upload: %v", err)
		}
	}
	list, err := w.profs.ListPortfolio(ctx, w.owner, w.biz, w.pro)
	if err != nil || len(list) != 10 || full != 2 {
		t.Fatalf("portfolio: %d images, %d refused, %v", len(list), full, err)
	}
	if n := count(t, w.pool, `SELECT count(*) FROM professional_portfolio_images WHERE professional_id = $1 AND caption = 'Corte'`, w.pro); n != 10 {
		t.Errorf("rows = %d", n)
	}
	// Public: the catalog's professionals carry their portfolio, in order.
	pros, err := w.profs.BookableProfessionals(ctx, []uuid.UUID{w.biz})
	if err != nil || len(pros[w.biz]) == 0 || len(pros[w.biz][0].Portfolio) != 10 {
		t.Errorf("public portfolio: %+v %v", pros, err)
	}
	moved := 500
	if _, err := w.profs.UpdatePortfolioImage(ctx, w.owner, w.biz, w.pro, list[0].ID, profapp.PortfolioPatch{SortOrder: &moved}); err != nil {
		t.Fatal(err)
	}
	again, _ := w.profs.ListPortfolio(ctx, w.owner, w.biz, w.pro)
	if again[9].ID != list[0].ID {
		t.Errorf("sort order not applied")
	}
	if err := w.profs.DeletePortfolioImage(ctx, w.barber, w.biz, w.pro, list[1].ID); err != nil {
		t.Fatal(err)
	}
	if n := count(t, w.pool, `SELECT count(*) FROM professional_portfolio_images WHERE professional_id = $1`, w.pro); n != 9 {
		t.Errorf("after delete: %d", n)
	}
}
