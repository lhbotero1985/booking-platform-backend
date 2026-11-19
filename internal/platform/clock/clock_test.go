package clock

import (
	"testing"
	"time"
)

func TestFixedAndSystem(t *testing.T) {
	start := time.Date(2026, 11, 19, 8, 0, 0, 0, time.FixedZone("COT", -5*3600))
	c := NewFixed(start)
	if !c.Now().Equal(start) || c.Now().Location() != time.UTC {
		t.Fatalf("Fixed.Now() = %v", c.Now())
	}
	c.Advance(90 * time.Minute)
	if got := c.Now().Sub(start); got != 90*time.Minute {
		t.Fatalf("Advance: %v", got)
	}
	var s Clock = System{}
	if s.Now().Location() != time.UTC || time.Since(s.Now()) > time.Second {
		t.Fatalf("System.Now() = %v", s.Now())
	}
}
