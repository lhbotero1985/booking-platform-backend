package app

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	bizapp "github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/application/biztest"
	businesseshttp "github.com/lhbotero1985/booking-platform-backend/internal/modules/businesses/interfaces/http"
	catapp "github.com/lhbotero1985/booking-platform-backend/internal/modules/catalog/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/catalog/application/catalogtest"
	cataloghttp "github.com/lhbotero1985/booking-platform-backend/internal/modules/catalog/interfaces/http"
	geoapp "github.com/lhbotero1985/booking-platform-backend/internal/modules/geography/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/geography/application/geotest"
	geographyhttp "github.com/lhbotero1985/booking-platform-backend/internal/modules/geography/interfaces/http"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/application/apptest"
	identityhttp "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/interfaces/http"
	notifapp "github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/application/notiftest"
	notificationshttp "github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/interfaces/http"
	profapp "github.com/lhbotero1985/booking-platform-backend/internal/modules/professionals/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/professionals/application/proftest"
	professionalshttp "github.com/lhbotero1985/booking-platform-backend/internal/modules/professionals/interfaces/http"
	schedapp "github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/application/schedtest"
	schedulinghttp "github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/interfaces/http"
	subapp "github.com/lhbotero1985/booking-platform-backend/internal/modules/subscriptions/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/subscriptions/application/subtest"
	subscriptionshttp "github.com/lhbotero1985/booking-platform-backend/internal/modules/subscriptions/interfaces/http"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/clock"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/config"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/media"
)

type geoEnv struct {
	srv      *httptest.Server
	identity *application.Service
	users    *apptest.Store
	mailer   *apptest.Mailer
	geo      *geotest.Store
	biz      *biztest.Store
	subs     *subtest.Store
	modules  []RouteRegistrar // as wired in cmd/api
	media    *media.Local
	clock    *clock.Fixed
	notif    *notifapp.Service
	outbox   *notiftest.Store
}

// newGeoServer wires identity (authentication, authorizer) and geography like cmd/api does.
func newGeoServer(t *testing.T) geoEnv {
	t.Helper()
	cfg, err := config.Load(func(k string) string {
		return map[string]string{"APP_ENV": "test", "DATABASE_URL": "postgres://u:p@localhost:5432/test"}[k]
	})
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	clk := clock.NewFixed(time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC))
	e := geoEnv{users: apptest.NewStore(), mailer: &apptest.Mailer{}, geo: geotest.NewSeededStore(), clock: clk}
	e.identity = application.NewService(application.Deps{
		Store: e.users, Hasher: &apptest.Hasher{}, Mailer: e.mailer, Clock: clk, Random: &apptest.FixedRandom{},
		Config: application.Config{PublicAppURL: "https://app.example.test"}, Logger: logger,
	})
	idh := identityhttp.New(e.identity, logger, identityhttp.Options{})
	geo := geoapp.NewService(e.geo, e.identity, e.geo, clk)
	e.biz, e.subs = biztest.NewStore(), subtest.NewStore()
	subs := subapp.NewService(e.subs, e.identity, clk)
	biz := bizapp.NewService(bizapp.Deps{Store: e.biz, Authz: e.identity, Members: e.identity, Owners: e.identity, Trials: subs, Summaries: subs, Entitlements: subs, Places: geo, Clock: clk})
	profs := profapp.NewService(proftest.NewStore(), e.identity, clk)
	cat := catapp.NewService(catalogtest.NewStore(), e.identity, biz, profs, clk)
	LinkPublicCatalog(biz, cat, profs, biz)
	sched := schedapp.NewService(schedapp.Deps{Store: schedtest.NewStore(), Authz: e.identity, Businesses: biz, Catalog: cat, Professionals: profs, Bookable: profs, Customers: e.identity, Clock: clk,
		Schedules: profs, Currencies: biz, Memberships: e.identity})
	LinkAgendaGuard(profs, sched)
	if e.media, err = media.NewLocal(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	biz.SetMedia(e.media)
	profs.SetMedia(e.media)
	notif, outbox := notiftest.NewService(sched, biz, clk, func(d *notifapp.Deps) {
		d.Authz, d.Recipients = e.identity, NewEmailRecipients(e.identity, profs)
	})
	e.notif, e.outbox = notif, outbox
	LinkNotifications(sched, notif)
	e.modules = []RouteRegistrar{
		idh, geographyhttp.New(geo, logger, idh.RequireAuth), businesseshttp.New(biz, logger, idh.RequireAuth, nil),
		subscriptionshttp.New(subs, logger, idh.RequireAuth),
		professionalshttp.New(profs, logger, idh.RequireAuth),
		cataloghttp.New(cat, logger, idh.RequireAuth),
		schedulinghttp.New(sched, logger, idh.RequireAuth),
		notificationshttp.New(notif, logger, idh.RequireAuth),
		notificationshttp.NewWebhook(notif, notificationshttp.WebhookConfig{AppSecret: testWebhookSecret, VerifyToken: testWebhookVerifyToken}, logger),
		notificationshttp.NewLinks(notif, notificationshttp.LinksConfig{}, nil, logger),
	}
	e.srv = httptest.NewServer(NewHTTPHandler(cfg, logger, Deps{Modules: e.modules, Media: e.media.Handler()}))
	t.Cleanup(e.srv.Close)
	return e
}

