// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package notify

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"github.com/nightCode42/plux3/backend/internal/httpx"
	"github.com/nightCode42/plux3/backend/internal/plxerr"
)

// TLSMode is how a connection to the SMTP relay is secured.
type TLSMode string

// The modes. STARTTLS is the default and implicit TLS is for port 465;
// "none" sends everything, the password included, in the clear, so it is
// accepted only where the installation allows insecure development
// settings (Options.AllowInsecure) and each use is logged as a warning.
const (
	TLSStartTLS TLSMode = "starttls"
	TLSImplicit TLSMode = "implicit"
	TLSNone     TLSMode = "none"
)

// smtpConfig is the non-secret part of an SMTP channel.
type smtpConfig struct {
	Host     string
	Port     int
	From     string
	Username string
	TLS      TLSMode
}

// parseSMTP reads and validates the settings of an SMTP channel.
func parseSMTP(settings map[string]string, allowInsecure bool) (smtpConfig, error) {
	bad := func(format string, args ...any) error {
		return plxerr.New(plxerr.ChannelMisconfigured, format, args...)
	}
	for k := range settings {
		switch k {
		case "host", "port", "from", "username", "tls":
		default:
			return smtpConfig{}, bad("smtp channels have no setting %q", k)
		}
	}
	c := smtpConfig{Host: settings["host"], Username: settings["username"], TLS: TLSMode(settings["tls"]), Port: 587}
	if !hostOK(c.Host) {
		return smtpConfig{}, bad("host names the SMTP relay")
	}
	if p := settings["port"]; p != "" {
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 || n > 65535 {
			return smtpConfig{}, bad("port is a number from 1 to 65535")
		}
		c.Port = n
	}
	from, err := parseAddress(settings["from"])
	if err != nil {
		return smtpConfig{}, bad("from: %v", err)
	}
	c.From = from.String()
	if strings.ContainsAny(c.Username, "\r\n") {
		return smtpConfig{}, bad("a username has no line breaks")
	}
	switch c.TLS {
	case "":
		c.TLS = TLSStartTLS
		if c.Port == 465 {
			c.TLS = TLSImplicit
		}
	case TLSStartTLS, TLSImplicit:
	case TLSNone:
		if !allowInsecure {
			return smtpConfig{}, bad("tls = none sends mail and the password unencrypted; it is refused unless the installation allows insecure development settings")
		}
	default:
		return smtpConfig{}, bad("tls is starttls, implicit or none")
	}
	return c, nil
}

// smtpTimeout bounds a whole mail transaction.
const smtpTimeout = 30 * time.Second

// dial connects to the relay. The default dialer applies the SSRF policy to
// the address the name resolved to, as the HTTP transport does (SEC-105).
func (s *Service) dial(ctx context.Context, address string) (net.Conn, error) {
	if s.o.SMTPDial != nil {
		return s.o.SMTPDial(ctx, "tcp", address)
	}
	d, err := httpx.GuardedDialer(httpx.KindSMTP, s.o.Policy)
	if err != nil {
		return nil, fmt.Errorf("notify: %w", err)
	}
	return d.DialContext(ctx, "tcp", address)
}

// sendMail sends one message. STARTTLS is required: a relay that does not
// offer it is refused rather than used in the clear (SEC-092).
func (s *Service) sendMail(ctx context.Context, cfg smtpConfig, password string, m Message) error {
	to, err := parseAddress(m.To)
	if err != nil {
		return plxerr.New(plxerr.ChannelMisconfigured, "the recipient: %v", err)
	}
	tlsConfig := &tls.Config{ServerName: cfg.Host, MinVersion: tls.VersionTLS12}
	if s.o.SMTPTLS != nil {
		tlsConfig = s.o.SMTPTLS.Clone()
		tlsConfig.ServerName = cfg.Host
	}
	conn, err := s.dial(ctx, net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port)))
	if err != nil {
		return clean(err, password)
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(smtpTimeout))
	if cfg.TLS == TLSImplicit {
		conn = tls.Client(conn, tlsConfig)
	}
	c, err := smtp.NewClient(conn, cfg.Host)
	if err != nil {
		return clean(err, password)
	}
	defer func() { _ = c.Close() }()
	switch cfg.TLS {
	case TLSStartTLS:
		if ok, _ := c.Extension("STARTTLS"); !ok {
			return errors.New("the relay does not offer STARTTLS, and mail is not sent unencrypted")
		}
		if err := c.StartTLS(tlsConfig); err != nil {
			return clean(err, password)
		}
	case TLSNone:
		s.warn(ctx, "sending mail without TLS because the installation allows insecure development settings", "host", cfg.Host)
	}
	if cfg.Username != "" {
		if err := c.Auth(plainAuth{user: cfg.Username, password: password}); err != nil {
			return clean(err, password)
		}
	}
	from, err := parseAddress(cfg.From)
	if err != nil {
		return clean(err)
	}
	if err := c.Mail(from.Address); err != nil {
		return clean(err, password)
	}
	if err := c.Rcpt(to.Address); err != nil {
		return clean(err, password)
	}
	w, err := c.Data()
	if err != nil {
		return clean(err, password)
	}
	if _, err := w.Write(mailBody(from.String(), to.String(), m, cfg.Host, time.Now())); err != nil {
		return clean(err, password)
	}
	if err := w.Close(); err != nil {
		return clean(err, password)
	}
	// The message is accepted once Data closes; a relay that errs on QUIT
	// has still taken it.
	_ = c.Quit()
	return nil
}

// plainAuth is PLAIN authentication. net/smtp's own refuses a connection
// without TLS even where the installation has allowed one for development;
// the TLS requirement is enforced by sendMail instead, before this runs.
type plainAuth struct{ user, password string }

func (a plainAuth) Start(*smtp.ServerInfo) (string, []byte, error) {
	return "PLAIN", []byte("\x00" + a.user + "\x00" + a.password), nil
}

func (plainAuth) Next(_ []byte, more bool) ([]byte, error) {
	if more {
		return nil, errors.New("unexpected server challenge")
	}
	return nil, nil
}

// mailBody renders the message: UTF-8 text, quoted-printable, with the
// subject encoded so that no header can be injected.
func mailBody(from, to string, m Message, host string, now time.Time) []byte {
	var b bytes.Buffer
	header := func(k, v string) {
		b.WriteString(k + ": " + strings.NewReplacer("\r", " ", "\n", " ").Replace(v) + "\r\n")
	}
	header("From", from)
	header("To", to)
	header("Subject", mime.QEncoding.Encode("utf-8", "[Plux] "+m.Title))
	header("Date", now.UTC().Format(time.RFC1123Z))
	header("Message-ID", "<"+m.ID+"@"+host+">")
	header("MIME-Version", "1.0")
	header("Content-Type", "text/plain; charset=UTF-8")
	header("Content-Transfer-Encoding", "quoted-printable")
	header("Auto-Submitted", "auto-generated")
	b.WriteString("\r\n")
	text := m.Title + "\r\n\r\n" + m.Body
	if m.Link != "" {
		text += "\r\n\r\n" + m.Link
	}
	w := quotedprintable.NewWriter(&b)
	_, _ = w.Write([]byte(text))
	_ = w.Close()
	b.WriteString("\r\n")
	return b.Bytes()
}
