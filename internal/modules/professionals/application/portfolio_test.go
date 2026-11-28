package application_test

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/professionals/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/professionals/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/authz"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/media"
)

func photo(t *testing.T, side int, caption string) media.Upload {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, side, side))
	for i := range img.Pix {
		img.Pix[i] = 0xff
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return media.Bytes(b.Bytes(), map[string]string{"caption": caption})
}

// files counts the images stored under dir.
func files(t *testing.T, dir string) int {
	t.Helper()
	n := 0
	_ = filepath.WalkDir(dir, func(_ string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			n++
		}
		return nil
	})
	return n
}

// TestPortfolio: the professional manages its own, the OWNER anyone's; 10 at most; a refused
// upload leaves no file behind; deleting removes the file.
func TestPortfolio(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	dir := t.TempDir()
	store, err := media.NewLocal(dir)
	if err != nil {
		t.Fatal(err)
	}
	juan, _ := e.svc.Create(ctx, e.owner, e.biz, application.CreateInput{Email: "barbero@example.com", DisplayName: "Juan"})
	ana, _ := e.svc.Create(ctx, e.owner, e.biz, application.CreateInput{Email: "ana@example.com", DisplayName: "Ana"})
	barber := e.principal("barbero@example.com")

	if _, err := e.svc.AddPortfolioImage(ctx, barber, e.biz, juan.ID, photo(t, 10, "")); !errors.Is(err, application.ErrMediaUnavailable) {
		t.Errorf("without a store: %v", err)
	}
	e.svc.SetMedia(store)
	first, err := e.svc.AddPortfolioImage(ctx, barber, e.biz, juan.ID, photo(t, 2000, "  Degradado  "))
	if err != nil || first.Caption != "Degradado" || first.Width != 1600 || first.SortOrder != 0 || first.CreatedBy != barber.UserID || files(t, dir) != 1 {
		t.Fatalf("first: %+v %v", first, err)
	}
	// Another professional's portfolio: 403, read before any upload is read or stored.
	read := false
	spy := media.Upload{Read: func() ([]byte, error) { read = true; return nil, nil }, Field: func(string) string { return "" }}
	if _, err := e.svc.AddPortfolioImage(ctx, barber, e.biz, ana.ID, spy); !errors.Is(err, authz.ErrForbidden) || read {
		t.Errorf("barber on Ana's: %v (upload read: %v)", err, read)
	}
	if _, err := e.svc.AddPortfolioImage(ctx, barber, uuid.Must(uuid.NewV7()), juan.ID, spy); !errors.Is(err, authz.ErrNotFound) || read {
		t.Errorf("another business: %v", err)
	}
	if _, err := e.svc.AddPortfolioImage(ctx, e.owner, e.biz, uuid.Must(uuid.NewV7()), spy); !errors.Is(err, application.ErrNotFound) || read {
		t.Errorf("unknown professional: %v", err)
	}
	var verr *application.ValidationError
	if _, err := e.svc.AddPortfolioImage(ctx, barber, e.biz, juan.ID, photo(t, 10, string(make([]rune, 141)))); !errors.As(err, &verr) {
		t.Errorf("long caption: %v", err)
	}
	if _, err := e.svc.AddPortfolioImage(ctx, barber, e.biz, juan.ID, media.Bytes([]byte("%PDF-1.7"), nil)); !errors.Is(err, media.ErrUnsupported) {
		t.Errorf("pdf: %v", err)
	}
	for i := range 9 { // the OWNER fills it up to 10
		if _, err := e.svc.AddPortfolioImage(ctx, e.owner, e.biz, juan.ID, photo(t, 20+i, "")); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := e.svc.AddPortfolioImage(ctx, barber, e.biz, juan.ID, photo(t, 10, "")); !errors.Is(err, application.ErrPortfolioFull) || files(t, dir) != domain.MaxPortfolioImages {
		t.Errorf("11th: %v, %d files (the refused one must not stay)", err, files(t, dir))
	}
	list, err := e.svc.ListPortfolio(ctx, barber, e.biz, juan.ID)
	if err != nil || len(list) != 10 || list[0].ID != first.ID || list[9].SortOrder != 9 {
		t.Fatalf("list: %d %v", len(list), err)
	}
	// Caption and position; validation; another professional's image is not found.
	if img, err := e.svc.UpdatePortfolioImage(ctx, barber, e.biz, juan.ID, first.ID, application.PortfolioPatch{Caption: ptr("Fade"), SortOrder: ptr(50)}); err != nil || img.Caption != "Fade" || img.SortOrder != 50 {
		t.Errorf("update: %+v %v", img, err)
	}
	if _, err := e.svc.UpdatePortfolioImage(ctx, e.owner, e.biz, juan.ID, first.ID, application.PortfolioPatch{SortOrder: ptr(1001)}); !errors.As(err, &verr) {
		t.Errorf("sort order out of range: %v", err)
	}
	if _, err := e.svc.UpdatePortfolioImage(ctx, e.owner, e.biz, ana.ID, first.ID, application.PortfolioPatch{Caption: ptr("x")}); !errors.Is(err, application.ErrNotFound) {
		t.Errorf("image of another professional: %v", err)
	}
	if err := e.svc.DeletePortfolioImage(ctx, barber, e.biz, ana.ID, first.ID); !errors.Is(err, authz.ErrForbidden) {
		t.Errorf("barber deletes on Ana's: %v", err)
	}
	if err := e.svc.DeletePortfolioImage(ctx, barber, e.biz, juan.ID, first.ID); err != nil || files(t, dir) != 9 {
		t.Errorf("delete: %v, %d files", err, files(t, dir))
	}
	actions := e.store.AuditActions()
	if n := countOf(actions, application.AuditPortfolioAdded); n != 10 || countOf(actions, application.AuditPortfolioDeleted) != 1 {
		t.Errorf("audit: %v", actions)
	}
}

func countOf(list []string, want string) int {
	n := 0
	for _, s := range list {
		if s == want {
			n++
		}
	}
	return n
}
