package telegram

import (
	"sync"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// MockAPI is an in-memory implementation of the telegram.API interface.
type MockAPI struct {
	mu sync.Mutex

	GetMeFunc          func() (tgbotapi.User, error)
	GetUpdatesFunc     func(config tgbotapi.UpdateConfig) ([]tgbotapi.Update, error)
	GetChatFunc        func(config tgbotapi.ChatInfoConfig) (tgbotapi.Chat, error)
	GetChatMemberFunc  func(config tgbotapi.GetChatMemberConfig) (tgbotapi.ChatMember, error)
	CopyMessageFunc    func(config tgbotapi.CopyMessageConfig) (tgbotapi.MessageID, error)
	CopyMessagesFunc   func(fromChatID, toChatID int64, messageIDs []int) ([]int, error)
	SendFunc           func(c tgbotapi.Chattable) (tgbotapi.Message, error)
	SendMediaGroupFunc func(config tgbotapi.MediaGroupConfig) ([]tgbotapi.Message, error)

	CopiedMessages  []tgbotapi.CopyMessageConfig
	CopiedAlbums    []CopiedAlbumRecord
	SentChattables  []tgbotapi.Chattable
	SentMediaGroups []tgbotapi.MediaGroupConfig

	NextMessageID int
}

// CopiedAlbumRecord captures parameters passed to CopyMessages.
type CopiedAlbumRecord struct {
	FromChatID int64
	ToChatID   int64
	MessageIDs []int
}

// NewMockAPI creates a new MockAPI with standard default behaviors.
func NewMockAPI() *MockAPI {
	return &MockAPI{
		NextMessageID: 1000,
	}
}

func (m *MockAPI) GetMe() (tgbotapi.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.GetMeFunc != nil {
		return m.GetMeFunc()
	}
	return tgbotapi.User{
		ID:        999999,
		IsBot:     true,
		FirstName: "KerobBot",
		UserName:  "kerob_mirror_bot",
	}, nil
}

func (m *MockAPI) GetUpdates(config tgbotapi.UpdateConfig) ([]tgbotapi.Update, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.GetUpdatesFunc != nil {
		return m.GetUpdatesFunc(config)
	}
	return nil, nil
}

func (m *MockAPI) GetChat(config tgbotapi.ChatInfoConfig) (tgbotapi.Chat, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.GetChatFunc != nil {
		return m.GetChatFunc(config)
	}
	return tgbotapi.Chat{
		ID:    config.ChatID,
		Title: "Test Chat",
		Type:  "channel",
	}, nil
}

func (m *MockAPI) GetChatMember(config tgbotapi.GetChatMemberConfig) (tgbotapi.ChatMember, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.GetChatMemberFunc != nil {
		return m.GetChatMemberFunc(config)
	}
	return tgbotapi.ChatMember{
		Status: "administrator",
	}, nil
}

func (m *MockAPI) CopyMessage(config tgbotapi.CopyMessageConfig) (tgbotapi.MessageID, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.CopyMessageFunc != nil {
		return m.CopyMessageFunc(config)
	}
	m.CopiedMessages = append(m.CopiedMessages, config)
	m.NextMessageID++
	return tgbotapi.MessageID{MessageID: m.NextMessageID}, nil
}

func (m *MockAPI) CopyMessages(fromChatID, toChatID int64, messageIDs []int) ([]int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.CopyMessagesFunc != nil {
		return m.CopyMessagesFunc(fromChatID, toChatID, messageIDs)
	}
	m.CopiedAlbums = append(m.CopiedAlbums, CopiedAlbumRecord{
		FromChatID: fromChatID,
		ToChatID:   toChatID,
		MessageIDs: messageIDs,
	})
	out := make([]int, len(messageIDs))
	for i := range messageIDs {
		m.NextMessageID++
		out[i] = m.NextMessageID
	}
	return out, nil
}

func (m *MockAPI) Send(c tgbotapi.Chattable) (tgbotapi.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.SendFunc != nil {
		return m.SendFunc(c)
	}
	m.SentChattables = append(m.SentChattables, c)
	m.NextMessageID++
	return tgbotapi.Message{
		MessageID: m.NextMessageID,
	}, nil
}

func (m *MockAPI) SendMediaGroup(config tgbotapi.MediaGroupConfig) ([]tgbotapi.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.SendMediaGroupFunc != nil {
		return m.SendMediaGroupFunc(config)
	}
	m.SentMediaGroups = append(m.SentMediaGroups, config)
	var out []tgbotapi.Message
	// Mock returns one message per item in the media group
	count := 1
	if len(config.Media) > 0 {
		count = len(config.Media)
	}
	for i := 0; i < count; i++ {
		m.NextMessageID++
		out = append(out, tgbotapi.Message{MessageID: m.NextMessageID})
	}
	return out, nil
}
