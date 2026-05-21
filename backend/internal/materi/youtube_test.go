package materi

import (
	"errors"
	"testing"
)

func TestParseYouTubeID(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "watch_v_https", input: "https://www.youtube.com/watch?v=dQw4w9WgXcQ", want: "dQw4w9WgXcQ"},
		{name: "watch_v_with_extra_params", input: "https://www.youtube.com/watch?v=dQw4w9WgXcQ&t=43s&list=RD", want: "dQw4w9WgXcQ"},
		{name: "watch_v_no_www", input: "https://youtube.com/watch?v=dQw4w9WgXcQ", want: "dQw4w9WgXcQ"},
		{name: "watch_v_mobile", input: "https://m.youtube.com/watch?v=dQw4w9WgXcQ", want: "dQw4w9WgXcQ"},
		{name: "youtu_be_short", input: "https://youtu.be/dQw4w9WgXcQ", want: "dQw4w9WgXcQ"},
		{name: "youtu_be_with_query", input: "https://youtu.be/dQw4w9WgXcQ?t=10", want: "dQw4w9WgXcQ"},
		{name: "shorts", input: "https://www.youtube.com/shorts/abcDEF12345", want: "abcDEF12345"},
		{name: "embed", input: "https://www.youtube.com/embed/dQw4w9WgXcQ", want: "dQw4w9WgXcQ"},
		{name: "nocookie_embed", input: "https://www.youtube-nocookie.com/embed/dQw4w9WgXcQ", want: "dQw4w9WgXcQ"},
		{name: "scheme_less", input: "youtu.be/dQw4w9WgXcQ", want: "dQw4w9WgXcQ"},
		{name: "trim_whitespace", input: "  https://youtu.be/dQw4w9WgXcQ  ", want: "dQw4w9WgXcQ"},
		{name: "id_with_underscore_and_dash", input: "https://www.youtube.com/watch?v=A_b-12345_-", want: "A_b-12345_-"},
		{name: "http_scheme", input: "http://www.youtube.com/watch?v=dQw4w9WgXcQ", want: "dQw4w9WgXcQ"},
		{name: "uppercase_host", input: "https://WWW.YOUTUBE.COM/watch?v=dQw4w9WgXcQ", want: "dQw4w9WgXcQ"},

		// Invalid cases.
		{name: "empty", input: "", wantErr: true},
		{name: "non_youtube_host", input: "https://vimeo.com/12345", wantErr: true},
		{name: "youtube_no_video_id", input: "https://www.youtube.com/watch", wantErr: true},
		{name: "youtube_short_id", input: "https://www.youtube.com/watch?v=short", wantErr: true},
		{name: "youtube_long_id", input: "https://www.youtube.com/watch?v=dQw4w9WgXcQEXTRA", wantErr: true},
		{name: "youtube_invalid_chars", input: "https://www.youtube.com/watch?v=dQw4w9!gXcQ", wantErr: true},
		{name: "ftp_scheme", input: "ftp://youtu.be/dQw4w9WgXcQ", wantErr: true},
		{name: "youtu_be_empty_path", input: "https://youtu.be/", wantErr: true},
		{name: "shorts_empty", input: "https://www.youtube.com/shorts/", wantErr: true},
		{name: "embed_empty", input: "https://www.youtube.com/embed/", wantErr: true},
		{name: "youtube_unknown_path", input: "https://www.youtube.com/playlist?list=ABC", wantErr: true},
		{name: "garbage", input: "not a url", wantErr: true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseYouTubeID(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got id=%q", got)
				}
				if !errors.Is(err, ErrInvalidYouTubeURL) {
					t.Fatalf("expected ErrInvalidYouTubeURL, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("id mismatch: got %q want %q", got, tc.want)
			}
		})
	}
}
