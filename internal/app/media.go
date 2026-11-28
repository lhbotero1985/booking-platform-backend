package app

import (
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/config"
	"github.com/lhbotero1985/booking-platform-backend/internal/platform/media"
)

// LinkMedia opens the media store (MEDIA_DIR), sets the public origin of the image URLs
// (PUBLIC_API_URL) and gives the store to the modules that upload images. Call once, before
// serving.
func LinkMedia(cfg config.Config, modules ...interface{ SetMedia(media.Store) }) (*media.Local, error) {
	store, err := media.NewLocal(cfg.MediaDir)
	if err != nil {
		return nil, err
	}
	media.SetBaseURL(cfg.PublicAPIURL)
	for _, m := range modules {
		m.SetMedia(store)
	}
	return store, nil
}
