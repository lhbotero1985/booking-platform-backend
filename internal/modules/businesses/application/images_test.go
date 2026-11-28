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

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/application/biztest"
	identity "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/authz"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/clock"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/media"
)

func pngUpload(t *testing.T, w, h int) media.Upload {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := range img.Pix {
		img.Pix[i] = 0xff
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return media.Bytes(b.Bytes(), nil)
}

func storedFiles(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			rel, _ := filepath.Rel(dir, p)
			out = append(out, filepath.ToSlash(rel))
		}
		return nil
	})
	return out
}

// TestBusinessImages: only the OWNER; replacing deletes the old file; a failed update leaves no
// file behind; the upload is not read before authorizing.
func TestBusinessImages(t *testing.T) {
	ctx := context.Background()
	e := newEnv()
	owner := identity.Principal{UserID: uuid.Must(uuid.NewV7())}
	b, err := e.svc.CreateOwnBusiness(ctx, owner, validBusiness())
	if err != nil {
		t.Fatal(err)
	}
	ghost := uuid.Must(uuid.NewV7()) // a membership in a business the store does not have
	barber := identity.Principal{UserID: uuid.Must(uuid.NewV7())}
	az := biztest.Authorizer{Memberships: map[uuid.UUID][]identity.Membership{
		owner.UserID:  {{BusinessID: b.ID(), Role: identity.RoleOwner}, {BusinessID: ghost, Role: identity.RoleOwner}},
		barber.UserID: {{BusinessID: b.ID(), Role: identity.RoleProfessional}},
	}}
	svc := application.NewService(application.Deps{Store: e.store, Authz: az, Members: e.members, Owners: e.owners, Trials: e.trials, Places: e.places,
		Clock: clock.NewFixed(b.CreatedAt())})
	if _, err := svc.SetImage(ctx, owner, b.ID(), application.ImageLogo, pngUpload(t, 10, 10)); !errors.Is(err, application.ErrMediaUnavailable) {
		t.Errorf("without a store: %v", err)
	}
	dir := t.TempDir()
	store, err := media.NewLocal(dir)
	if err != nil {
		t.Fatal(err)
	}
	svc.SetMedia(store)

	got, err := svc.SetImage(ctx, owner, b.ID(), application.ImageLogo, pngUpload(t, 1000, 500))
	first := got.Images().Logo
	if err != nil || first == "" || got.Images().Cover != "" || len(storedFiles(t, dir)) != 1 {
		t.Fatalf("logo: %+v %v", got.Images(), err)
	}
	got, _ = svc.SetImage(ctx, owner, b.ID(), application.ImageLogo, pngUpload(t, 20, 20))
	if files := storedFiles(t, dir); len(files) != 1 || files[0] != got.Images().Logo || got.Images().Logo == first {
		t.Errorf("replaced logo: %v", files)
	}
	read := false
	spy := media.Upload{Read: func() ([]byte, error) { read = true; return nil, nil }}
	if _, err := svc.SetImage(ctx, barber, b.ID(), application.ImageCover, spy); !errors.Is(err, authz.ErrForbidden) || read {
		t.Errorf("a PROFESSIONAL: %v (read %v)", err, read)
	}
	if _, err := svc.SetImage(ctx, identity.Principal{UserID: uuid.Must(uuid.NewV7())}, b.ID(), application.ImageCover, spy); !errors.Is(err, authz.ErrNotFound) || read {
		t.Errorf("an outsider: %v", err)
	}
	// The update fails after the file was stored: the file is removed.
	if _, err := svc.SetImage(ctx, owner, ghost, application.ImageCover, pngUpload(t, 10, 10)); !errors.Is(err, application.ErrNotFound) || len(storedFiles(t, dir)) != 1 {
		t.Errorf("failed update: %v, files %v", err, storedFiles(t, dir))
	}
	if _, err := svc.SetImage(ctx, owner, b.ID(), application.ImageCover, media.Bytes([]byte("not an image"), nil)); !errors.Is(err, media.ErrUnsupported) {
		t.Errorf("not an image: %v", err)
	}
	if got, err := svc.RemoveImage(ctx, owner, b.ID(), application.ImageLogo); err != nil || got.Images().Logo != "" || len(storedFiles(t, dir)) != 0 {
		t.Errorf("remove: %v %v", got.Images(), err)
	}
	if _, err := svc.RemoveImage(ctx, barber, b.ID(), application.ImageLogo); !errors.Is(err, authz.ErrForbidden) {
		t.Errorf("a PROFESSIONAL removes: %v", err)
	}
	if n := countActions(e.store.AuditActions(), application.AuditBusinessImageUpdated); n != 2 {
		t.Errorf("audit: %v", e.store.AuditActions())
	}
}

func countActions(list []string, want string) int {
	n := 0
	for _, s := range list {
		if s == want {
			n++
		}
	}
	return n
}
