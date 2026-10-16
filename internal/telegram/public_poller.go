package telegram

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"
)

// PublicSourceInfo pairs a Damtie source ID with a normalized public Telegram channel username.
type PublicSourceInfo struct {
	SourceID string
	Channel  string
}

// PublicPoller periodically scrapes registered public Telegram channels and
// ingests new posts into the Damtie pipeline.
type PublicPoller struct {
	scraper    *PublicScraper
	stateStore PublicChannelStateStore
	ingester   Ingester
	resolver   func(ctx context.Context) ([]PublicSourceInfo, error)
	interval   time.Duration

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

// NewPublicPoller creates a PublicPoller.
func NewPublicPoller(
	scraper *PublicScraper,
	stateStore PublicChannelStateStore,
	ingester Ingester,
	resolver func(ctx context.Context) ([]PublicSourceInfo, error),
	interval time.Duration,
) *PublicPoller {
	if scraper == nil {
		scraper = NewPublicScraper()
	}
	if interval <= 0 {
		interval = 15 * time.Second
	}
	return &PublicPoller{
		scraper:    scraper,
		stateStore: stateStore,
		ingester:   ingester,
		resolver:   resolver,
		interval:   interval,
	}
}

// Start launches the public channel polling loop in the background.
func (p *PublicPoller) Start(ctx context.Context) {
	p.mu.Lock()
	defer p.mu.Unlock()

	pollCtx, cancel := context.WithCancel(ctx)
	p.cancel = cancel
	p.done = make(chan struct{})

	go p.run(pollCtx)
}

// Stop shuts down the public channel poller.
func (p *PublicPoller) Stop() {
	p.mu.Lock()
	cancel := p.cancel
	done := p.done
	p.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
}

func (p *PublicPoller) run(ctx context.Context) {
	defer close(p.done)

	slog.Info("[telegram-public] poller starting", slog.Duration("interval", p.interval))

	// Initial check on startup
	p.pollAll(ctx)

	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("[telegram-public] poller stopped")
			return
		case <-ticker.C:
			p.pollAll(ctx)
		}
	}
}

func (p *PublicPoller) pollAll(ctx context.Context) {
	if p.resolver == nil {
		return
	}

	sources, err := p.resolver(ctx)
	if err != nil {
		slog.Warn("[telegram-public] failed to resolve public sources", slog.String("error", err.Error()))
		return
	}

	for _, s := range sources {
		if ctx.Err() != nil {
			return
		}
		p.pollSource(ctx, s)
	}
}

func (p *PublicPoller) pollSource(ctx context.Context, source PublicSourceInfo) {
	channel := NormalizeChannelName(source.Channel)
	if channel == "" {
		return
	}

	lastID, err := p.stateStore.GetLastMessageID(ctx, channel)
	if err != nil {
		slog.Warn("[telegram-public] failed to get last message id",
			slog.String("channel", channel),
			slog.String("error", err.Error()),
		)
		return
	}

	posts, err := p.scraper.FetchPosts(ctx, channel)
	if err != nil {
		slog.Warn("[telegram-public] failed to fetch public channel posts",
			slog.String("channel", channel),
			slog.String("error", err.Error()),
		)
		return
	}

	if len(posts) == 0 {
		return
	}

	// First time seeing this channel: bookmark the highest existing message ID so
	// we never flood the destination with past channel history.
	if lastID == 0 {
		latestMsgID := posts[len(posts)-1].MessageID
		if err := p.stateStore.SetLastMessageID(ctx, channel, latestMsgID); err != nil {
			slog.Warn("[telegram-public] failed to initialize channel offset",
				slog.String("channel", channel),
				slog.Int("offset", latestMsgID),
				slog.String("error", err.Error()),
			)
		} else {
			slog.Info("[telegram-public] initialized channel offset",
				slog.String("channel", channel),
				slog.Int("latest_message_id", latestMsgID),
			)
		}
		return
	}

	// Process any posts newer than lastID
	for _, post := range posts {
		if post.MessageID <= lastID {
			continue
		}

		rawBytes, err := json.Marshal(post)
		if err != nil {
			slog.Error("[telegram-public] failed to marshal post",
				slog.String("channel", channel),
				slog.Int("message_id", post.MessageID),
				slog.String("error", err.Error()),
			)
			continue
		}

		slog.Info("[telegram-public] new post detected",
			slog.String("channel", channel),
			slog.Int("message_id", post.MessageID),
			slog.Int("media_count", len(post.Media)),
			slog.Bool("is_album", post.IsAlbum),
		)

		if err := p.ingester.IngestRaw(ctx, source.SourceID, rawBytes); err != nil {
			slog.Error("[telegram-public] failed to ingest post",
				slog.String("channel", channel),
				slog.Int("message_id", post.MessageID),
				slog.String("error", err.Error()),
			)
			// Don't advance offset if ingestion failed completely
			break
		}

		// Advance confirmed offset
		lastID = post.MessageID
		if err := p.stateStore.SetLastMessageID(ctx, channel, lastID); err != nil {
			slog.Warn("[telegram-public] failed to update channel offset",
				slog.String("channel", channel),
				slog.Int("offset", lastID),
				slog.String("error", err.Error()),
			)
		}
	}
}
