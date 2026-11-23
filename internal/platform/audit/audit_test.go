package audit

import (
	"context"
	"net/netip"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/lhbotero1985/booking-platform-backend/internal/platform/reqctx"
)

func TestNewTakesRequestContextAndBuilders(t *testing.T) {
	ip := netip.MustParseAddr("181.54.0.95")
	ctx := reqctx.WithClientIP(reqctx.WithRequestID(context.Background(), "req-1"), ip)
	now := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	actor, biz := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())

	e := New(ctx, now, "geography.city_created", Success).
		Actor(actor).Business(biz).Resource("city", "c-1").Meta("fields", "name")

	if e.ID.Version() != 7 || !e.OccurredAt.Equal(now) || e.Action != "geography.city_created" || e.Outcome != Success {
		t.Errorf("entry = %+v", e)
	}
	if e.IP == nil || *e.IP != ip || e.Metadata["request_id"] != "req-1" {
		t.Errorf("request context not captured: ip=%v meta=%v", e.IP, e.Metadata)
	}
	if *e.ActorUserID != actor || *e.BusinessID != biz || e.ResourceType != "city" || e.ResourceID != "c-1" || e.Metadata["fields"] != "name" {
		t.Errorf("builders: %+v", e)
	}

	// Nil IDs and an empty context leave the optional fields unset.
	e = New(context.Background(), now, "x", Denied).Actor(uuid.Nil).Business(uuid.Nil)
	if e.ActorUserID != nil || e.BusinessID != nil || e.IP != nil || len(e.Metadata) != 0 {
		t.Errorf("optional fields set: %+v", e)
	}
	if optional("") != nil || *optional("a") != "a" {
		t.Error("optional")
	}
}
