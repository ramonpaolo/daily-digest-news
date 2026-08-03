package email

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"strconv"
	"strings"
	"time"

	"github.com/ramonpaolo/daily-digest-news/internal/config"
)

type SMTPMailer struct {
	config config.Config
	dial   func(context.Context, string, string) (net.Conn, error)
}

func NewSMTPMailer(cfg config.Config) *SMTPMailer {
	dialer := &net.Dialer{Timeout: 15 * time.Second}
	return &SMTPMailer{
		config: cfg,
		dial:   dialer.DialContext,
	}
}

func (m *SMTPMailer) Send(ctx context.Context, message Message) error {
	raw, from, recipient, err := buildMessage(message, m.config.SMTPFrom, m.config.EmailTo)
	if err != nil {
		return err
	}
	address := net.JoinHostPort(m.config.SMTPHost, strconv.Itoa(m.config.SMTPPort))
	var conn net.Conn
	if m.config.SMTPSecurity == "tls" {
		plainConn, dialErr := m.dial(ctx, "tcp", address)
		if dialErr != nil {
			return fmt.Errorf("dial SMTP: %w", dialErr)
		}
		tlsConn := tls.Client(plainConn, &tls.Config{MinVersion: tls.VersionTLS12, ServerName: m.config.SMTPHost})
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			_ = plainConn.Close()
			return fmt.Errorf("establish SMTP TLS: %w", err)
		}
		conn = tlsConn
	} else {
		conn, err = m.dial(ctx, "tcp", address)
		if err != nil {
			return fmt.Errorf("dial SMTP: %w", err)
		}
	}
	defer conn.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	} else {
		_ = conn.SetDeadline(time.Now().Add(15 * time.Second))
	}

	client, err := smtp.NewClient(conn, m.config.SMTPHost)
	if err != nil {
		return fmt.Errorf("start SMTP client: %w", err)
	}
	defer client.Close()
	if err := client.Hello(m.config.SMTPHost); err != nil {
		return fmt.Errorf("SMTP hello: %w", err)
	}
	if m.config.SMTPSecurity == "starttls" {
		if err := client.StartTLS(&tls.Config{MinVersion: tls.VersionTLS12, ServerName: m.config.SMTPHost}); err != nil {
			return fmt.Errorf("start SMTP TLS: %w", err)
		}
	}
	if err := client.Auth(smtp.PlainAuth("", m.config.SMTPUsername, m.config.SMTPPassword, m.config.SMTPHost)); err != nil {
		return fmt.Errorf("SMTP authentication failed: %w", err)
	}
	if err := client.Mail(from); err != nil {
		return fmt.Errorf("SMTP sender rejected: %w", err)
	}
	if err := client.Rcpt(recipient); err != nil {
		return fmt.Errorf("SMTP recipient rejected: %w", err)
	}
	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("open SMTP message data: %w", err)
	}
	if _, err := writer.Write(raw); err != nil {
		_ = writer.Close()
		return fmt.Errorf("write SMTP message: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("finish SMTP message: %w", err)
	}
	if err := client.Quit(); err != nil {
		return fmt.Errorf("finish SMTP session: %w", err)
	}
	return nil
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
