package player

import "testing"

func TestStreamKindString(t *testing.T) {
	cases := map[StreamKind]string{
		StreamHLS:   "hls",
		StreamMP4:   "mp4",
		StreamFLV:   "flv",
		StreamTS:    "ts",
		StreamRTMP:  "rtmp",
		StreamLocal: "local",
		StreamAudio: "audio",
		StreamVideo: "video",
	}
	for k, want := range cases {
		if got := k.String(); got != want {
			t.Errorf("StreamKind(%d).String() = %q, want %q", k, got, want)
		}
	}
}

// KindForURL 要识别音频直链，不能把 .flac/.mflac/.mp3 等误判成 HLS。
func TestKindForURLAudio(t *testing.T) {
	cases := []struct {
		url  string
		want StreamKind
	}{
		{"https://cdn.example.com/song.mflac", StreamAudio},
		{"https://cdn.example.com/song.flac", StreamAudio},
		{"https://cdn.example.com/song.mp3", StreamAudio},
		{"https://cdn.example.com/song.m4a", StreamAudio},
		{"https://cdn.example.com/song.aac", StreamAudio},
		{"https://cdn.example.com/song.m3u8", StreamHLS},
		{"https://cdn.example.com/a.mp4", StreamMP4},
		{"https://cdn.example.com/a.flv", StreamFLV},
		{"https://cdn.example.com/a.ts", StreamTS},
		{"https://cdn.example.com/a.mkv", StreamVideo}, // 视频容器走 mpv，不再按 HLS 兜底
	}
	for _, tc := range cases {
		if got := KindForURL(tc.url); got != tc.want {
			t.Errorf("KindForURL(%q) = %v, want %v", tc.url, got, tc.want)
		}
	}
}

// KindForURL 要把浏览器播不了的视频容器（mkv/avi/mov 等）判给 mpv，
// 不能按未知流兜底成 HLS 走 Web；webm 浏览器原生可播，按 MP4 走 Web。
func TestKindForURLVideoContainers(t *testing.T) {
	cases := []struct {
		url  string
		want StreamKind
	}{
		{"https://cdn.example.com/a.mkv", StreamVideo},
		{"https://cdn.example.com/a.avi", StreamVideo},
		{"https://cdn.example.com/a.mov", StreamVideo},
		{"https://cdn.example.com/a.wmv", StreamVideo},
		{"https://cdn.example.com/a.rmvb", StreamVideo},
		{"https://cdn.example.com/a.rm", StreamVideo},
		{"https://cdn.example.com/a.mpg", StreamVideo},
		{"https://cdn.example.com/a.mpeg", StreamVideo},
		{"https://cdn.example.com/a.3gp", StreamVideo},
		{"https://cdn.example.com/a.vob", StreamVideo},
		{"https://cdn.example.com/a.MKV", StreamVideo},
		{"https://cdn.example.com/a.mkv?token=1", StreamVideo},
		{"https://cdn.example.com/a.webm", StreamMP4},
		{"https://cdn.example.com/song.wma", StreamAudio},
		{"https://cdn.example.com/song.mka", StreamAudio},
		{"https://cdn.example.com/live?token=1", StreamHLS},
	}
	for _, tc := range cases {
		if got := KindForURL(tc.url); got != tc.want {
			t.Errorf("KindForURL(%q) = %v, want %v", tc.url, got, tc.want)
		}
	}
}

func TestTrackKindString(t *testing.T) {
	cases := map[TrackKind]string{
		TrackAudio:    "audio",
		TrackSubtitle: "subtitle",
		TrackVideo:    "video",
	}
	for k, want := range cases {
		if got := k.String(); got != want {
			t.Errorf("TrackKind(%d).String() = %q, want %q", k, got, want)
		}
	}
}

func TestStateZeroValueIsStopped(t *testing.T) {
	var s State
	if s.Playing != StateStopped {
		t.Errorf("零值 State.Playing = %v, want %v", s.Playing, StateStopped)
	}
}
