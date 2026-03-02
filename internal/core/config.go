package core

import "time"

// Config holds the application configuration.
type Config struct {
	SuccessURLPattern string
	SuccessSelector   string
	UserDataDir       string
	VenueBaseURL      string
	Provider          string
	Headless          bool
	Timeout           time.Duration
}
