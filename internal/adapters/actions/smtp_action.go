package actions

import (
	"context"
	"encoding/json"
	"fmt"
	"net/smtp"
	"strings"

	"kerob/internal/core/workflow"
)

// SMTPSender is a function signature compatible with net/smtp.SendMail.
type SMTPSender func(addr string, a smtp.Auth, from string, to []string, msg []byte) error

// SMTPActionConfig defines email dispatch parameters.
type SMTPActionConfig struct {
	Host     string   `json:"host,omitempty"`
	Port     int      `json:"port,omitempty"`
	Username string   `json:"username,omitempty"`
	Password string   `json:"password,omitempty"`
	From     string   `json:"from,omitempty"`
	To       []string `json:"to,omitempty"`
	Subject  string   `json:"subject,omitempty"`
	Body     string   `json:"body,omitempty"`
	IsHTML   bool     `json:"is_html,omitempty"`
}

// SMTPActionExecutor sends email notifications via standard SMTP.
type SMTPActionExecutor struct {
	sender SMTPSender
}

func NewSMTPActionExecutor() *SMTPActionExecutor {
	return NewSMTPActionExecutorWithSender(smtp.SendMail)
}

func NewSMTPActionExecutorWithSender(sender SMTPSender) *SMTPActionExecutor {
	if sender == nil {
		sender = smtp.SendMail
	}
	return &SMTPActionExecutor{sender: sender}
}

func (s *SMTPActionExecutor) Name() string { return "SMTP Email" }
func (s *SMTPActionExecutor) Type() string { return "smtp" }

func (s *SMTPActionExecutor) Validate(config json.RawMessage) error {
	var cfg SMTPActionConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return fmt.Errorf("invalid smtp action config: %w", err)
	}
	if strings.TrimSpace(cfg.From) == "" {
		return fmt.Errorf("smtp action requires 'from' address")
	}
	if len(cfg.To) == 0 {
		return fmt.Errorf("smtp action requires at least one 'to' recipient")
	}
	if strings.TrimSpace(cfg.Subject) == "" {
		return fmt.Errorf("smtp action requires 'subject'")
	}
	return nil
}

func (s *SMTPActionExecutor) Execute(ctx context.Context, req workflow.ActionRequest) (workflow.ActionResult, error) {
	var cfg SMTPActionConfig
	if err := json.Unmarshal(req.Config, &cfg); err != nil {
		return workflow.ActionResult{Success: false}, fmt.Errorf("failed to parse smtp config: %w", err)
	}

	host := cfg.Host
	username := cfg.Username
	password := cfg.Password
	if req.Credentials != nil {
		if h, ok := req.Credentials["host"]; ok && h != "" {
			host = h
		}
		if u, ok := req.Credentials["username"]; ok && u != "" {
			username = u
		}
		if p, ok := req.Credentials["password"]; ok && p != "" {
			password = p
		}
	}

	if host == "" {
		host = "localhost"
	}
	port := cfg.Port
	if port <= 0 {
		port = 587
	}

	addr := fmt.Sprintf("%s:%d", host, port)

	var auth smtp.Auth
	if username != "" && password != "" {
		auth = smtp.PlainAuth("", username, password, host)
	}

	body := cfg.Body
	if body == "" && req.Payload != nil {
		if b, ok := req.Payload["text"].(string); ok {
			body = b
		}
	}

	contentType := "text/plain; charset=UTF-8"
	if cfg.IsHTML {
		contentType = "text/html; charset=UTF-8"
	}

	toHeader := strings.Join(cfg.To, ", ")
	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: %s\r\n\r\n%s",
		cfg.From, toHeader, cfg.Subject, contentType, body)

	errChan := make(chan error, 1)
	go func() {
		errChan <- s.sender(addr, auth, cfg.From, cfg.To, []byte(msg))
	}()

	select {
	case <-ctx.Done():
		return workflow.ActionResult{Success: false}, ctx.Err()
	case err := <-errChan:
		if err != nil {
			return workflow.ActionResult{Success: false}, fmt.Errorf("smtp send failed: %w", err)
		}
	}

	outBytes, _ := json.Marshal(map[string]any{
		"sent_to": cfg.To,
		"subject": cfg.Subject,
	})

	return workflow.ActionResult{
		Success: true,
		Output:  outBytes,
		Metadata: map[string]any{
			"host": host,
			"port": port,
		},
	}, nil
}
