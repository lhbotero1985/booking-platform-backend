package domain

import (
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
)

// MaxPortfolioImages is how many images of their best work a professional may show (decision of
// the user, 2026-11-27).
const MaxPortfolioImages = 10

// MaxCaptionLength bounds the caption of a portfolio image.
const MaxCaptionLength = 140

// ErrInvalidCaption means the caption is too long or has control characters.
var ErrInvalidCaption = errors.New("professionals: invalid caption")

// PortfolioImage is one image of a professional's best work; the file lives in the media store
// under Key.
type PortfolioImage struct {
	ID, BusinessID, ProfessionalID uuid.UUID
	Key                            string
	Caption                        string
	SortOrder                      int
	Width, Height                  int
	CreatedBy                      uuid.UUID
	CreatedAt, UpdatedAt           time.Time
}

// ParseCaption trims a caption and checks it (empty is allowed).
func ParseCaption(s string) (string, error) {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) > MaxCaptionLength || !utf8.ValidString(s) {
		return "", ErrInvalidCaption
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return "", ErrInvalidCaption
		}
	}
	return s, nil
}
