package email

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ramonpaolo/daily-digest-news/internal/config"
)

type SMTPMailer struct {
	config config.Config
	dial   func(context.Context, string, string) (net.Conn, error)
	logf   func(string, ...any)
}

func NewSMTPMailer(cfg config.Config) *SMTPMailer {
	dialer := &net.Dialer{Timeout: 15 * time.Second}
	return &SMTPMailer{
		config: cfg,
		dial:   dialer.DialContext,
		logf:   log.Printf,
	}
}

func (m *SMTPMailer) SetLogger(logf func(string, ...any)) {
	if logf != nil {
		m.logf = logf
	}
}

func (m *SMTPMailer) Send(ctx context.Context, message Message) error {
	sendStarted := time.Now()
	raw, from, recipient, err := buildMessage(message, m.config.SMTPFrom, m.config.EmailTo)
	if err != nil {
		m.logf("component=smtp event=message_build_failed error=%q", safeError(err))
		return err
	}
	m.logf("component=smtp event=send_start host=%s port=%d security=%s from_domain=%s recipient_domain=%s bytes=%d", hostLabel(m.config.SMTPHost), m.config.SMTPPort, m.config.SMTPSecurity, addressDomain(from), addressDomain(recipient), len(raw))
	address := net.JoinHostPort(m.config.SMTPHost, strconv.Itoa(m.config.SMTPPort))
	var conn net.Conn
	stageStarted := time.Now()
	if m.config.SMTPSecurity == "tls" {
		plainConn, dialErr := m.dial(ctx, "tcp", address)
		if dialErr != nil {
			wrapped := fmt.Errorf("dial SMTP: %w", dialErr)
			m.logStageFailure("dial", stageStarted, wrapped)
			return wrapped
		}
		m.logStageSuccess("dial", stageStarted)
		stageStarted = time.Now()
		tlsConn := tls.Client(plainConn, &tls.Config{MinVersion: tls.VersionTLS12, ServerName: m.config.SMTPHost})
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			_ = plainConn.Close()
			wrapped := fmt.Errorf("establish SMTP TLS: %w", err)
			m.logStageFailure("tls_handshake", stageStarted, wrapped)
			return wrapped
		}
		m.logStageSuccess("tls_handshake", stageStarted)
		conn = tlsConn
	} else {
		conn, err = m.dial(ctx, "tcp", address)
		if err != nil {
			wrapped := fmt.Errorf("dial SMTP: %w", err)
			m.logStageFailure("dial", stageStarted, wrapped)
			return wrapped
		}
		m.logStageSuccess("dial", stageStarted)
	}
	defer conn.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	} else {
		_ = conn.SetDeadline(time.Now().Add(15 * time.Second))
	}

	stageStarted = time.Now()
	client, err := smtp.NewClient(conn, m.config.SMTPHost)
	if err != nil {
		wrapped := fmt.Errorf("start SMTP client: %w", err)
		m.logStageFailure("client", stageStarted, wrapped)
		return wrapped
	}
	m.logStageSuccess("client", stageStarted)
	defer client.Close()
	stageStarted = time.Now()
	if err := client.Hello(m.config.SMTPHost); err != nil {
		wrapped := fmt.Errorf("SMTP hello: %w", err)
		m.logStageFailure("hello", stageStarted, wrapped)
		return wrapped
	}
	m.logStageSuccess("hello", stageStarted)
	if m.config.SMTPSecurity == "starttls" {
		stageStarted = time.Now()
		if err := client.StartTLS(&tls.Config{MinVersion: tls.VersionTLS12, ServerName: m.config.SMTPHost}); err != nil {
			wrapped := fmt.Errorf("start SMTP TLS: %w", err)
			m.logStageFailure("starttls", stageStarted, wrapped)
			return wrapped
		}
		m.logStageSuccess("starttls", stageStarted)
	}
	stageStarted = time.Now()
	if err := client.Auth(smtp.PlainAuth("", m.config.SMTPUsername, m.config.SMTPPassword, m.config.SMTPHost)); err != nil {
		wrapped := fmt.Errorf("SMTP authentication failed: %w", err)
		m.logStageFailure("auth", stageStarted, wrapped)
		return wrapped
	}
	m.logStageSuccess("auth", stageStarted)
	stageStarted = time.Now()
	if err := client.Mail(from); err != nil {
		wrapped := fmt.Errorf("SMTP sender rejected: %w", err)
		m.logStageFailure("mail_from", stageStarted, wrapped)
		return wrapped
	}
	m.logStageSuccess("mail_from", stageStarted)
	stageStarted = time.Now()
	if err := client.Rcpt(recipient); err != nil {
		wrapped := fmt.Errorf("SMTP recipient rejected: %w", err)
		m.logStageFailure("rcpt_to", stageStarted, wrapped)
		return wrapped
	}
	m.logStageSuccess("rcpt_to", stageStarted)
	stageStarted = time.Now()
	writer, err := client.Data()
	if err != nil {
		wrapped := fmt.Errorf("open SMTP message data: %w", err)
		m.logStageFailure("data_start", stageStarted, wrapped)
		return wrapped
	}
	m.logStageSuccess("data_start", stageStarted)
	stageStarted = time.Now()
	if _, err := writer.Write(raw); err != nil {
		_ = writer.Close()
		wrapped := fmt.Errorf("write SMTP message: %w", err)
		m.logStageFailure("data_write", stageStarted, wrapped)
		return wrapped
	}
	m.logStageSuccess("data_write", stageStarted)
	stageStarted = time.Now()
	if err := writer.Close(); err != nil {
		wrapped := fmt.Errorf("finish SMTP message: %w", err)
		m.logStageFailure("data_finish", stageStarted, wrapped)
		return wrapped
	}
	m.logStageSuccess("data_finish", stageStarted)
	stageStarted = time.Now()
	if err := client.Quit(); err != nil {
		wrapped := fmt.Errorf("finish SMTP session: %w", err)
		m.logStageFailure("quit", stageStarted, wrapped)
		return wrapped
	}
	m.logStageSuccess("quit", stageStarted)
	m.logf("component=smtp event=send_success duration_ms=%d bytes=%d", time.Since(sendStarted).Milliseconds(), len(raw))
	return nil
}

