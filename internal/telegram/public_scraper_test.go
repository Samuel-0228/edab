package telegram

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCleanTelegramHTML(t *testing.T) {
	raw := `<b>Bold title</b><br/><br/>Some text with <a href="https://example.com" onclick="alert(1)">link</a> and <tg-emoji emoji-id="123"><i class="emoji"><b>🚀</b></i></tg-emoji> and <div class="bad">nested</div>.`
	got := CleanTelegramHTML(raw)
	expected := `<b>Bold title</b>

Some text with <a href="https://example.com">link</a> and <b>🚀</b> and nested.`
	if got != expected {
		t.Errorf("CleanTelegramHTML() mismatch\ngot:\n%s\nexpected:\n%s", got, expected)
	}
}

func TestStripHTML(t *testing.T) {
	raw := `<b>Title</b><br/>Line 2 &amp; &quot;quotes&quot;`
	got := StripHTML(raw)
	expected := "Title\nLine 2 & \"quotes\""
	if got != expected {
		t.Errorf("StripHTML() mismatch: got %q, want %q", got, expected)
	}
}

func TestParseChannelHTML(t *testing.T) {
	sampleHTML := `
<!DOCTYPE html>
<html>
<body>
<div class="tgme_widget_message_wrap js-widget_message_wrap">
  <div class="tgme_widget_message text_not_supported_wrap js-widget_message" data-post="tech_news/101">
    <a class="tgme_widget_message_photo_wrap" style="background-image:url('https://cdn1.telesco.pe/file/photo1.jpg')"></a>
    <div class="tgme_widget_message_text js-message_text" dir="auto"><b>First Post</b><br/>Details here</div>
    <time datetime="2026-10-16T12:00:00Z"></time>
  </div>
</div>
<div class="tgme_widget_message_wrap js-widget_message_wrap">
  <div class="tgme_widget_message text_not_supported_wrap js-widget_message" data-post="tech_news/102">
    <video src="https://cdn1.telesco.pe/file/video1.mp4" class="tgme_widget_message_video"></video>
    <div class="tgme_widget_message_text js-message_text" dir="auto">Video caption</div>
    <time datetime="2026-10-16T12:05:00Z"></time>
  </div>
</div>
<div class="tgme_widget_message_wrap js-widget_message_wrap">
  <div class="tgme_widget_message text_not_supported_wrap js-widget_message" data-post="tech_news/103">
    <div class="tgme_widget_message_grouped">
      <a class="tgme_widget_message_photo_wrap" style="background-image:url('https://cdn1.telesco.pe/file/album1.jpg')"></a>
      <a class="tgme_widget_message_photo_wrap" style="background-image:url('https://cdn1.telesco.pe/file/album2.jpg')"></a>
    </div>
    <div class="tgme_widget_message_text js-message_text" dir="auto">Album caption</div>
    <time datetime="2026-10-16T12:10:00Z"></time>
  </div>
</div>
</body>
</html>`

	posts := ParseChannelHTML(sampleHTML, "tech_news")
	if len(posts) != 3 {
		t.Fatalf("expected 3 posts, got %d", len(posts))
	}

	// Post 101
	p1 := posts[0]
	if p1.MessageID != 101 || len(p1.Media) != 1 || p1.Media[0].Kind != "photo" {
		t.Errorf("post 101 mismatch: %+v", p1)
	}
	if p1.HTMLText != "<b>First Post</b>\nDetails here" {
		t.Errorf("post 101 html mismatch: %q", p1.HTMLText)
	}

	// Post 102
	p2 := posts[1]
	if p2.MessageID != 102 || len(p2.Media) != 1 || p2.Media[0].Kind != "video" {
		t.Errorf("post 102 mismatch: %+v", p2)
	}
	if p2.Text != "Video caption" {
		t.Errorf("post 102 text mismatch: %q", p2.Text)
	}

	// Post 103 (Album)
	p3 := posts[2]
	if p3.MessageID != 103 || len(p3.Media) != 2 || !p3.IsAlbum {
		t.Errorf("post 103 mismatch: %+v", p3)
	}
}

func TestPublicScraper_FetchPosts(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/testchannel" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`
<div class="tgme_widget_message_wrap">
  <div class="tgme_widget_message" data-post="testchannel/55">
    <div class="tgme_widget_message_text">Hello test</div>
  </div>
</div>
`))
	}))
	defer ts.Close()

	scraper := NewPublicScraperWithClient(ts.Client(), ts.URL+"/")
	posts, err := scraper.FetchPosts(context.Background(), "@testchannel")
	if err != nil {
		t.Fatalf("FetchPosts failed: %v", err)
	}
	if len(posts) != 1 || posts[0].MessageID != 55 {
		t.Fatalf("unexpected posts: %+v", posts)
	}
	if posts[0].Text != "Hello test" {
		t.Errorf("unexpected text: %q", posts[0].Text)
	}
}
