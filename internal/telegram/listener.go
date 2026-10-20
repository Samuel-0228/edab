package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Ingester is the minimal interface the Telegram listener needs to push
// normalized raw posts into the Kerob workflow engine.
type Ingester interface {
	IngestRaw(ctx context.Context, sourceID string, rawData []byte) error
}

// IngesterFunc allows using a plain function as an Ingester.
type IngesterFunc func(ctx context.Context, sourceID string, rawData []byte) error

// IngestRaw implements Ingester.
func (f IngesterFunc) IngestRaw(ctx context.Context, sourceID string, rawData []byte) error {
	return f(ctx, sourceID, rawData)
}

// SourceResolver resolves a Telegram chat ID to an active internal source ID.
// This allows the listener to stay decoupled from the storage layer.
type SourceResolver interface {
	ResolveSource(ctx context.Context, chatID int64) (sourceID string, ok bool, err error)
}

// SourceResolverFunc adapts a function to the SourceResolver interface.
type SourceResolverFunc func(ctx context.Context, chatID int64) (string, bool, error)

// ResolveSource implements SourceResolver.
func (f SourceResolverFunc) ResolveSource(ctx context.Context, chatID int64) (string, bool, error) {
	return f(ctx, chatID)
}

// ListenerOptions configures the Telegram update listener.
type ListenerOptions struct {
	PollTimeout    int           // long-poll timeout in seconds for getUpdates (e.g. 30)
	MediaGroupWait time.Duration // buffering window for albums (default: 1s)
}

// DefaultListenerOptions returns sensible defaults for the listener.
func DefaultListenerOptions() ListenerOptions {
	return ListenerOptions{
		PollTimeout:    30,
		MediaGroupWait: 1 * time.Second,
	}
}

// Listener receives updates from the Telegram Bot API via long-polling,
// filters for registered sources, buffers media groups, and feeds posts
// to Kerob's ingestion pipeline.
type Listener struct {
	api            API
	ingester       Ingester
	sourceResolver SourceResolver
	stateStore     StateStore
	opts           ListenerOptions

	mu          sync.Mutex
	albumBuffer map[string]*albumEntry // key: "chatID:mediaGroupID"
	stopChan    chan struct{}
	wg          sync.WaitGroup
}

type albumEntry struct {
	sourceID     string
	chatID       int64
	chatTitle    string
	chatType     string
	mediaGroupID string
	messages     []json.RawMessage
	timer        *time.Timer
}

// NewListener creates a Telegram listener.
func NewListener(
	api API,
	ingester Ingester,
	sourceResolver SourceResolver,
	stateStore StateStore,
	opts ListenerOptions,
) *Listener {
	if opts.PollTimeout <= 0 {
		opts.PollTimeout = 30
	}
	if opts.MediaGroupWait <= 0 {
		opts.MediaGroupWait = 1 * time.Second
	}
	return &Listener{
		api:            api,
		ingester:       ingester,
		sourceResolver: sourceResolver,
		stateStore:     stateStore,
		opts:           opts,
		albumBuffer:    make(map[string]*albumEntry),
		stopChan:       make(chan struct{}),
	}
}

// Start runs the update listener loop in the background until ctx is cancelled.
func (l *Listener) Start(ctx context.Context) {
	l.wg.Add(1)
	go func() {
		defer l.wg.Done()
		l.run(ctx)
	}()
}

// Stop stops the listener and flushes any pending buffered media groups.
func (l *Listener) Stop() {
	close(l.stopChan)
	l.wg.Wait()
	l.flushAllAlbums(context.Background())
}

func (l *Listener) run(ctx context.Context) {
	slog.Info("[telegram] listener starting",
		slog.Int("poll_timeout", l.opts.PollTimeout),
		slog.Duration("media_group_window", l.opts.MediaGroupWait),
	)

	offset := l.initializeOffset(ctx)
	slog.Info("[telegram] listener initialized offset", slog.Int("offset", offset))

	for {
		select {
		case <-ctx.Done():
			return
		case <-l.stopChan:
			return
		default:
		}

		uCfg := tgbotapi.NewUpdate(offset)
		uCfg.Timeout = l.opts.PollTimeout
		uCfg.AllowedUpdates = []string{"message", "channel_post"}

		updates, err := l.api.GetUpdates(uCfg)
		if err != nil {
			select {
			case <-ctx.Done():
				return
			case <-l.stopChan:
				return
			default:
			}

			if wait, ok := RetryAfter(err); ok {
				slog.Warn("[telegram] rate limited by Telegram, backing off", slog.Int("retry_after_sec", wait))
				select {
				case <-time.After(time.Duration(wait) * time.Second):
				case <-ctx.Done():
					return
				case <-l.stopChan:
					return
				}
				continue
			}

			slog.Error("[telegram] failed to fetch updates", slog.String("error", err.Error()))
			select {
			case <-time.After(2 * time.Second):
			case <-ctx.Done():
				return
			case <-l.stopChan:
				return
			}
			continue
		}

		for _, update := range updates {
			if update.UpdateID >= offset {
				offset = update.UpdateID + 1
			}

			l.processUpdate(ctx, update)

			if l.stateStore != nil {
				if err := l.stateStore.SetLastUpdateID(ctx, update.UpdateID); err != nil {
					slog.Warn("[telegram] failed to persist last update id",
						slog.Int("update_id", update.UpdateID),
						slog.String("error", err.Error()),
					)
				}
			}
		}
	}
}

