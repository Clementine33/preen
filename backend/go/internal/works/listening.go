package works

// Listening states of a work, derived from its Rendition rows.
const (
	ListeningNone     = "none"
	ListeningActive   = "listening"
	ListeningFinished = "finished"
)

// Listening is one stored Listening Position row. The track fields are nil
// once the Rendition is finished. The server never derives tracks: index and
// count are what the client snapshotted at write time.
type Listening struct {
	Rendition   string
	TrackKey    *string
	PositionSec *float64
	TrackIndex  *int
	TrackCount  *int
	FinishedAt  *string
	UpdatedAt   string
}

// ListeningSummary is the listing's per-work digest; index, count and time come
// from the most recently updated row with a position.
type ListeningSummary struct {
	State      string
	TrackIndex *int
	TrackCount *int
	UpdatedAt  *string
}

// ListeningRequest is the PUT /works/{rj}/listening body. Client and Seq order
// one page's writes (the store drops a same-client write whose Seq is not
// greater).
type ListeningRequest struct {
	Rendition   string  `json:"rendition"`
	TrackKey    string  `json:"track_key"`
	PositionSec float64 `json:"position_sec"`
	TrackIndex  int     `json:"track_index"`
	TrackCount  int     `json:"track_count"`
	Finished    bool    `json:"finished"`
	Client      string  `json:"client"`
	Seq         int64   `json:"seq"`
}

// Validate checks the body; the track fields are required even when finishing.
func (r ListeningRequest) Validate() *Error {
	switch {
	case r.TrackKey == "":
		return Invalid(CodeInvalidArgument, "track_key must not be empty")
	case r.PositionSec < 0:
		return Invalid(CodeInvalidArgument, "position_sec must not be negative")
	case r.TrackIndex < 0 || r.TrackIndex >= r.TrackCount:
		return Invalid(CodeInvalidArgument, "track_index must be within track_count")
	case r.Client == "":
		return Invalid(CodeInvalidArgument, "client must not be empty")
	}
	return nil
}

// validListeningStates reports whether every state is one the filter accepts.
func validListeningStates(states []string) bool {
	for _, state := range states {
		if state != ListeningNone && state != ListeningActive && state != ListeningFinished {
			return false
		}
	}
	return true
}
