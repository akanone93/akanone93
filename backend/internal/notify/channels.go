// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/mail"
	"net/netip"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/nightCode42/plux3/backend/internal/httpx"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// Kind is the type of a channel.
type Kind string

// The channel kinds (SRV-062).
const (
	KindSMTP     Kind = "smtp"
	KindSlack    Kind = "slack"
	KindTeams    Kind = "teams"
	KindTelegram Kind = "telegram"
)

// Kinds are the channel kinds, in order.
func Kinds() []Kind { return []Kind{KindSMTP, KindSlack, KindTeams, KindTelegram} }

// Message is what a channel sends.
type Message struct {
	Type  EventType
	ID    string
	Title string
	Body  string
	// Link is absolute, or empty.
	Link string
	// To is the recipient's address for mail; chat channels ignore it.
	To string
}

// maxResponse bounds how much of a reply is read; it is never stored.
const maxResponse = 4 << 10

// telegramToken is the shape of a bot token: a number, a colon and the secret.
var telegramToken = regexp.MustCompile(`^[0-9]{3,20}:[A-Za-z0-9_-]{20,}$`)

// telegramChat is a numeric chat ID or an @channel name.
var telegramChat = regexp.MustCompile(`^(-?[0-9]{1,20}|@[A-Za-z][A-Za-z0-9_]{4,31})$`)

// validateChannel checks a channel's settings and secret and returns the
// settings to store. A misconfigured channel is PLX-8400.
func (s *Service) validateChannel(kind Kind, settings map[string]string, secret string) (map[string]string, error) {
	switch kind {
	case KindSlack, KindTeams:
		return s.validateWebhookChannel(kind, settings, secret)
	case KindTelegram:
		return validateTelegram(settings, secret)
	case KindSMTP:
		return s.validateSMTPChannel(settings, secret)
	default:
		return nil, plxerr.New(plxerr.InvalidEnumValue, "%q is not a channel kind; the kinds are smtp, slack, teams and telegram", kind)
	}
}

// misconfigured is the error of a channel whose settings or secret are wrong.
func misconfigured(format string, args ...any) error {
	return plxerr.New(plxerr.ChannelMisconfigured, format, args...)
}

// onlySettings refuses a setting that is not one of keys.
func onlySettings(kind Kind, settings map[string]string, keys ...string) error {
	for k := range settings {
		if !contains(keys, k) {
			return misconfigured("%s channels have no setting %q", kind, k)
		}
	}
	return nil
}

// validateWebhookChannel checks a Slack or Teams channel: no settings, and
// the webhook URL as the secret.
func (s *Service) validateWebhookChannel(kind Kind, settings map[string]string, secret string) (map[string]string, error) {
	if err := onlySettings(kind, settings); err != nil {
		return nil, err
	}
	if err := s.checkWebhookURL(secret); err != nil {
		return nil, misconfigured("the %s webhook URL: %v", kind, err)
	}
	return map[string]string{}, nil
}

// validateTelegram checks a Telegram channel: the chat and the bot token.
func validateTelegram(settings map[string]string, secret string) (map[string]string, error) {
	if err := onlySettings(KindTelegram, settings, "chat_id"); err != nil {
		return nil, err
	}
	if !telegramChat.MatchString(settings["chat_id"]) {
		return nil, misconfigured("chat_id is a number or an @channel name")
	}
	if !telegramToken.MatchString(secret) {
		return nil, misconfigured("the bot token looks like 123456:ABC-DEF, as @BotFather gives it")
	}
	return map[string]string{"chat_id": settings["chat_id"]}, nil
}

// validateSMTPChannel checks an SMTP channel: the server settings, and the
// password as the secret when there is a username.
func (s *Service) validateSMTPChannel(settings map[string]string, secret string) (map[string]string, error) {
	cfg, err := parseSMTP(settings, s.o.AllowInsecure)
	if err != nil {
		return nil, err
	}
	if cfg.Username != "" && secret == "" {
		return nil, misconfigured("a username needs the password as the secret")
	}
	out := map[string]string{"host": cfg.Host, "port": strconv.Itoa(cfg.Port), "from": cfg.From, "tls": string(cfg.TLS)}
	if cfg.Username != "" {
		out["username"] = cfg.Username
	}
	return out, nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// checkWebhookURL checks a destination URL: https (http only where the
// installation allows insecure development settings), a host, no credentials,
// and not an address the SSRF policy refuses. The dialer checks again at
// connection time against the resolved address (SEC-105).
func (s *Service) checkWebhookURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil || u.Fragment != "" || len(raw) > 2048 {
		return errors.New("it is not a valid URL")
	}
	if u.Scheme != "https" && (u.Scheme != "http" || !s.o.AllowInsecure) {
		return errors.New("it must be an https URL")
	}
	if err := httpx.CheckURL(u.Scheme, u.Hostname()); err != nil {
		return errors.New("its host is not allowed")
	}
	if addr, perr := netip.ParseAddr(u.Hostname()); perr == nil {
		if err := s.o.Policy.Check(addr); err != nil {
			return errors.New("its address is in a reserved range")
		}
	}
	return nil
}

// absolute turns a Studio path into a URL; an https URL stays.
func (s *Service) absolute(link string) string {
	switch {
	case strings.HasPrefix(link, "https://"):
		return link
	case strings.HasPrefix(link, "/") && s.o.StudioURL != "":
		return strings.TrimRight(s.o.StudioURL, "/") + link
	}
	return ""
}

