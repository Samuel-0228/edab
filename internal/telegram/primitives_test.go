package telegram

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func TestRateLimiter(t *testing.T) {
	ctx := context.Background()

	// 1. Nil limiter safe
	var nilLimiter *RateLimiter
	if err := nilLimiter.Wait(ctx, 100); err != nil {
		t.Errorf("expected nil error on nil limiter, got %v", err)
	}

	// 2. Active limiter
	limiter := NewRateLimiter(50.0, 20.0)
	if err := limiter.Wait(ctx, 100); err != nil {
		t.Fatalf("unexpected limiter error: %v", err)
	}
	if err := limiter.Wait(ctx, 100); err != nil {
		t.Fatalf("unexpected limiter error on 2nd wait: %v", err)
	}

	// 3. Cancelled context
	cancCtx, cancel := context.WithCancel(ctx)
	cancel()
	limiterSlow := NewRateLimiter(0.1, 0.1) // 1 request per 10s
	_ = limiterSlow.Wait(ctx, 200)          // prime lastGlobal
	err := limiterSlow.Wait(cancCtx, 200)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}

	// 4. Rate <= 0 disables limit
	if d := intervalFromRate(0); d != 0 {
		t.Errorf("expected 0 duration for 0 rate, got %v", d)
	}
	if d := intervalFromRate(-5); d != 0 {
		t.Errorf("expected 0 duration for negative rate, got %v", d)
	}
	if d := intervalFromRate(10); d != 100*time.Millisecond {
		t.Errorf("expected 100ms for 10/s, got %v", d)
	}
}

func TestPayloadAndChatIDParsing(t *testing.T) {
	// 1. ParseChatID
	id, err := ParseChatID("-100123456789")
	if err != nil || id != -100123456789 {
		t.Errorf("expected -100123456789, got %d, err: %v", id, err)
	}

	_, err = ParseChatID("@my_channel")
	if err == nil {
		t.Errorf("expected error parsing username as integer chat ID")
	}

	_, err = ParseChatID("")
	if err == nil {
		t.Errorf("expected error parsing empty chat ID")
	}

	// 2. ExternalEventID
	extID := ExternalEventID(-100555, "mg-42", 101)
	if extID != "telegram:-100555:mg:mg-42" {
		t.Errorf("expected media group external event ID, got %s", extID)
	}

	singleID := ExternalEventID(-100555, "", 202)
	if singleID != "telegram:-100555:202" {
		t.Errorf("expected single message ID, got %s", singleID)
	}

	// 3. ExternalPublicEventID
	pubID := ExternalPublicEventID("golang_news", 555)
	if pubID != "telegram:public:golang_news:555" {
		t.Errorf("expected telegram:public:golang_news:555, got %s", pubID)
	}

	// 4. MediaKind and ExtractMediaRefs
	msgPhoto := tgbotapi.Message{
		Photo: []tgbotapi.PhotoSize{
			{FileID: "photo-small"},
			{FileID: "photo-large"},
		},
	}
	if kind := MediaKind(&msgPhoto); kind != "photo" {
		t.Errorf("expected media kind photo, got %s", kind)
	}
	refs := ExtractMediaRefs([]*tgbotapi.Message{&msgPhoto})
	if len(refs) != 1 || refs[0].FileID != "photo-large" {
		t.Errorf("expected photo-large ref, got %+v", refs)
	}

	msgVideo := tgbotapi.Message{
		Video: &tgbotapi.Video{FileID: "vid-123"},
	}
	if kind := MediaKind(&msgVideo); kind != "video" {
		t.Errorf("expected video, got %s", kind)
	}

	msgDoc := tgbotapi.Message{
		Document: &tgbotapi.Document{FileID: "doc-123"},
	}
	if kind := MediaKind(&msgDoc); kind != "document" {
		t.Errorf("expected document, got %s", kind)
	}

	msgAudio := tgbotapi.Message{
		Audio: &tgbotapi.Audio{FileID: "aud-123"},
	}
	if kind := MediaKind(&msgAudio); kind != "audio" {
		t.Errorf("expected audio, got %s", kind)
	}

	msgAnim := tgbotapi.Message{
		Animation: &tgbotapi.Animation{FileID: "anim-123"},
	}
	if kind := MediaKind(&msgAnim); kind != "animation" {
		t.Errorf("expected animation, got %s", kind)
	}

	msgVoice := tgbotapi.Message{
		Voice: &tgbotapi.Voice{FileID: "voice-123"},
	}
	if kind := MediaKind(&msgVoice); kind != "voice" {
		t.Errorf("expected voice, got %s", kind)
	}

	msgSticker := tgbotapi.Message{
		Sticker: &tgbotapi.Sticker{FileID: "sticker-123"},
	}
	if kind := MediaKind(&msgSticker); kind != "sticker" {
		t.Errorf("expected sticker, got %s", kind)
	}

	msgNone := tgbotapi.Message{Text: "Plain text"}
	if kind := MediaKind(&msgNone); kind != "" {
		t.Errorf("expected empty media kind for plain text, got %s", kind)
	}
	if kind := MediaKind(nil); kind != "" {
		t.Errorf("expected empty media kind for nil, got %s", kind)
	}
}

