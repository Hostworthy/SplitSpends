// Package notifications implements ports.Notifier over SMTP. Without SMTP
// settings it logs messages instead, which is handy in development.
package notifications

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"log/slog"
	"strings"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/core/ports"
	"github.com/wneessen/go-mail"
)

// Config holds SMTP settings.
type Config struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	// Security is "starttls" (default), "tls" or "none".
	Security string
	AppName  string
	BaseURL  string
	// Sync sends every email before returning, for serverless hosts that
	// freeze the process between requests.
	Sync bool
}

// Sender delivers one message.
type Sender interface {
	Send(ctx context.Context, to, subject, text, html string) error
}

// Notifier renders and delivers emails through a bounded background queue,
// or before returning when Sync is set.
type Notifier struct {
	sender Sender
	cfg    Config
	log    *slog.Logger
	queue  chan ports.Notification
}

// New builds a Notifier and starts its worker. Call Close on shutdown.
func New(cfg Config, log *slog.Logger) (*Notifier, error) {
	var sender Sender
	if cfg.Host == "" {
		log.Warn("notifications.smtp.disabled", "reason", "SMTP_HOST is empty; emails are written to the log")
		sender = logSender{log: log}
	} else {
		s, err := newSMTP(cfg)
		if err != nil {
			return nil, err
		}
		sender = s
	}
	n := &Notifier{sender: sender, cfg: cfg, log: log, queue: make(chan ports.Notification, 256)}
	go n.work()
	return n, nil
}

// Close stops the worker after draining queued messages.
func (n *Notifier) Close() { close(n.queue) }

func (n *Notifier) work() {
	for msg := range n.queue {
		n.deliver(msg)
	}
}

func (n *Notifier) deliver(msg ports.Notification) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	text, html := n.render(msg.Name, msg.Lines, msg.Link, "Open "+n.cfg.AppName)
	if err := n.sender.Send(ctx, msg.To, msg.Subject, text, html); err != nil {
		n.log.Error("notifications.send.failed", about(msg), "error", err)
	}
}

// loginCode is the kind of login code emails. Their subject holds the code,
// so it is never logged.
const loginCode = "loginCode"

func about(msg ports.Notification) slog.Attr {
	if msg.Kind == loginCode {
		return slog.String("kind", msg.Kind)
	}
	return slog.String("subject", msg.Subject)
}

// LoginCode queues the code like any other email. Sign-in answers the same
// way and as quickly whether or not a code is sent, so a slow or failing
// mail server must not show in the response; failures are logged.
func (n *Notifier) LoginCode(ctx context.Context, to, code string, ttl time.Duration) error {
	lines := []string{"Your " + n.cfg.AppName + " sign-in code is:", code,
		fmt.Sprintf("It expires in %d minutes. If you did not ask for it, you can ignore this email.", int(ttl.Minutes()))}
	n.Notify(ctx, ports.Notification{To: to, Kind: loginCode, Subject: code + " is your " + n.cfg.AppName + " code", Lines: lines})
	return nil
}

func (n *Notifier) Invite(ctx context.Context, to, inviter string) error {
	lines := []string{inviter + " invited you to split expenses on " + n.cfg.AppName + ".",
		"Sign in with this email address to see what you share."}
	n.Notify(ctx, ports.Notification{To: to, Subject: "You are invited to " + n.cfg.AppName, Lines: lines, Link: n.cfg.BaseURL})
	return nil
}

func (n *Notifier) Notify(_ context.Context, msg ports.Notification) {
	if msg.To == "" {
		return
	}
	if n.cfg.Sync {
		n.deliver(msg)
		return
	}
	select {
	case n.queue <- msg:
	default:
		n.log.Warn("notifications.queue.full", about(msg))
	}
}

var page = template.Must(template.New("mail").Parse(`<!doctype html><html><body style="margin:0;background:#f4f7f6;font-family:-apple-system,Segoe UI,Roboto,Helvetica,Arial,sans-serif;color:#1b2b28">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0"><tr><td align="center" style="padding:32px 16px">
<table role="presentation" width="100%" style="max-width:520px;background:#ffffff;border-radius:16px;padding:32px" cellpadding="0" cellspacing="0">
<tr><td style="font-size:20px;font-weight:700;color:#00796B;padding-bottom:16px">{{.App}}</td></tr>
{{if .Name}}<tr><td style="padding-bottom:12px">Hi {{.Name}},</td></tr>{{end}}
{{range .Lines}}<tr><td style="padding-bottom:12px;font-size:15px;line-height:1.5">{{if $.IsCode .}}<span style="font-size:32px;letter-spacing:8px;font-weight:700">{{.}}</span>{{else}}{{.}}{{end}}</td></tr>{{end}}
{{if .Link}}<tr><td style="padding-top:8px"><a href="{{.Link}}" style="background:#00897B;color:#ffffff;text-decoration:none;padding:12px 20px;border-radius:10px;display:inline-block;font-weight:600">{{.Action}}</a></td></tr>{{end}}
</table></td></tr></table></body></html>`))

func (n *Notifier) render(name string, lines []string, link, action string) (string, string) {
	var text strings.Builder
	if name != "" {
		text.WriteString("Hi " + name + ",\n\n")
	}
	for _, l := range lines {
		text.WriteString(l + "\n\n")
	}
	if link != "" {
		text.WriteString(link + "\n")
	}
	var html bytes.Buffer
	_ = page.Execute(&html, map[string]any{
		"App": n.cfg.AppName, "Name": name, "Lines": lines, "Link": link, "Action": action,
		"IsCode": func(s string) bool { return len(s) == 6 && strings.Trim(s, "0123456789") == "" },
	})
	return text.String(), html.String()
}

type logSender struct{ log *slog.Logger }

func (l logSender) Send(_ context.Context, to, subject, text, _ string) error {
	l.log.Info("notifications.email.logged", "to", to, "subject", subject, "body", text)
	return nil
}

type smtpSender struct {
	cfg    Config
	client *mail.Client
}

func newSMTP(cfg Config) (*smtpSender, error) {
	opts := []mail.Option{mail.WithPort(cfg.Port), mail.WithTimeout(20 * time.Second)}
	switch strings.ToLower(cfg.Security) {
	case "tls", "ssl":
		opts = append(opts, mail.WithSSL())
	case "none":
		opts = append(opts, mail.WithTLSPolicy(mail.NoTLS))
	default:
		opts = append(opts, mail.WithTLSPolicy(mail.TLSMandatory))
	}
	if cfg.Username != "" {
		opts = append(opts, mail.WithSMTPAuth(mail.SMTPAuthAutoDiscover), mail.WithUsername(cfg.Username), mail.WithPassword(cfg.Password))
	}
	client, err := mail.NewClient(cfg.Host, opts...)
	if err != nil {
		return nil, fmt.Errorf("notifications.smtp.config.invalid: %w", err)
	}
	return &smtpSender{cfg: cfg, client: client}, nil
}

func (s *smtpSender) Send(ctx context.Context, to, subject, text, html string) error {
	m := mail.NewMsg()
	if err := m.From(s.cfg.From); err != nil {
		return fmt.Errorf("notifications.smtp.from.invalid: %w", err)
	}
	if err := m.To(to); err != nil {
		return fmt.Errorf("notifications.smtp.to.invalid: %w", err)
	}
	m.Subject(subject)
	m.SetBodyString(mail.TypeTextPlain, text)
	m.AddAlternativeString(mail.TypeTextHTML, html)
	return s.client.DialAndSendWithContext(ctx, m)
}