// send delivers a message through a channel. The status is the HTTP status
// where there was one. Errors are free of secrets.
func (s *Service) send(ctx context.Context, kind Kind, settings map[string]string, secret string, m Message) (int, error) {
	switch kind {
	case KindSlack:
		return s.post(ctx, secret, slackBody(m), secret)
	case KindTeams:
		return s.post(ctx, secret, teamsBody(m), secret)
	case KindTelegram:
		body, err := telegramBody(settings["chat_id"], m)
		if err != nil {
			return 0, err
		}
		return s.post(ctx, strings.TrimRight(s.telegramBase(), "/")+"/bot"+secret+"/sendMessage", body, secret)
	case KindSMTP:
		cfg, err := parseSMTP(settings, s.o.AllowInsecure)
		if err != nil {
			return 0, err
		}
		return 0, s.sendMail(ctx, cfg, secret, m)
	}
	return 0, plxerr.New(plxerr.InvalidEnumValue, "%q is not a channel kind", kind)
}

func (s *Service) telegramBase() string {
	if s.o.TelegramBase != "" {
		return s.o.TelegramBase
	}
	return "https://api.telegram.org"
}

// client returns the SSRF-safe client without redirects: a redirect
// would carry the secret in the URL to a place nobody configured.
func (s *Service) client() *http.Client {
	c := *s.o.HTTP
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &c
}

// post sends a JSON body. secrets are scrubbed from any error text.
func (s *Service) post(ctx context.Context, target string, body []byte, secrets ...string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return 0, clean(err, secrets...)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "plux-server")
	return s.do(req, secrets...)
}

// do runs a request and reports the status; a non-2xx status is an error.
func (s *Service) do(req *http.Request, secrets ...string) (int, error) {
	if err := httpx.CheckURL(req.URL.Scheme, req.URL.Hostname()); err != nil {
		return 0, clean(err, secrets...)
	}
	resp, err := s.client().Do(req)
	if err != nil {
		return 0, clean(err, secrets...)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponse))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return resp.StatusCode, fmt.Errorf("the service answered %d", resp.StatusCode)
	}
	return resp.StatusCode, nil
}

// clean removes what identifies a destination from an error: net/http puts
// the URL, which holds a webhook's secret, in its errors.
func clean(err error, secrets ...string) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		err = ue.Err
	}
	msg := err.Error()
	for _, sec := range secrets {
		if sec != "" {
			msg = strings.ReplaceAll(msg, sec, "***")
		}
	}
	if len(msg) > 200 {
		msg = msg[:200]
	}
	return errors.New(msg)
}

// slackBody formats a message for a Slack incoming webhook.
func slackBody(m Message) []byte {
	esc := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace
	text := "*" + esc(m.Title) + "*"
	if m.Body != "" {
		text += "\n" + esc(m.Body)
	}
	if m.Link != "" {
		text += "\n<" + strings.NewReplacer("<", "%3C", ">", "%3E", "|", "%7C").Replace(m.Link) + "|Open in Plux>"
	}
	b, _ := json.Marshal(map[string]string{"text": text})
	return b
}

// teamsBody formats a message as an Adaptive Card for a Teams workflow
// webhook.
func teamsBody(m Message) []byte {
	card := map[string]any{
		"$schema": "http://adaptivecards.io/schemas/adaptive-card.json",
		"type":    "AdaptiveCard",
		"version": "1.4",
		"body": []any{
			map[string]any{"type": "TextBlock", "text": m.Title, "weight": "Bolder", "size": "Medium", "wrap": true},
			map[string]any{"type": "TextBlock", "text": m.Body, "wrap": true},
		},
	}
	if m.Link != "" {
		card["actions"] = []any{map[string]any{"type": "Action.OpenUrl", "title": "Open in Plux", "url": m.Link}}
	}
	b, _ := json.Marshal(map[string]any{
		"type":        "message",
		"attachments": []any{map[string]any{"contentType": "application/vnd.microsoft.card.adaptive", "content": card}},
	})
	return b
}

// telegramBody formats a message for the Bot API's sendMessage.
func telegramBody(chat string, m Message) ([]byte, error) {
	text := "<b>" + html.EscapeString(m.Title) + "</b>"
	if m.Body != "" {
		text += "\n" + html.EscapeString(m.Body)
	}
	if m.Link != "" {
		text += "\n<a href=\"" + html.EscapeString(m.Link) + "\">Open in Plux</a>"
	}
	b, err := json.Marshal(map[string]any{"chat_id": chat, "text": text, "parse_mode": "HTML", "disable_web_page_preview": true})
	if err != nil {
		return nil, fmt.Errorf("notify: %w", err)
	}
	return b, nil
}

// parseAddress reads a mail address and refuses anything with a line break,
// which would let a value inject headers.
func parseAddress(s string) (*mail.Address, error) {
	if strings.ContainsAny(s, "\r\n") {
		return nil, errors.New("an address has no line breaks")
	}
	a, err := mail.ParseAddress(s)
	if err != nil {
		return nil, fmt.Errorf("%q is not an address", s)
	}
	return a, nil
}

// hostOK checks a host name or address for the SMTP relay.
func hostOK(h string) bool {
	if h == "" || len(h) > 253 || strings.ContainsAny(h, " \r\n/@") {
		return false
	}
	_, _, err := net.SplitHostPort(net.JoinHostPort(h, "1"))
	return err == nil
}