func TestMemoryStateStore(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStateStore()

	// Update ID
	id, err := store.GetLastUpdateID(ctx)
	if err != nil || id != 0 {
		t.Errorf("expected initial 0 update ID, got %d", id)
	}

	if err := store.SetLastUpdateID(ctx, 105); err != nil {
		t.Fatalf("SetLastUpdateID failed: %v", err)
	}
	id, _ = store.GetLastUpdateID(ctx)
	if id != 105 {
		t.Errorf("expected 105, got %d", id)
	}

	// Message ID
	msgID, err := store.GetLastMessageID(ctx, "channel-x")
	if err != nil || msgID != 0 {
		t.Errorf("expected initial 0 msg ID, got %d", msgID)
	}
	if err := store.SetLastMessageID(ctx, "channel-x", 999); err != nil {
		t.Fatalf("SetLastMessageID failed: %v", err)
	}
	msgID, _ = store.GetLastMessageID(ctx, "channel-x")
	if msgID != 999 {
		t.Errorf("expected 999, got %d", msgID)
	}
}

func TestMockAPIFull(t *testing.T) {
	api := NewMockAPI()

	// GetMe
	me, err := api.GetMe()
	if err != nil || me.UserName != "kerob_mirror_bot" {
		t.Errorf("unexpected GetMe: %+v, err: %v", me, err)
	}

	// GetChat
	chat, err := api.GetChat(tgbotapi.ChatInfoConfig{ChatConfig: tgbotapi.ChatConfig{ChatID: -100123}})
	if err != nil || chat.Title != "Test Chat" {
		t.Errorf("unexpected GetChat: %+v, err: %v", chat, err)
	}

	// GetChatMember
	member, err := api.GetChatMember(tgbotapi.GetChatMemberConfig{ChatConfigWithUser: tgbotapi.ChatConfigWithUser{ChatID: -100123, UserID: 777}})
	if err != nil || member.Status != "administrator" {
		t.Errorf("unexpected GetChatMember: %+v, err: %v", member, err)
	}

	// CopyMessage
	copyCfg := tgbotapi.CopyMessageConfig{
		BaseChat:   tgbotapi.BaseChat{ChatID: -100222},
		FromChatID: -100111,
		MessageID:  42,
	}
	msgID, err := api.CopyMessage(copyCfg)
	if err != nil || msgID.MessageID == 0 {
		t.Errorf("unexpected CopyMessage result: %+v, err: %v", msgID, err)
	}

	// CopyMessages
	msgIDs, err := api.CopyMessages(-100111, -100222, []int{1, 2, 3})
	if err != nil || len(msgIDs) != 3 {
		t.Errorf("unexpected CopyMessages result: %+v, err: %v", msgIDs, err)
	}

	// Send
	sent, err := api.Send(tgbotapi.NewMessage(-100222, "hello mock"))
	if err != nil || sent.MessageID == 0 {
		t.Errorf("unexpected Send result: %+v, err: %v", sent, err)
	}

	// SendMediaGroup
	groupMsgs, err := api.SendMediaGroup(tgbotapi.NewMediaGroup(-100222, []any{
		tgbotapi.NewInputMediaPhoto(tgbotapi.FilePath("fake.jpg")),
	}))
	if err != nil || len(groupMsgs) == 0 {
		t.Errorf("unexpected SendMediaGroup result: %+v, err: %v", groupMsgs, err)
	}
}

func TestClientHelpers(t *testing.T) {
	// 1. envFloat
	_ = os.Unsetenv("TEST_FLOAT_VAR")
	if val := envFloat("TEST_FLOAT_VAR", 3.14); val != 3.14 {
		t.Errorf("expected default 3.14, got %f", val)
	}

	_ = os.Setenv("TEST_FLOAT_VAR", "9.5")
	defer func() { _ = os.Unsetenv("TEST_FLOAT_VAR") }()
	if val := envFloat("TEST_FLOAT_VAR", 3.14); val != 9.5 {
		t.Errorf("expected 9.5, got %f", val)
	}

	// 2. RetryAfter
	if sec, ok := RetryAfter(nil); ok || sec != 0 {
		t.Errorf("expected no retry after on nil error")
	}

	err429 := tgbotapi.Error{
		Code:               429,
		Message:            "Too Many Requests: retry after 12",
		ResponseParameters: tgbotapi.ResponseParameters{RetryAfter: 12},
	}
	if sec, ok := RetryAfter(err429); !ok || sec != 12 {
		t.Errorf("expected 12s retry after, got %v (ok=%v)", sec, ok)
	}

	// 3. ClientOptionsFromEnv
	opts := ClientOptionsFromEnv()
	if opts.GlobalRate <= 0 {
		t.Errorf("expected positive default GlobalRate, got %f", opts.GlobalRate)
	}

	// 4. LogDeliveryResult safe call
	LogDeliveryResult(-100111, 42, -100222, []int{10, 11}, nil)
	LogDeliveryResult(-100111, 42, -100222, nil, errors.New("network timeout"))

	// 5. CheckAccess
	mockAPI := NewMockAPI()
	access := CheckAccess(mockAPI, -100123)
	if !access.Accessible || access.Status != "administrator" {
		t.Errorf("expected accessible chat with administrator status, got %+v", access)
	}
}
