package telegram

import (
	"context"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var (
	rePostChunk   = regexp.MustCompile(`(?s)<div class="[^"]*tgme_widget_message_wrap[^"]*"`)
	reDataPost    = regexp.MustCompile(`data-post="([^"/]+)/(\d+)"`)
	rePhoto       = regexp.MustCompile(`class="[^"]*tgme_widget_message_photo_wrap[^"]*"[^>]*style="[^"]*background-image:url\('([^']+)'\)`)
	reVideo       = regexp.MustCompile(`<video[^>]*src="([^"]+)"`)
	reText        = regexp.MustCompile(`(?s)<div class="[^"]*tgme_widget_message_text[^"]*"[^>]*>(.*?)</div>\s*(?:<div class="tgme_widget_message_footer|<div class="message_media_not_supported_wrap|<div class="media_not_supported_cont|$)`)
	reTime        = regexp.MustCompile(`(?s)<time datetime="([^"]+)"`)
	reGroupedWrap = regexp.MustCompile(`tgme_widget_message_grouped`)
	reBr          = regexp.MustCompile(`(?i)<br\s*/?>`)
	reEmojiWrap   = regexp.MustCompile(`(?i)</?(?:tg-emoji|i)[^>]*>`)
	reAnchorHref  = regexp.MustCompile(`(?i)<a\b[^>]*href="([^"]+)"[^>]*>`)
	reGenericTag  = regexp.MustCompile(`(?i)</?([a-z0-9_-]+)[^>]*>`)
	reAllTags     = regexp.MustCompile(`(?s)<[^>]+>`)
)

// Allowed Telegram HTML formatting tags
var allowedTags = map[string]bool{
	"b":                     true,
	"strong":                true,
	"i":                     true,
	"em":                    true,
	"u":                     true,
	"ins":                   true,
	"s":                     true,
	"strike":                true,
	"del":                   true,
	"a":                     true,
	"code":                  true,
	"pre":                   true,
	"blockquote":            true,
	"tg-spoiler":            true,
	"expandable-blockquote": true,
}

// PublicMedia represents a media item attached to a public Telegram post.
type PublicMedia struct {
	Kind string `json:"kind"` // "photo" or "video"
	URL  string `json:"url"`
}

// PublicPost represents a normalized post scraped from a public Telegram channel.
type PublicPost struct {
	Channel   string        `json:"channel"`
	MessageID int           `json:"message_id"`
	Text      string        `json:"text"`
	HTMLText  string        `json:"html_text"`
	Media     []PublicMedia `json:"media"`
	Date      time.Time     `json:"date"`
	IsAlbum   bool          `json:"is_album"`
}

// PublicScraper fetches and parses posts from public Telegram channel web previews.
type PublicScraper struct {
	client  *http.Client
	baseURL string
}

// NewPublicScraper creates a new PublicScraper with a default timeout and base URL.
func NewPublicScraper() *PublicScraper {
	return &PublicScraper{
		client: &http.Client{
			Timeout: 20 * time.Second,
		},
		baseURL: "https://t.me/s/",
	}
}

// NewPublicScraperWithClient creates a PublicScraper with a custom client and baseURL (useful in tests).
func NewPublicScraperWithClient(client *http.Client, baseURL string) *PublicScraper {
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	if baseURL == "" {
		baseURL = "https://t.me/s/"
	}
	return &PublicScraper{
		client:  client,
		baseURL: baseURL,
	}
}

// NormalizeChannelName strips prefixes like '@', 'https://t.me/', etc.
func NormalizeChannelName(raw string) string {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "@")
	s = strings.TrimPrefix(s, "https://t.me/s/")
	s = strings.TrimPrefix(s, "http://t.me/s/")
	s = strings.TrimPrefix(s, "https://t.me/")
	s = strings.TrimPrefix(s, "http://t.me/")
	s = strings.TrimPrefix(s, "t.me/s/")
	s = strings.TrimPrefix(s, "t.me/")
	s = strings.Trim(s, "/")
	return strings.ToLower(s)
}

// FetchPosts scrapes the public channel web preview and returns posts sorted ascending by MessageID.
func (s *PublicScraper) FetchPosts(ctx context.Context, channelName string) ([]*PublicPost, error) {
	ch := NormalizeChannelName(channelName)
	if ch == "" {
		return nil, fmt.Errorf("channel name cannot be empty")
	}

	reqURL := s.baseURL + ch
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request for %s: %w", reqURL, err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request to %s failed: %w", reqURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code %d fetching %s", resp.StatusCode, reqURL)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024)) // 10MB safety cap
	if err != nil {
		return nil, fmt.Errorf("failed to read response body from %s: %w", reqURL, err)
	}

	return ParseChannelHTML(string(body), ch), nil
}

