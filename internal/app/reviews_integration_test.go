//go:build integration

package app

import (
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	identitycontract "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	notifdomain "github.com/lhbotero1985/booking-platform-backend/internal/modules/notifications/domain"
	schedapp "github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/application"
	scheddomain "github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/domain"
	schedpg "github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/infrastructure/postgres"
)

// TestReviewsOnPostgres (2026-11-28): the reviews through the real SQL: one review per
// appointment under 40 simultaneous submissions, every read, the ratings, the reply, the hiding,
// the constraints, and the review request queued when the appointment is completed.
func TestReviewsOnPostgres(t *testing.T) {
	ctx := t.Context()
	w := newNotifWorld(t)
	ana := w.customer(t)
	a, err := w.sched.Book(ctx, ana, w.in(w.at(9, 0), "resena-postgres-0900"))
	if err != nil {
		t.Fatal(err)
	}
	w.clk.Advance(w.at(9, 5).Sub(w.clk.Now()))
	for _, act := range []schedapp.Action{schedapp.ActionStart, schedapp.ActionComplete} {
		if _, err := w.sched.StaffChange(ctx, w.owner, w.biz, a.ID, act, ""); err != nil {
			t.Fatal(err)
		}
	}
	asked := 0
	for _, m := range w.messages(t, a.ID) {
		if m.Kind == notifdomain.KindReviewRequest && m.Channel == notifdomain.ChannelEmail && m.RecipientRole == notifdomain.RecipientCustomer {
			asked++
		}
	}
	if asked != 1 {
		t.Errorf("review requests = %d, want 1", asked)
	}

	// 40 submissions at once: one review, created once, the others change it (the appointment's
	// lock serializes them; without it two would both insert and one would fail).
	// Open every connection of the pool first, so the submissions really overlap.
	var conns []*pgxpool.Conn
	for range w.pool.Config().MaxConns {
		c, err := w.pool.Acquire(ctx)
		if err != nil {
			t.Fatal(err)
		}
		conns = append(conns, c)
	}
	for _, c := range conns {
		c.Release()
	}
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		errs    []error
		creates int
		gate    = make(chan struct{})
	)
	for i := range 40 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-gate
			_, created, err := w.sched.SubmitReview(ctx, ana, a.ID, schedapp.ReviewInput{BusinessRating: 1 + i%5, ProfessionalRating: 5, Comment: "Muy bien"})
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs = append(errs, err)
			}
			if created {
				creates++
			}
		}()
	}
	close(gate)
	wg.Wait()
	if len(errs) > 0 || creates != 1 {
		t.Fatalf("submissions: %d created, errors %v", creates, errs)
	}
	if n := count(t, w.pool, `SELECT count(*) FROM appointment_reviews WHERE appointment_id = $1`, a.ID); n != 1 {
		t.Fatalf("reviews of the appointment = %d", n)
	}

	r, _, err := w.sched.SubmitReview(ctx, ana, a.ID, schedapp.ReviewInput{BusinessRating: 4, ProfessionalRating: 5, Comment: "  Excelente  "})
	if err != nil || r.Comment != "Excelente" || r.ServiceName != "Corte" || r.ProfessionalID != w.pro {
		t.Fatalf("review: %+v %v", r, err)
	}
	if mine, err := w.sched.MyReview(ctx, ana, a.ID); err != nil || mine.ID != r.ID || mine.BusinessRating != 4 {
		t.Errorf("my review: %+v %v", mine, err)
	}
	if list, _, err := w.sched.MyAppointments(ctx, ana, schedapp.MyAppointmentsQuery{}); err != nil || len(list) != 1 || list[0].Review == nil || list[0].ReviewableUntil.IsZero() {
		t.Errorf("my appointments: %+v %v", list, err)
	}
	pub, err := w.sched.PublicReviews(ctx, w.slug, schedapp.ReviewsQuery{ProfessionalID: w.pro.String()})
	if err != nil || len(pub.Items) != 1 || pub.Rating.Count != 1 || *pub.Rating.Average() != 5 || pub.Names[w.pro] != "Juan" {
		t.Errorf("public of Juan: %+v %v", pub, err)
	}
	if rs, err := w.sched.BusinessRatings(ctx, []uuid.UUID{w.biz, uuid.Must(uuid.NewV7())}); err != nil || len(rs) != 1 || *rs[w.biz].Average != 4 {
		t.Errorf("business ratings: %+v %v", rs, err)
	}
	if staff, err := w.sched.BusinessReviews(ctx, w.barber, w.biz, schedapp.ReviewsQuery{}); err != nil || len(staff.Items) != 1 {
		t.Errorf("barber's reviews: %+v %v", staff, err)
	}
	if rep, err := w.sched.ReplyToReview(ctx, w.owner, w.biz, r.ID, "Gracias"); err != nil || rep.Reply != "Gracias" {
		t.Errorf("reply: %+v %v", rep, err)
	}
	if rep, err := w.sched.RemoveReviewReply(ctx, w.owner, w.biz, r.ID); err != nil || rep.Reply != "" {
		t.Errorf("remove reply: %+v %v", rep, err)
	}
	if n := count(t, w.pool, `SELECT count(*) FROM appointment_reviews WHERE id = $1 AND reply = '' AND replied_at IS NULL AND replied_by IS NULL`, r.ID); n != 1 {
		t.Error("reply not removed in the row")
	}

	// Hiding (an administrator): out of the public list and the ratings.
	admins := schedapp.NewService(schedapp.Deps{Store: schedpg.NewStore(w.pool), Authz: w.identity, Clock: w.clk, Admins: w.identity})
	adminID, _ := activeUser(t, w.pool, "ACTIVE")
	admin := identitycontract.Principal{UserID: adminID, IsPlatformAdmin: true}
	if _, err := admins.SetReviewHidden(ctx, admin, r.ID, true, "Ofensiva"); err != nil {
		t.Fatal(err)
	}
	if p, _ := w.sched.PublicReviews(ctx, w.slug, schedapp.ReviewsQuery{}); len(p.Items) != 0 || p.Rating.Count != 0 {
		t.Errorf("public after hiding: %+v", p)
	}
	if p, _ := w.sched.BusinessReviews(ctx, w.owner, w.biz, schedapp.ReviewsQuery{}); len(p.Items) != 1 || !p.Items[0].Hidden() || p.Items[0].HiddenBy != adminID {
		t.Errorf("owner after hiding: %+v", p)
	}
	if _, err := admins.SetReviewHidden(ctx, admin, r.ID, false, ""); err != nil {
		t.Fatal(err)
	}
	if n := count(t, w.pool, `SELECT count(*) FROM appointment_reviews WHERE id = $1 AND hidden_at IS NULL AND hidden_by IS NULL AND hidden_reason = ''`, r.ID); n != 1 {
		t.Error("not shown again in the row")
	}

	// The constraints are the safety net.
	for name, q := range map[string]string{
		"6 stars":                `UPDATE appointment_reviews SET business_rating = 6 WHERE id = $1`,
		"a reply without a date": `UPDATE appointment_reviews SET reply = 'x' WHERE id = $1`,
		"hidden without who":     `UPDATE appointment_reviews SET hidden_at = now() WHERE id = $1`,
		"a 1001-char comment":    `UPDATE appointment_reviews SET comment = repeat('a', 1001) WHERE id = $1`,
	} {
		if _, err := w.pool.Exec(ctx, q, r.ID); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if _, err := w.pool.Exec(ctx, `INSERT INTO appointment_reviews (id, business_id, appointment_id, professional_id, customer_user_id, author_name, service_name,
		business_rating, professional_rating, created_at, updated_at) SELECT $1, business_id, appointment_id, professional_id, customer_user_id, author_name,
		service_name, 5, 5, now(), now() FROM appointment_reviews WHERE id = $2`, uuid.Must(uuid.NewV7()), r.ID); err == nil {
		t.Error("a second review of the appointment accepted")
	}

	if err := w.sched.DeleteMyReview(ctx, ana, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := w.sched.MyReview(ctx, ana, a.ID); !errors.Is(err, schedapp.ErrReviewNotFound) {
		t.Errorf("after deleting: %v", err)
	}
	// A no-show cannot be reviewed.
	b, err := w.sched.Book(ctx, ana, w.in(w.at(11, 0), "resena-postgres-1100"))
	if err != nil {
		t.Fatal(err)
	}
	w.clk.Advance(w.at(11, 5).Sub(w.clk.Now()))
	if _, err := w.sched.StaffChange(ctx, w.owner, w.biz, b.ID, schedapp.ActionNoShow, ""); err != nil {
		t.Fatal(err)
	}
	if _, _, err := w.sched.SubmitReview(ctx, ana, b.ID, schedapp.ReviewInput{BusinessRating: 1, ProfessionalRating: 1}); !errors.Is(err, scheddomain.ErrNotReviewable) {
		t.Errorf("a no-show: %v", err)
	}
}
