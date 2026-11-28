package application_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	identity "github.com/lhbotero1985/booking-platform-backend/internal/modules/identity/contract"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/application"
	"github.com/lhbotero1985/booking-platform-backend/internal/modules/scheduling/domain"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/authz"
)

func fieldCodes(err error) map[string]string {
	var verr *application.ValidationError
	out := map[string]string{}
	if errors.As(err, &verr) {
		for _, f := range verr.Fields {
			out[f.Field] = f.Code
		}
	}
	return out
}

// TestReviews: only the customer of a COMPLETED appointment, up to 30 days after it; one review
// that it may change and delete; the OWNER replies; the staff read them (a PROFESSIONAL its own);
// only a platform administrator hides one, and a hidden one leaves the public list and ratings.
func TestReviews(t *testing.T) {
	ctx := context.Background()
	e := newChangeEnv(t)
	customer := e.bookingEnv.ana
	a, err := e.svc.Book(ctx, customer, e.in(e.day(9, 0), "resena-cliente-1"))
	if err != nil {
		t.Fatal(err)
	}
	good := application.ReviewInput{BusinessRating: 5, ProfessionalRating: 4, Comment: "  Excelente corte  "}
	if _, _, err := e.svc.SubmitReview(ctx, customer, a.ID, good); !errors.Is(err, domain.ErrNotReviewable) {
		t.Errorf("before it is completed: %v", err)
	}
	e.at(t, e.day(9, 5))
	if _, err := e.svc.StaffChange(ctx, e.juan, e.biz, a.ID, application.ActionStart, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.StaffChange(ctx, e.juan, e.biz, a.ID, application.ActionComplete, ""); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.svc.SubmitReview(ctx, e.bob, a.ID, good); !errors.Is(err, application.ErrAppointmentNotFound) {
		t.Errorf("another customer: %v", err)
	}
	if _, _, err := e.svc.SubmitReview(ctx, identity.Principal{}, a.ID, good); !errors.Is(err, application.ErrAppointmentNotFound) {
		t.Errorf("no account: %v", err)
	}
	// Another customer's invalid input is still a 404: the appointment is checked first.
	if _, _, err := e.svc.SubmitReview(ctx, e.bob, a.ID, application.ReviewInput{}); !errors.Is(err, application.ErrAppointmentNotFound) {
		t.Errorf("another customer, invalid input: %v", err)
	}
	codes := fieldCodes(func() error {
		_, _, err := e.svc.SubmitReview(ctx, customer, a.ID, application.ReviewInput{BusinessRating: 0, ProfessionalRating: 6, Comment: strings.Repeat("a", 1001)})
		return err
	}())
	if codes["businessRating"] != application.CodeInvalidRating || codes["professionalRating"] != application.CodeInvalidRating || codes["comment"] != application.CodeTooLong {
		t.Errorf("invalid input: %v", codes)
	}

	r, created, err := e.svc.SubmitReview(ctx, customer, a.ID, good)
	if err != nil || !created || r.Comment != "Excelente corte" || r.AuthorName != "Ana C." || r.ProfessionalID != e.pro ||
		r.ServiceName != "Corte" || r.BusinessRating != 5 || r.ProfessionalRating != 4 {
		t.Fatalf("review: %+v %v %v", r, created, err)
	}
	r2, created, err := e.svc.SubmitReview(ctx, customer, a.ID, application.ReviewInput{BusinessRating: 4, ProfessionalRating: 3})
	if err != nil || created || r2.ID != r.ID || r2.Comment != "" || r2.BusinessRating != 4 || len(e.store.Reviews) != 1 {
		t.Errorf("changed review: %+v %v %v", r2, created, err)
	}
	mine, err := e.svc.MyAppointmentDetail(ctx, customer, a.ID)
	if err != nil || mine.Review == nil || mine.Review.ID != r.ID || !mine.ReviewableUntil.Equal(a.End.Add(30*24*time.Hour)) {
		t.Errorf("my appointment: %+v %v", mine.Review, err)
	}

	// Public: the visible ones and the rating; of one professional, that professional's.
	pub, err := e.svc.PublicReviews(ctx, "barberia", application.ReviewsQuery{})
	if err != nil || len(pub.Items) != 1 || pub.Rating.Count != 1 || *pub.Rating.Average() != 4 || pub.Names[e.pro] != "Juan" {
		t.Fatalf("public: %+v %v", pub, err)
	}
	if p, err := e.svc.PublicReviews(ctx, "barberia", application.ReviewsQuery{ProfessionalID: e.pro.String()}); err != nil || *p.Rating.Average() != 3 {
		t.Errorf("public of Juan: %+v %v", p.Rating, err)
	}
	if p, err := e.svc.PublicReviews(ctx, "barberia", application.ReviewsQuery{ProfessionalID: e.anaPro.String()}); err != nil || len(p.Items) != 0 || p.Rating.Average() != nil {
		t.Errorf("public of Ana: %+v %v", p, err)
	}
	if _, err := e.svc.PublicReviews(ctx, "barberia", application.ReviewsQuery{Limit: "51", Cursor: "@@", ProfessionalID: "x"}); len(fieldCodes(err)) != 3 {
		t.Errorf("invalid query: %v", err)
	}

	// The OWNER replies; nobody else does.
	if _, err := e.svc.ReplyToReview(ctx, e.juan, e.biz, r.ID, "Gracias"); !errors.Is(err, authz.ErrForbidden) {
		t.Errorf("a professional replies: %v", err)
	}
	if _, err := e.svc.ReplyToReview(ctx, e.bob, e.biz, r.ID, ""); !errors.Is(err, authz.ErrNotFound) {
		t.Errorf("an outsider replies (invalid input): %v", err)
	}
	if _, err := e.svc.ReplyToReview(ctx, e.owner, e.biz, r.ID, "   "); fieldCodes(err)["reply"] != application.CodeRequired {
		t.Errorf("empty reply: %v", err)
	}
	if _, err := e.svc.ReplyToReview(ctx, e.owner, e.biz, uuid.Must(uuid.NewV7()), "Gracias"); !errors.Is(err, application.ErrReviewNotFound) {
		t.Errorf("unknown review: %v", err)
	}
	rep, err := e.svc.ReplyToReview(ctx, e.owner, e.biz, r.ID, " ¡Gracias, Ana! ")
	if err != nil || rep.Reply != "¡Gracias, Ana!" || rep.RepliedBy != e.owner.UserID || rep.RepliedAt.IsZero() {
		t.Errorf("reply: %+v %v", rep, err)
	}
	if again, _, _ := e.svc.SubmitReview(ctx, customer, a.ID, good); again.Reply != "¡Gracias, Ana!" {
		t.Errorf("changing the review keeps the reply: %+v", again)
	}

	// Staff: Juan reads his, Ana (another professional) none and not Juan's.
	if p, err := e.svc.BusinessReviews(ctx, e.juan, e.biz, application.ReviewsQuery{}); err != nil || len(p.Items) != 1 {
		t.Errorf("Juan: %+v %v", p, err)
	}
	if p, err := e.svc.BusinessReviews(ctx, e.ana, e.biz, application.ReviewsQuery{}); err != nil || len(p.Items) != 0 {
		t.Errorf("Ana: %+v %v", p, err)
	}
	if _, err := e.svc.BusinessReviews(ctx, e.ana, e.biz, application.ReviewsQuery{ProfessionalID: e.pro.String()}); !errors.Is(err, authz.ErrForbidden) {
		t.Errorf("Ana reads Juan's: %v", err)
	}
	if _, err := e.svc.BusinessReviews(ctx, e.bob, e.biz, application.ReviewsQuery{Limit: "0x"}); !errors.Is(err, authz.ErrNotFound) {
		t.Errorf("an outsider (invalid query): %v", err)
	}

	// Hiding: platform administrators only, with a reason.
	admin := identity.Principal{UserID: uuid.Must(uuid.NewV7()), IsPlatformAdmin: true}
	if _, err := e.svc.SetReviewHidden(ctx, e.owner, r.ID, true, "ofensiva"); !errors.Is(err, authz.ErrForbidden) {
		t.Errorf("the owner hides: %v", err)
	}
	if _, err := e.svc.SetReviewHidden(ctx, admin, r.ID, true, " "); fieldCodes(err)["reason"] != application.CodeRequired {
		t.Errorf("no reason: %v", err)
	}
	h, err := e.svc.SetReviewHidden(ctx, admin, r.ID, true, "Lenguaje ofensivo")
	if err != nil || !h.Hidden() || h.HiddenBy != admin.UserID || h.HiddenReason != "Lenguaje ofensivo" {
		t.Fatalf("hide: %+v %v", h, err)
	}
	if p, _ := e.svc.PublicReviews(ctx, "barberia", application.ReviewsQuery{}); len(p.Items) != 0 || p.Rating.Count != 0 {
		t.Errorf("public after hiding: %+v", p)
	}
	if rs, _ := e.svc.BusinessRatings(ctx, []uuid.UUID{e.biz}); len(rs) != 0 {
		t.Errorf("ratings after hiding: %+v", rs)
	}
	if p, _ := e.svc.BusinessReviews(ctx, e.owner, e.biz, application.ReviewsQuery{}); len(p.Items) != 1 || !p.Items[0].Hidden() {
		t.Errorf("the owner still sees it, marked: %+v", p)
	}
	if mr, err := e.svc.MyReview(ctx, customer, a.ID); err != nil || !mr.Hidden() {
		t.Errorf("the customer sees it hidden: %+v %v", mr, err)
	}
	if u, err := e.svc.SetReviewHidden(ctx, admin, r.ID, false, ""); err != nil || u.Hidden() || u.HiddenReason != "" {
		t.Errorf("unhide: %+v %v", u, err)
	}
	if rs, _ := e.svc.BusinessRatings(ctx, []uuid.UUID{e.biz}); rs[e.biz].Count != 1 || *rs[e.biz].Average != 5 {
		t.Errorf("ratings: %+v", rs)
	}
	if rs, _ := e.svc.ProfessionalRatings(ctx, e.biz); rs[e.pro].Count != 1 || *rs[e.pro].Average != 4 {
		t.Errorf("professional ratings: %+v", rs)
	}

	// 30 days after the end: no more changes; deleting is always possible.
	e.at(t, a.End.Add(30*24*time.Hour+time.Minute))
	if _, _, err := e.svc.SubmitReview(ctx, customer, a.ID, good); !errors.Is(err, domain.ErrReviewWindowClosed) {
		t.Errorf("after 30 days: %v", err)
	}
	if err := e.svc.DeleteMyReview(ctx, e.bob, a.ID); !errors.Is(err, application.ErrAppointmentNotFound) {
		t.Errorf("another customer deletes: %v", err)
	}
	if err := e.svc.DeleteMyReview(ctx, customer, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.MyReview(ctx, customer, a.ID); !errors.Is(err, application.ErrReviewNotFound) {
		t.Errorf("after deleting: %v", err)
	}
	if err := e.svc.DeleteMyReview(ctx, customer, a.ID); !errors.Is(err, application.ErrReviewNotFound) {
		t.Errorf("deleting twice: %v", err)
	}
}

// TestReviewPages: newest first, pages by cursor.
func TestReviewPages(t *testing.T) {
	ctx := context.Background()
	e := newChangeEnv(t)
	customers := []identity.Principal{e.bookingEnv.ana, e.bob, e.bookingEnv.ana}
	var appts []domain.Appointment
	for i, h := range []int{9, 10, 11} {
		a, err := e.svc.Book(ctx, customers[i], e.in(e.day(h, 0), "resena-pagina-"+string(rune('a'+i))+"-0001"))
		if err != nil {
			t.Fatal(err)
		}
		appts = append(appts, a)
	}
	var ids []uuid.UUID
	for i, a := range appts {
		e.at(t, a.Start.Add(5*time.Minute))
		for _, act := range []application.Action{application.ActionStart, application.ActionComplete} {
			if _, err := e.svc.StaffChange(ctx, e.juan, e.biz, a.ID, act, ""); err != nil {
				t.Fatal(err)
			}
		}
		r, _, err := e.svc.SubmitReview(ctx, customers[i], a.ID, application.ReviewInput{BusinessRating: 3 + i, ProfessionalRating: 5})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, r.ID)
	}
	p1, err := e.svc.PublicReviews(ctx, "barberia", application.ReviewsQuery{Limit: "2"})
	if err != nil || len(p1.Items) != 2 || p1.Items[0].ID != ids[2] || p1.Items[1].ID != ids[1] || p1.NextCursor == "" ||
		p1.Rating.Count != 3 || *p1.Rating.Average() != 4 {
		t.Fatalf("page 1: %+v %v", p1, err)
	}
	p2, err := e.svc.PublicReviews(ctx, "barberia", application.ReviewsQuery{Limit: "2", Cursor: p1.NextCursor})
	if err != nil || len(p2.Items) != 1 || p2.Items[0].ID != ids[0] || p2.NextCursor != "" {
		t.Errorf("page 2: %+v %v", p2, err)
	}
	// Ana's two appointments each carry their review in her list.
	past, _, err := e.svc.MyAppointments(ctx, e.bookingEnv.ana, application.MyAppointmentsQuery{Upcoming: false})
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, m := range past {
		if m.Review != nil && m.Review.AppointmentID == m.ID {
			n++
		}
	}
	if n != 2 {
		t.Errorf("reviews in Ana's appointments: %d", n)
	}
}