func (m *SMTPMailer) logStageSuccess(stage string, started time.Time) {
	m.logf("component=smtp event=stage_success stage=%s duration_ms=%d", stage, time.Since(started).Milliseconds())
}

func (m *SMTPMailer) logStageFailure(stage string, started time.Time, err error) {
	m.logf("component=smtp event=stage_failed stage=%s duration_ms=%d error=%q", stage, time.Since(started).Milliseconds(), safeError(err))
}

func addressDomain(value string) string {
	parsed, err := mail.ParseAddress(value)
	if err != nil {
		return "invalid"
	}
	at := strings.LastIndex(parsed.Address, "@")
	if at <= 0 || at == len(parsed.Address)-1 {
		return "invalid"
	}
	return strings.ToLower(parsed.Address[at+1:])
}

func hostLabel(value string) string {
	parsed, err := url.Parse("//" + strings.TrimSpace(value))
	if err != nil || parsed.Hostname() == "" {
		return "invalid"
	}
	return parsed.Hostname()
}

func safeError(err error) string {
	if err == nil {
		return ""
	}
	value := strings.Join(strings.Fields(err.Error()), " ")
	if len(value) > 240 {
		return value[:240] + "…"
	}
	return value
}

func buildMessage(message Message, from, recipient string) ([]byte, string, string, error) {
	fromAddress, err := mail.ParseAddress(from)
	if err != nil {
		return nil, "", "", fmt.Errorf("invalid sender address")
	}
	recipientAddress, err := mail.ParseAddress(recipient)
	if err != nil {
		return nil, "", "", fmt.Errorf("invalid recipient address")
	}
	if strings.TrimSpace(message.Subject) == "" || strings.TrimSpace(message.TextBody) == "" || strings.TrimSpace(message.HTMLBody) == "" {
		return nil, "", "", fmt.Errorf("email subject and both body representations are required")
	}

	boundary := "daily-digest-news-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	var body bytes.Buffer
	body.WriteString("MIME-Version: 1.0\r\n")
	body.WriteString("From: " + fromAddress.String() + "\r\n")
	body.WriteString("To: " + recipientAddress.String() + "\r\n")
	body.WriteString("Subject: " + mime.QEncoding.Encode("UTF-8", message.Subject) + "\r\n")
	body.WriteString("Content-Type: multipart/alternative; boundary=\"" + boundary + "\"\r\n")
	body.WriteString("\r\n")

	multipartWriter := multipart.NewWriter(&body)
	if err := multipartWriter.SetBoundary(boundary); err != nil {
		return nil, "", "", fmt.Errorf("set MIME boundary: %w", err)
	}
	for _, part := range []struct {
		contentType string
		content     string
	}{{"text/plain; charset=UTF-8", message.TextBody}, {"text/html; charset=UTF-8", message.HTMLBody}} {
		header := make(textproto.MIMEHeader)
		header["Content-Type"] = []string{part.contentType}
		header["Content-Transfer-Encoding"] = []string{"quoted-printable"}
		partWriter, err := multipartWriter.CreatePart(header)
		if err != nil {
			return nil, "", "", fmt.Errorf("create MIME part: %w", err)
		}
		quotedWriter := quotedprintable.NewWriter(partWriter)
		if _, err := io.WriteString(quotedWriter, part.content); err != nil {
			return nil, "", "", fmt.Errorf("encode MIME part: %w", err)
		}
		if err := quotedWriter.Close(); err != nil {
			return nil, "", "", fmt.Errorf("finish MIME part: %w", err)
		}
	}
	if err := multipartWriter.Close(); err != nil {
		return nil, "", "", fmt.Errorf("finish MIME message: %w", err)
	}
	return body.Bytes(), fromAddress.Address, recipientAddress.Address, nil
}
