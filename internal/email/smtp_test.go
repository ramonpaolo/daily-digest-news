package email

import (
	"strings"
	"testing"
)

func TestBuildMessageCreatesMultipartMessageWithValidatedAddresses(t *testing.T) {
	message, _, _, err := buildMessage(Message{
		Subject:  "Daily Digest",
		TextBody: "plain body",
		HTMLBody: "<p>html body</p>",
	}, "sender@example.test", "receiver@example.test")
	if err != nil {
		t.Fatalf("buildMessage() error = %v", err)
	}
	raw := string(message)
	for _, want := range []string{"From: <sender@example.test>", "To: <receiver@example.test>", "Subject: Daily Digest", "text/plain", "text/html", "plain body", "html body"} {
		if !strings.Contains(raw, want) {
			t.Fatalf("message missing %q: %s", want, raw)
		}
	}
}

func TestBuildMessageRejectsInvalidRecipient(t *testing.T) {
	_, _, _, err := buildMessage(Message{Subject: "Subject", TextBody: "Text", HTMLBody: "<p>HTML</p>"}, "sender@example.test", "not-an-email")
	if err == nil || !strings.Contains(err.Error(), "recipient") {
		t.Fatalf("buildMessage() error = %v, want recipient validation", err)
	}
}