func (l *Listener) initializeOffset(ctx context.Context) int {
	if l.stateStore != nil {
		lastID, err := l.stateStore.GetLastUpdateID(ctx)
		if err == nil && lastID > 0 {
			return lastID + 1
		}
	}

	primeConfig := tgbotapi.NewUpdate(-1)
	primeConfig.Limit = 1
	primeConfig.Timeout = 0
	updates, err := l.api.GetUpdates(primeConfig)
	if err == nil && len(updates) > 0 {
		latestID := updates[len(updates)-1].UpdateID
		if l.stateStore != nil {
			_ = l.stateStore.SetLastUpdateID(ctx, latestID)
		}
		return latestID + 1
	}

	return 0
}

func (l *Listener) processUpdate(ctx context.Context, u tgbotapi.Update) {
	msg := extractMessage(u)
	if msg == nil {
		return
	}

	slog.Info("[telegram] message received",
		slog.Int64("source", msg.Chat.ID),
		slog.Int("message", msg.MessageID),
		slog.String("chat_title", msg.Chat.Title),
		slog.String("chat_type", msg.Chat.Type),
	)

	var sourceID string
	if l.sourceResolver != nil {
		id, ok, err := l.sourceResolver.ResolveSource(ctx, msg.Chat.ID)
		if err != nil {
			slog.Error("[telegram] error resolving matching source",
				slog.Int64("source", msg.Chat.ID),
				slog.String("error", err.Error()),
			)
			return
		}
		if !ok {
			slog.Debug("[telegram] ignoring message from unmonitored chat",
				slog.Int64("source", msg.Chat.ID),
				slog.Int("message", msg.MessageID),
			)
			return
		}
		sourceID = id
	}

	rawMsg, err := json.Marshal(msg)
	if err != nil {
		slog.Error("[telegram] failed to marshal telegram message", slog.String("error", err.Error()))
		return
	}

	if msg.MediaGroupID != "" {
		l.bufferMediaGroup(ctx, sourceID, msg, rawMsg)
		return
	}

	l.ingestSinglePost(ctx, sourceID, msg, rawMsg)
}

func (l *Listener) ingestSinglePost(ctx context.Context, sourceID string, msg *tgbotapi.Message, rawMsg json.RawMessage) {
	post := RawPost{
		ChatID:       msg.Chat.ID,
		ChatTitle:    msg.Chat.Title,
		ChatType:     msg.Chat.Type,
		MediaGroupID: "",
		Messages:     []json.RawMessage{rawMsg},
	}

	postBytes, err := json.Marshal(post)
	if err != nil {
		slog.Error("[telegram] failed to marshal raw post", slog.String("error", err.Error()))
		return
	}

	if l.ingester != nil {
		if err := l.ingester.IngestRaw(ctx, sourceID, postBytes); err != nil {
			slog.Error("[telegram] ingestion failed",
				slog.String("source_id", sourceID),
				slog.Int64("chat_id", msg.Chat.ID),
				slog.Int("message_id", msg.MessageID),
				slog.String("error", err.Error()),
			)
		}
	}
}

func (l *Listener) bufferMediaGroup(ctx context.Context, sourceID string, msg *tgbotapi.Message, rawMsg json.RawMessage) {
	key := fmt.Sprintf("%d:%s", msg.Chat.ID, msg.MediaGroupID)

	l.mu.Lock()
	defer l.mu.Unlock()

	entry, exists := l.albumBuffer[key]
	if exists {
		entry.messages = append(entry.messages, rawMsg)
		entry.timer.Reset(l.opts.MediaGroupWait)
		return
	}

	entry = &albumEntry{
		sourceID:     sourceID,
		chatID:       msg.Chat.ID,
		chatTitle:    msg.Chat.Title,
		chatType:     msg.Chat.Type,
		mediaGroupID: msg.MediaGroupID,
		messages:     []json.RawMessage{rawMsg},
	}

	entry.timer = time.AfterFunc(l.opts.MediaGroupWait, func() {
		l.flushAlbum(context.Background(), key)
	})

	l.albumBuffer[key] = entry
}

func (l *Listener) flushAlbum(ctx context.Context, key string) {
	l.mu.Lock()
	entry, exists := l.albumBuffer[key]
	if !exists {
		l.mu.Unlock()
		return
	}
	delete(l.albumBuffer, key)
	l.mu.Unlock()

	post := RawPost{
		ChatID:       entry.chatID,
		ChatTitle:    entry.chatTitle,
		ChatType:     entry.chatType,
		MediaGroupID: entry.mediaGroupID,
		Messages:     entry.messages,
	}

	postBytes, err := json.Marshal(post)
	if err != nil {
		slog.Error("[telegram] failed to marshal album post", slog.String("error", err.Error()))
		return
	}

	slog.Info("[telegram] flushing media group album",
		slog.Int64("chat_id", entry.chatID),
		slog.String("media_group_id", entry.mediaGroupID),
		slog.Int("items", len(entry.messages)),
	)

	if l.ingester != nil {
		if err := l.ingester.IngestRaw(ctx, entry.sourceID, postBytes); err != nil {
			slog.Error("[telegram] album ingestion failed",
				slog.String("source_id", entry.sourceID),
				slog.Int64("chat_id", entry.chatID),
				slog.String("media_group_id", entry.mediaGroupID),
				slog.String("error", err.Error()),
			)
		}
	}
}

func (l *Listener) flushAllAlbums(ctx context.Context) {
	l.mu.Lock()
	keys := make([]string, 0, len(l.albumBuffer))
	for k, e := range l.albumBuffer {
		if e.timer != nil {
			e.timer.Stop()
		}
		keys = append(keys, k)
	}
	l.mu.Unlock()

	for _, k := range keys {
		l.flushAlbum(ctx, k)
	}
}

func extractMessage(u tgbotapi.Update) *tgbotapi.Message {
	if u.ChannelPost != nil {
		return u.ChannelPost
	}
	if u.Message != nil {
		return u.Message
	}
	return nil
}
