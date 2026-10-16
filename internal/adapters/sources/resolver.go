package sources

import (
	"context"
	"encoding/json"

	"automation-engine/internal/telegram"
)

// NewSourceResolver returns a telegram.SourceResolver that checks a sources.Store
// for an enabled Telegram source matching the given chat ID.
func NewSourceResolver(store Store) telegram.SourceResolver {
	return telegram.SourceResolverFunc(func(ctx context.Context, chatID int64) (string, bool, error) {
		all, err := store.List(ctx)
		if err != nil {
			return "", false, err
		}
		for _, s := range all {
			if !s.Enabled || s.AdapterType != "telegram" {
				continue
			}
			var cfg TelegramSourceConfig
			if err := json.Unmarshal(s.Config, &cfg); err != nil {
				continue
			}
			parsedID, err := telegram.ParseChatID(cfg.TelegramChatID)
			if err != nil {
				continue
			}
			if parsedID == chatID {
				return s.ID, true, nil
			}
		}
		return "", false, nil
	})
}

// NewPublicSourceResolver returns a resolver func that lists all enabled telegram_public sources.
func NewPublicSourceResolver(store Store) func(ctx context.Context) ([]telegram.PublicSourceInfo, error) {
	return func(ctx context.Context) ([]telegram.PublicSourceInfo, error) {
		all, err := store.List(ctx)
		if err != nil {
			return nil, err
		}
		var out []telegram.PublicSourceInfo
		for _, s := range all {
			if !s.Enabled || s.AdapterType != "telegram_public" {
				continue
			}
			var cfg TelegramPublicSourceConfig
			if err := json.Unmarshal(s.Config, &cfg); err != nil {
				continue
			}
			ch := telegram.NormalizeChannelName(cfg.ChannelUsername)
			if ch != "" {
				out = append(out, telegram.PublicSourceInfo{
					SourceID: s.ID,
					Channel:  ch,
				})
			}
		}
		return out, nil
	}
}
