// YouTube URL parser for materi tipe='youtube' (locked decision #65).
//
// parseYouTubeID extracts the 11-character video_id from a YouTube URL,
// supporting four formats:
//   - youtube.com/watch?v=<id>      (with optional extra params)
//   - youtu.be/<id>                 (short share link)
//   - youtube.com/shorts/<id>       (Shorts)
//   - youtube.com/embed/<id>        (embed link)
//
// The id MUST be exactly 11 chars from [A-Za-z0-9_-]. Anything else returns
// ErrInvalidYouTubeURL. Schemes are normalised: http/https + optional www
// or m subdomain are accepted.
//
// Storage: only the video_id is persisted (in materi.konten). Embeds use
// `https://www.youtube-nocookie.com/embed/<id>` for privacy (no tracking
// cookie until user clicks play).
package materi

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
)

// ErrInvalidYouTubeURL is returned when an input URL doesn't match a known
// YouTube format or yields a video_id that isn't 11 chars [A-Za-z0-9_-].
var ErrInvalidYouTubeURL = errors.New("materi: invalid youtube url")

// videoIDRe matches an 11-char YouTube video id. URL-safe base64 alphabet
// without padding (Google's id format since launch).
var videoIDRe = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)

// parseYouTubeID returns the canonical 11-char video_id for a YouTube URL.
//
// Trims whitespace; tolerates http/https + www/m subdomain. Returns
// ErrInvalidYouTubeURL for anything that doesn't resolve cleanly to an
// 11-char id.
func parseYouTubeID(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", ErrInvalidYouTubeURL
	}

	// Allow scheme-less input like "youtu.be/abc..." by prepending https://.
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}

	u, err := url.Parse(s)
	if err != nil {
		return "", ErrInvalidYouTubeURL
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", ErrInvalidYouTubeURL
	}

	host := strings.ToLower(u.Host)
	host = strings.TrimPrefix(host, "www.")
	host = strings.TrimPrefix(host, "m.")

	var id string
	switch host {
	case "youtu.be":
		// youtu.be/<id>[?...]
		id = strings.TrimPrefix(u.Path, "/")
		// Stop at any further slash (defensive — youtu.be shouldn't have nested paths).
		if i := strings.Index(id, "/"); i >= 0 {
			id = id[:i]
		}
	case "youtube.com", "youtube-nocookie.com":
		switch {
		case u.Path == "/watch":
			id = u.Query().Get("v")
		case strings.HasPrefix(u.Path, "/embed/"):
			id = strings.TrimPrefix(u.Path, "/embed/")
			if i := strings.Index(id, "/"); i >= 0 {
				id = id[:i]
			}
		case strings.HasPrefix(u.Path, "/shorts/"):
			id = strings.TrimPrefix(u.Path, "/shorts/")
			if i := strings.Index(id, "/"); i >= 0 {
				id = id[:i]
			}
		case strings.HasPrefix(u.Path, "/v/"):
			// Legacy /v/<id> embed.
			id = strings.TrimPrefix(u.Path, "/v/")
			if i := strings.Index(id, "/"); i >= 0 {
				id = id[:i]
			}
		default:
			return "", ErrInvalidYouTubeURL
		}
	default:
		return "", ErrInvalidYouTubeURL
	}

	if !videoIDRe.MatchString(id) {
		return "", ErrInvalidYouTubeURL
	}
	return id, nil
}
