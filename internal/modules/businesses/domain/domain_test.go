package domain

import (
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestParseTexts(t *testing.T) {
	if v, err := ParseName("  Barbería El Paisa  "); err != nil || v != "Barbería El Paisa" {
		t.Errorf("ParseName = %q, %v", v, err)
	}
	for _, in := range []string{"", "A", strings.Repeat("a", MaxNameLength+1), "Bar\nbería", "Bar\x00"} {
		if _, err := ParseName(in); !errors.Is(err, ErrInvalidName) {
			t.Errorf("ParseName(%q) err = %v", in, err)
		}
	}
	if v, err := ParseDescription("  "); err != nil || v != "" {
		t.Errorf("empty description: %q %v", v, err)
	}
	if v, err := ParseDescription("Cortes clásicos.\nAtención con cita."); err != nil || !strings.Contains(v, "\n") {
		t.Errorf("description with line break: %q %v", v, err)
	}
	for _, in := range []string{strings.Repeat("a", MaxDescriptionLength+1), "x\x07y"} {
		if _, err := ParseDescription(in); !errors.Is(err, ErrInvalidDescription) {
			t.Errorf("ParseDescription(%q…) err = %v", in[:3], err)
		}
	}
	if _, err := ParseAddress("Calle 5 # 38-25, San Fernando"); err != nil {
		t.Error(err)
	}
	for _, in := range []string{"", strings.Repeat("a", MaxAddressLength+1), "Calle\n5"} {
		if _, err := ParseAddress(in); !errors.Is(err, ErrInvalidAddress) {
			t.Errorf("ParseAddress(%q) err = %v", in, err)
		}
	}
}

func TestSlugs(t *testing.T) {
	for _, in := range []string{"abc", "barberia-el-paisa", "a1b", strings.Repeat("a", 60)} {
		if _, err := ParseSlug(in); err != nil {
			t.Errorf("ParseSlug(%q): %v", in, err)
		}
	}
	for _, in := range []string{"ab", "-abc", "abc-", "a--b", "ABC", "a_b", "añb", strings.Repeat("a", 61)} {
		if _, err := ParseSlug(in); !errors.Is(err, ErrInvalidSlug) {
			t.Errorf("ParseSlug(%q) err = %v", in, err)
		}
	}
	if _, err := ParseSlug("admin"); !errors.Is(err, ErrReservedSlug) {
		t.Errorf("reserved slug err = %v", err)
	}

	for in, want := range map[string]Slug{
		"Barbería El Paisa":         "barberia-el-paisa",
		"  Peluquería Ñandú & Co. ": "peluqueria-nandu-co",
		"Uñas___Spa!!!  Cali":       "unas-spa-cali",
		"Admin":                     "negocio-admin",
		"AB":                        "negocio-ab",
		"¡!":                        "mi-negocio",
		"日本の店":                      "mi-negocio",
	} {
		got := SlugFromName(in)
		if got != want {
			t.Errorf("SlugFromName(%q) = %q, want %q", in, got, want)
		}
		if _, err := ParseSlug(string(got)); err != nil {
			t.Errorf("SlugFromName(%q) = %q is not a valid slug: %v", in, got, err)
		}
	}
	long := SlugFromName(strings.Repeat("peluqueria ", 10))
	if len(long) > MaxSlugLength || strings.HasSuffix(string(long), "-") {
		t.Errorf("long slug %q", long)
	}
	if got := Slug("barberia").WithSuffix(2); got != "barberia-2" {
		t.Errorf("WithSuffix = %q", got)
	}
	if got := Slug(strings.Repeat("a", 60)).WithSuffix(12); len(got) != 60 || !strings.HasSuffix(string(got), "-12") {
		t.Errorf("WithSuffix on a max-length slug = %q (%d)", got, len(got))
	}
}

func TestParsePhone(t *testing.T) {
	for in, want := range map[string]string{
		"300 123 4567":     "+573001234567",
		"(602) 486 1234":   "+576024861234",
		"+57 300 123 4567": "+573001234567",
		"+1 202 555 0123":  "+12025550123",
		"  3001234567  ":   "+573001234567",
	} {
		got, err := ParsePhone(in, "co")
		if err != nil || got != want {
			t.Errorf("ParsePhone(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"", "123", "abc", "+57 1", strings.Repeat("3", 33), "+999 123456"} {
		if _, err := ParsePhone(in, "CO"); !errors.Is(err, ErrInvalidPhone) {
			t.Errorf("ParsePhone(%q) err = %v", in, err)
		}
	}
}

func TestCoordinatesTimeZoneCurrency(t *testing.T) {
	if _, err := NewCoordinates(3.4516, -76.5320); err != nil {
		t.Error(err)
	}
	for _, c := range [][2]float64{{91, 0}, {0, 181}, {-91, 1}, {0, 0}, {math.NaN(), 1}} {
		if _, err := NewCoordinates(c[0], c[1]); !errors.Is(err, ErrInvalidCoordinates) {
			t.Errorf("NewCoordinates(%v) err = %v", c, err)
		}
	}
	if _, err := ParseTimeZone("America/Bogota"); err != nil {
		t.Error(err)
	}
	for _, in := range []string{"", "Local", "Nope/Nope", strings.Repeat("A", 65)} {
		if _, err := ParseTimeZone(in); !errors.Is(err, ErrInvalidTimeZone) {
			t.Errorf("ParseTimeZone(%q) err = %v", in, err)
		}
	}
	if _, err := ParseCurrency("COP"); err != nil {
		t.Error(err)
	}
	for _, in := range []string{"cop", "CO", "COPS", ""} {
		if _, err := ParseCurrency(in); !errors.Is(err, ErrInvalidCurrency) {
			t.Errorf("ParseCurrency(%q) err = %v", in, err)
		}
	}
	if _, err := ParseStatus("DELETED"); !errors.Is(err, ErrInvalidStatus) {
		t.Errorf("ParseStatus err = %v", err)
	}
}

func TestPolicies(t *testing.T) {
	p := DefaultBookingPolicy()
	if p.Validate() != nil || p.SlotIntervalMinutes != 10 || p.MinLeadMinutes != 60 || p.MaxAdvanceDays != 60 || !p.RequireCustomerConfirmation {
		t.Fatalf("defaults %+v", p)
	}
	for field, mutate := range map[string]func(*BookingPolicy){
		"slotIntervalMinutes":     func(p *BookingPolicy) { p.SlotIntervalMinutes = 7 },
		"bufferMinutes":           func(p *BookingPolicy) { p.BufferMinutes = 61 },
		"minLeadMinutes":          func(p *BookingPolicy) { p.MinLeadMinutes = -1 },
		"maxAdvanceDays":          func(p *BookingPolicy) { p.MaxAdvanceDays = 0 },
		"cancellationCutoffHours": func(p *BookingPolicy) { p.CancellationCutoffHours = 169 },
		"rescheduleCutoffHours":   func(p *BookingPolicy) { p.RescheduleCutoffHours = -1 },
	} {
		q := DefaultBookingPolicy()
		mutate(&q)
		if q.PolicyField() != field || !errors.Is(q.Validate(), ErrInvalidPolicy) {
			t.Errorf("%s: PolicyField = %q", field, q.PolicyField())
		}
	}
	if r := DefaultReminders(); r.Validate() != nil || !r.Enabled || r.HoursBefore != 24 {
		t.Errorf("default reminders %+v", r)
	}
	for _, h := range []int{0, 73} {
		if err := (Reminders{HoursBefore: h}).Validate(); !errors.Is(err, ErrInvalidReminder) {
			t.Errorf("reminder hours %d err = %v", h, err)
		}
	}
}

func validParams() NewBusinessParams {
	return NewBusinessParams{
		ID: uuid.Must(uuid.NewV7()), Name: "Barbería El Paisa", Slug: "barberia-el-paisa", Status: StatusActive,
		ContactPhone: "+573001234567",
		Location: Location{
			CountryCode: "CO", DivisionID: uuid.Must(uuid.NewV7()), CityID: uuid.Must(uuid.NewV7()),
			AddressLine: "Calle 5 # 38-25", Coordinates: &Coordinates{Lat: 3.45, Lng: -76.53},
		},
		TimeZone: "America/Bogota", Currency: "COP", Policy: DefaultBookingPolicy(), Reminders: DefaultReminders(),
		Now: time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC),
	}
}

func TestNewBusinessInvariants(t *testing.T) {
	b, err := NewBusiness(validParams())
	if err != nil {
		t.Fatal(err)
	}
	if !b.IsBookable() || b.Slug() != "barberia-el-paisa" || b.Location().Coordinates == nil || !b.CreatedAt().Equal(b.UpdatedAt()) {
		t.Errorf("business = %+v", b)
	}
	later := b.CreatedAt().Add(time.Hour)
	b.Suspend(later)
	if b.IsBookable() || b.Status() != StatusSuspended || !b.UpdatedAt().Equal(later) {
		t.Errorf("after suspend: %s %v", b.Status(), b.UpdatedAt())
	}
	b.Suspend(later.Add(time.Hour)) // idempotent: no new timestamp
	if !b.UpdatedAt().Equal(later) {
		t.Error("idempotent Suspend changed updatedAt")
	}
	b.Activate(later.Add(2 * time.Hour))
	if !b.IsBookable() {
		t.Error("Activate")
	}

	for name, tc := range map[string]struct {
		mutate func(*NewBusinessParams)
		want   error
	}{
		"nil id":      {func(p *NewBusinessParams) { p.ID = uuid.Nil }, ErrInvalidID},
		"name":        {func(p *NewBusinessParams) { p.Name = "x" }, ErrInvalidName},
		"slug":        {func(p *NewBusinessParams) { p.Slug = "Bad Slug" }, ErrInvalidSlug},
		"status":      {func(p *NewBusinessParams) { p.Status = "OPEN" }, ErrInvalidStatus},
		"phone":       {func(p *NewBusinessParams) { p.ContactPhone = "3001234567" }, ErrInvalidPhone},
		"description": {func(p *NewBusinessParams) { p.Description = strings.Repeat("a", 1001) }, ErrInvalidDescription},
		"no city":     {func(p *NewBusinessParams) { p.Location.CityID = uuid.Nil }, ErrInvalidLocation},
		"country":     {func(p *NewBusinessParams) { p.Location.CountryCode = "COL" }, ErrInvalidLocation},
		"address":     {func(p *NewBusinessParams) { p.Location.AddressLine = "" }, ErrInvalidAddress},
		"coordinates": {func(p *NewBusinessParams) { p.Location.Coordinates = &Coordinates{Lat: 100} }, ErrInvalidCoordinates},
		"time zone":   {func(p *NewBusinessParams) { p.TimeZone = "Local" }, ErrInvalidTimeZone},
		"currency":    {func(p *NewBusinessParams) { p.Currency = "pesos" }, ErrInvalidCurrency},
		"policy":      {func(p *NewBusinessParams) { p.Policy.SlotIntervalMinutes = 3 }, ErrInvalidPolicy},
		"reminder":    {func(p *NewBusinessParams) { p.Reminders.HoursBefore = 0 }, ErrInvalidReminder},
	} {
		p := validParams()
		tc.mutate(&p)
		if _, err := NewBusiness(p); !errors.Is(err, tc.want) {
			t.Errorf("%s: err = %v, want %v", name, err, tc.want)
		}
	}
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	r, err := RehydrateBusiness(validParams(), created, created.Add(time.Minute))
	if err != nil || !r.CreatedAt().Equal(created) || r.UpdatedAt().Sub(created) != time.Minute {
		t.Errorf("rehydrate: %v %v", r, err)
	}
	if _, err := RehydrateBusiness(NewBusinessParams{}, created, created); err == nil {
		t.Error("rehydrating an invalid row must fail")
	}
}

func TestApplyUpdate(t *testing.T) {
	b, err := NewBusiness(validParams())
	if err != nil {
		t.Fatal(err)
	}
	later := b.CreatedAt().Add(time.Hour)
	name, phone := "Barbería Nueva", "+576024861234"
	loc := b.Location()
	loc.CityID, loc.DivisionID, loc.AddressLine, loc.Coordinates = uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()), "Cra 70 # 1-1", nil
	tz := "America/Bogota"
	pol := b.Policy()
	pol.SlotIntervalMinutes = 15
	rem := Reminders{Enabled: false, HoursBefore: 2}

	changed, err := b.Apply(Update{Name: &name, ContactPhone: &phone, Location: &loc, TimeZone: &tz, Policy: &pol, Reminders: &rem}, later)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"name", "contactPhone", "location", "bookingPolicy", "reminders"} // timeZone unchanged
	if strings.Join(changed, ",") != strings.Join(want, ",") {
		t.Errorf("changed = %v, want %v", changed, want)
	}
	if b.Name() != name || b.Location().Coordinates != nil || b.Policy().SlotIntervalMinutes != 15 || !b.UpdatedAt().Equal(later) {
		t.Errorf("after apply: %+v", b)
	}

	// Same values: no change, no new timestamp.
	if changed, err := b.Apply(Update{Name: &name, Location: &loc}, later.Add(time.Hour)); err != nil || len(changed) != 0 || !b.UpdatedAt().Equal(later) {
		t.Errorf("no-op: %v %v %v", changed, err, b.UpdatedAt())
	}
	// Invalid results leave the business untouched.
	bad := "x"
	if _, err := b.Apply(Update{Name: &bad}, later); !errors.Is(err, ErrInvalidName) || b.Name() != name {
		t.Errorf("invalid name: %v, name %q", err, b.Name())
	}
	other := loc
	other.CountryCode = "MX"
	if _, err := b.Apply(Update{Location: &other}, later); !errors.Is(err, ErrInvalidLocation) {
		t.Errorf("moving country: %v", err)
	}
	badPol := pol
	badPol.BufferMinutes = 90
	if _, err := b.Apply(Update{Policy: &badPol, Name: &name}, later); !errors.Is(err, ErrInvalidPolicy) || b.Policy().BufferMinutes == 90 {
		t.Errorf("invalid policy: %v", err)
	}
	withCoords := loc
	withCoords.Coordinates = &Coordinates{Lat: 6.2, Lng: -75.5}
	if changed, _ := b.Apply(Update{Location: &withCoords}, later); len(changed) != 1 || b.Location().Coordinates == nil {
		t.Errorf("adding coordinates: %v", changed)
	}
	sameCoords := withCoords
	sameCoords.Coordinates = &Coordinates{Lat: 6.2, Lng: -75.5}
	if changed, _ := b.Apply(Update{Location: &sameCoords}, later); len(changed) != 0 {
		t.Errorf("equal coordinates (other pointer) reported as a change: %v", changed)
	}
}
