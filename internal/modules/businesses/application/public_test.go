package application_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/application/biztest"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/domain"
	identity "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/clock"
)

type publicEnv struct {
	svc *application.Service
	ent *biztest.Entitlements
	ids map[string]uuid.UUID
}

// newPublicEnv creates self-service (ACTIVE) businesses named names in Cali; entitled lists the
// names whose demo is valid.
func newPublicEnv(t *testing.T, names []string, entitled ...string) publicEnv {
	t.Helper()
	e := publicEnv{ent: &biztest.Entitlements{IDs: map[uuid.UUID]bool{}}, ids: map[string]uuid.UUID{}}
	e.svc = application.NewService(application.Deps{
		Store: biztest.NewStore(), Authz: biztest.Authorizer{}, Members: &biztest.Members{}, Owners: &biztest.Owners{},
		Trials: &biztest.Trials{}, Summaries: biztest.Summaries{}, Entitlements: e.ent,
		Places: biztest.Places{cali: caliCity}, Clock: clock.NewFixed(time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)),
	})
	for _, n := range names {
		in := validBusiness()
		in.Name = n
		b, err := e.svc.CreateOwnBusiness(context.Background(), identity.Principal{UserID: uuid.Must(uuid.NewV7())}, in)
		if err != nil {
			t.Fatal(err)
		}
		e.ids[n] = b.ID()
	}
	for _, n := range entitled {
		e.ent.IDs[e.ids[n]] = true
	}
	return e
}

func names(bs []*domain.Business) string {
	out := ""
	for i, b := range bs {
		if i > 0 {
			out += ","
		}
		out += b.Name()
	}
	return out
}

func TestSearchPublicVisibilityAndOrder(t *testing.T) {
	ctx := context.Background()
	e := newPublicEnv(t, []string{"Zeta Barber", "alfa cortes", "Barbería Ñandú", "Vencida"}, "Zeta Barber", "alfa cortes", "Barbería Ñandú")
	items, next, err := e.svc.SearchPublic(ctx, application.SearchQuery{CityID: cali})
	if err != nil || next != "" {
		t.Fatalf("search: %v next=%q", err, next)
	}
	if got := names(items); got != "alfa cortes,Barbería Ñandú,Zeta Barber" {
		t.Errorf("order/visibility = %q (expired demo hidden, case-insensitive order)", got)
	}
	if items, _, _ := e.svc.SearchPublic(ctx, application.SearchQuery{CityID: cali, Text: "BARB"}); names(items) != "Barbería Ñandú,Zeta Barber" {
		t.Errorf("text filter = %q", names(items))
	}
	if items, _, _ := e.svc.SearchPublic(ctx, application.SearchQuery{CityID: uuid.Must(uuid.NewV7())}); len(items) != 0 {
		t.Errorf("other city: %d items", len(items))
	}
	// LIKE wildcards are literal text, not patterns.
	if items, _, _ := e.svc.SearchPublic(ctx, application.SearchQuery{CityID: cali, Text: "%_"}); len(items) != 0 {
		t.Errorf("wildcards matched %q", names(items))
	}
}

func TestSearchPublicPagination(t *testing.T) {
	ctx := context.Background()
	var all, visible []string
	for i := 0; i < 30; i++ {
		n := fmt.Sprintf("Barbería %02d", i)
		all = append(all, n)
		if i%3 != 0 { // a third of the demos expired
			visible = append(visible, n)
		}
	}
	e := newPublicEnv(t, all, visible...)
	var got []string
	cursor := ""
	for pages := 0; ; pages++ {
		if pages > 20 {
			t.Fatal("pagination does not end")
		}
		items, next, err := e.svc.SearchPublic(ctx, application.SearchQuery{CityID: cali, Cursor: cursor, Limit: 4})
		if err != nil {
			t.Fatal(err)
		}
		if len(items) > 4 {
			t.Fatalf("page of %d > limit", len(items))
		}
		for _, b := range items {
			got = append(got, b.Name())
		}
		if next == "" {
			break
		}
		cursor = next
	}
	if fmt.Sprint(got) != fmt.Sprint(visible) {
		t.Errorf("paged = %v\nwant    %v", got, visible)
	}
}

func TestSearchPublicValidation(t *testing.T) {
	ctx := context.Background()
	e := newPublicEnv(t, nil)
	for name, q := range map[string]application.SearchQuery{
		"no city":     {},
		"short text":  {CityID: cali, Text: "a"},
		"long text":   {CityID: cali, Text: string(make([]byte, 61))},
		"bad cursor":  {CityID: cali, Cursor: "!!!"},
		"forged":      {CityID: cali, Cursor: "bm9fc2VwYXJhdG9y"}, // "no_separator"
		"limit 51":    {CityID: cali, Limit: 51},
		"limit minus": {CityID: cali, Limit: -1},
	} {
		var verr *application.ValidationError
		if _, _, err := e.svc.SearchPublic(ctx, q); !errors.As(err, &verr) {
			t.Errorf("%s: %v, want ValidationError", name, err)
		}
	}
}

func TestGetPublic(t *testing.T) {
	ctx := context.Background()
	e := newPublicEnv(t, []string{"Barbería Visible", "Barbería Vencida"}, "Barbería Visible")
	b, err := e.svc.GetPublic(ctx, "BARBERIA-VISIBLE")
	if err != nil || b.Name() != "Barbería Visible" {
		t.Fatalf("visible: %v %v", b, err)
	}
	for _, slug := range []string{"barberia-vencida", "no-existe", "../etc", ""} {
		if _, err := e.svc.GetPublic(ctx, slug); !errors.Is(err, application.ErrNotFound) {
			t.Errorf("%q: %v, want ErrNotFound", slug, err)
		}
	}
	// A suspended business is hidden even with a valid demo.
	admin := identity.Principal{UserID: uuid.Must(uuid.NewV7()), IsPlatformAdmin: true}
	if _, err := e.svc.SuspendBusiness(ctx, admin, b.ID()); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.GetPublic(ctx, "barberia-visible"); !errors.Is(err, application.ErrNotFound) {
		t.Errorf("suspended: %v", err)
	}
	if items, _, _ := e.svc.SearchPublic(ctx, application.SearchQuery{CityID: cali}); len(items) != 0 {
		t.Errorf("suspended listed: %q", names(items))
	}
}