// signIn registers, verifies and logs in email, returning a bearer token.
func (e geoEnv) signIn(t *testing.T, email string) string {
	t.Helper()
	ctx := context.Background()
	pw := "una clave suficientemente larga"
	if err := e.identity.Register(ctx, application.RegisterInput{Email: email, Password: pw, FullName: "QA", Phone: "3001234567"}); err != nil {
		t.Fatal(err)
	}
	if err := e.identity.SendVerificationEmail(ctx, e.users.Enqueued[len(e.users.Enqueued)-1]); err != nil {
		t.Fatal(err)
	}
	raw, _ := url.QueryUnescape(regexp.MustCompile(`token=([^\s]+)`).FindStringSubmatch(e.mailer.Last().Body)[1])
	if err := e.identity.VerifyEmail(ctx, raw); err != nil {
		t.Fatal(err)
	}
	res, err := e.identity.Login(ctx, application.LoginInput{Email: email, Password: pw, ClientType: "MOBILE"})
	if err != nil {
		t.Fatal(err)
	}
	return res.Token.Reveal()
}

func TestContractGeographyEndpoints(t *testing.T) {
	c := loadContract(t)
	e := newGeoServer(t)
	userTok := e.signIn(t, "ana@example.com")
	adminTok := e.signIn(t, "admin@example.com")
	if err := e.identity.SetPlatformAdmin(context.Background(), "admin@example.com", true); err != nil {
		t.Fatal(err)
	}
	valle, cali := geotest.ValleID.String(), geotest.CaliID.String()

	// Public reads.
	c.callAuth(t, e.srv, http.MethodGet, "/api/v1/countries", "", "", http.StatusOK)
	c.callAuth(t, e.srv, http.MethodGet, "/api/v1/countries/CO/divisions", "", "", http.StatusOK)
	c.callAuth(t, e.srv, http.MethodGet, "/api/v1/countries/ZZ/divisions", "", "", http.StatusNotFound)
	c.callAuth(t, e.srv, http.MethodGet, "/api/v1/divisions/"+valle+"/cities", "", "", http.StatusOK)
	c.callAuth(t, e.srv, http.MethodGet, "/api/v1/divisions/not-a-uuid/cities", "", "", http.StatusNotFound)

	// Admin writes.
	c.callAuth(t, e.srv, http.MethodPost, "/api/v1/admin/countries", `{"code":"US","name":"Estados Unidos","phoneCode":"+1","defaultCurrency":"USD"}`, adminTok, http.StatusCreated)
	c.callAuth(t, e.srv, http.MethodPost, "/api/v1/admin/countries", `{"code":"US","name":"Estados Unidos","phoneCode":"+1","defaultCurrency":"USD"}`, adminTok, http.StatusConflict)
	c.callAuth(t, e.srv, http.MethodPost, "/api/v1/admin/countries", `{"code":"X","name":"","phoneCode":"1","defaultCurrency":"EUR"}`, adminTok, http.StatusBadRequest)
	c.callAuth(t, e.srv, http.MethodPatch, "/api/v1/admin/countries/US", `{"active":false}`, adminTok, http.StatusOK)
	c.callAuth(t, e.srv, http.MethodPatch, "/api/v1/admin/countries/PE", `{"active":false}`, adminTok, http.StatusNotFound)

	body := c.callAuth(t, e.srv, http.MethodPost, "/api/v1/admin/divisions", `{"countryCode":"CO","code":"CO-CAU","name":"Cauca","type":"DEPARTMENT"}`, adminTok, http.StatusCreated)
	div := regexp.MustCompile(`"id":"([^"]+)"`).FindStringSubmatch(body)[1]
	c.callAuth(t, e.srv, http.MethodPatch, "/api/v1/admin/divisions/"+div, `{"code":""}`, adminTok, http.StatusOK)
	c.callAuth(t, e.srv, http.MethodPatch, "/api/v1/admin/divisions/"+div, `{"name":"Antioquia"}`, adminTok, http.StatusConflict)

	body = c.callAuth(t, e.srv, http.MethodPost, "/api/v1/admin/cities", `{"divisionId":"`+div+`","name":"Popayán","timeZone":"America/Bogota"}`, adminTok, http.StatusCreated)
	if !strings.Contains(body, `"timeZone":"America/Bogota"`) {
		t.Errorf("city body = %s", body)
	}
	c.callAuth(t, e.srv, http.MethodPatch, "/api/v1/admin/cities/"+cali, `{"timeZone":"Mars/Olympus"}`, adminTok, http.StatusBadRequest)
	c.callAuth(t, e.srv, http.MethodPatch, "/api/v1/admin/cities/"+cali, `{"name":"Santiago de Cali"}`, adminTok, http.StatusOK)
	e.geo.ActiveBusinesses[cali] = 1
	e.geo.ActiveBusinesses["CO"] = 1
	if body := c.callAuth(t, e.srv, http.MethodPatch, "/api/v1/admin/cities/"+cali, `{"active":false}`, adminTok, http.StatusConflict); !strings.Contains(body, `"IN_USE"`) {
		t.Errorf("city in use: %s", body)
	}
	c.callAuth(t, e.srv, http.MethodPatch, "/api/v1/admin/countries/CO", `{"active":false}`, adminTok, http.StatusConflict)

	// Authentication and authorization.
	c.callAuth(t, e.srv, http.MethodPost, "/api/v1/admin/cities", `{"divisionId":"`+valle+`","name":"Palmira","timeZone":"America/Bogota"}`, "", http.StatusUnauthorized)
	c.callAuth(t, e.srv, http.MethodPost, "/api/v1/admin/cities", `{"divisionId":"`+valle+`","name":"Palmira","timeZone":"America/Bogota"}`, userTok, http.StatusForbidden)
	c.callAuth(t, e.srv, http.MethodPatch, "/api/v1/admin/countries/CO", `{"active":false}`, userTok, http.StatusForbidden)

	if got := len(e.geo.Audit); got != 6 {
		t.Errorf("geography audit entries = %d (%v), want 6 successful writes", got, e.geo.AuditActions())
	}
	if n := len(e.users.AuditActions()); n == 0 || e.users.AuditActions()[n-1] != application.AuditAuthzDenied+":DENIED" {
		t.Errorf("denied admin attempts must be audited by identity: %v", e.users.AuditActions())
	}
}

// Webhook secrets of the test servers (built, not literal: they are not credentials).
var (
	testWebhookSecret      = strings.Repeat("ab", 16)
	testWebhookVerifyToken = strings.Repeat("verify", 4)
)
