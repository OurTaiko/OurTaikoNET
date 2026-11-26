package httpapi

import "ourtaiko.dev/net/internal/awards"

// What a source needs of the players' part is shared by both sources (package awards); these
// are the names this code has always used for it.
type (
	ProfileClient    = awards.Client
	ProfileConfig    = awards.Config
	ProfileRejection = awards.Rejection
	AwardRequest     = awards.Request
	Awarder          = awards.Awarder
)

var NewProfile = awards.New