// ParseChannelHTML extracts PublicPosts from raw Telegram web preview HTML.
func ParseChannelHTML(htmlContent, expectedChannel string) []*PublicPost {
	chunks := rePostChunk.Split(htmlContent, -1)
	if len(chunks) <= 1 {
		return nil
	}

	var posts []*PublicPost

	for _, chunk := range chunks[1:] {
		dataMatch := reDataPost.FindStringSubmatch(chunk)
		if len(dataMatch) < 3 {
			continue
		}
		ch := strings.ToLower(dataMatch[1])
		if expectedChannel != "" && ch != expectedChannel {
			// allow parsing even if channel doesn't match strictly, but prioritize expected
			ch = expectedChannel
		}
		msgID, err := strconv.Atoi(dataMatch[2])
		if err != nil || msgID <= 0 {
			continue
		}

		// Extract photos
		photoMatches := rePhoto.FindAllStringSubmatch(chunk, -1)
		var media []PublicMedia
		seenURLs := make(map[string]bool)
		for _, pm := range photoMatches {
			if len(pm) >= 2 && !seenURLs[pm[1]] {
				seenURLs[pm[1]] = true
				media = append(media, PublicMedia{Kind: "photo", URL: pm[1]})
			}
		}

		// Extract videos
		videoMatches := reVideo.FindAllStringSubmatch(chunk, -1)
		for _, vm := range videoMatches {
			if len(vm) >= 2 && !seenURLs[vm[1]] {
				seenURLs[vm[1]] = true
				media = append(media, PublicMedia{Kind: "video", URL: vm[1]})
			}
		}

		// Extract text
		var rawText string
		textMatch := reText.FindStringSubmatch(chunk)
		if len(textMatch) >= 2 {
			rawText = textMatch[1]
		}

		htmlText := CleanTelegramHTML(rawText)
		plainText := StripHTML(rawText)

		// Check if it's an album / media group
		isAlbum := len(media) > 1 || reGroupedWrap.MatchString(chunk)

		// Extract date
		var postDate time.Time
		timeMatch := reTime.FindStringSubmatch(chunk)
		if len(timeMatch) >= 2 {
			if t, err := time.Parse(time.RFC3339, timeMatch[1]); err == nil {
				postDate = t
			}
		}

		posts = append(posts, &PublicPost{
			Channel:   ch,
			MessageID: msgID,
			Text:      plainText,
			HTMLText:  htmlText,
			Media:     media,
			Date:      postDate,
			IsAlbum:   isAlbum,
		})
	}

	sort.Slice(posts, func(i, j int) bool {
		return posts[i].MessageID < posts[j].MessageID
	})

	return posts
}

// CleanTelegramHTML sanitizes raw web HTML into Telegram Bot API compatible HTML formatting.
func CleanTelegramHTML(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}

	// 1. Replace <br> and <br/> with newline
	s := reBr.ReplaceAllString(raw, "\n")

	// 2. Strip <tg-emoji> and <i> wrappers from custom emojis
	s = reEmojiWrap.ReplaceAllString(s, "")

	// 3. Normalize <a> tags so they only keep href
	s = reAnchorHref.ReplaceAllString(s, `<a href="$1">`)

	// 4. Remove any HTML tags not in Telegram's whitelist while preserving text content
	s = reGenericTag.ReplaceAllStringFunc(s, func(tag string) string {
		match := reGenericTag.FindStringSubmatch(tag)
		if len(match) < 2 {
			return ""
		}
		tagName := strings.ToLower(match[1])
		if allowedTags[tagName] {
			return tag
		}
		return ""
	})

	return strings.TrimSpace(s)
}

// StripHTML removes all HTML tags and unescapes HTML entities.
func StripHTML(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	s := reBr.ReplaceAllString(raw, "\n")
	s = reAllTags.ReplaceAllString(s, "")
	return strings.TrimSpace(html.UnescapeString(s))
}

// DownloadMedia downloads binary data from a public URL with a maximum size cap.
func DownloadMedia(ctx context.Context, client *http.Client, mediaURL string, maxBytes int64) ([]byte, error) {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	if maxBytes <= 0 {
		maxBytes = 50 * 1024 * 1024 // 50MB default Telegram bot file limit
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, mediaURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create download request for %s: %w", mediaURL, err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to download media from %s: %w", mediaURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d downloading media from %s", resp.StatusCode, mediaURL)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to read media stream from %s: %w", mediaURL, err)
	}

	return data, nil
}
