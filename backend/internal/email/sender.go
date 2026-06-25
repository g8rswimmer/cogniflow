package email

import (
	"bytes"
	"fmt"
	"net/smtp"
	"strings"
	"text/template"
	"time"
)

// DefaultSubject is the built-in invite email subject template.
const DefaultSubject = `You've been invited to join {{.OrgName}} on cogniflow`

// DefaultBody is the built-in invite email body template.
const DefaultBody = `Hi,

{{.InviterEmail}} has invited you to join {{.OrgName}} on cogniflow.

Accept your invitation and set your password here:
{{.InviteURL}}

This link expires on {{.ExpiresAt.Format "January 2, 2006 at 3:04 PM UTC"}}.

If you did not expect this invitation, you can safely ignore this email.
`

// InviteData holds the values available to invite email templates.
type InviteData struct {
	OrgName      string
	InviteURL    string
	InviteeEmail string
	InviterEmail string
	ExpiresAt    time.Time
}

// Sender delivers transactional emails via SMTP (STARTTLS on port 587).
type Sender struct {
	host     string
	port     string
	user     string
	password string
	from     string
}

// New creates a Sender. host must be non-empty; port defaults to "587" if empty.
func New(host, port, user, password, from string) *Sender {
	if port == "" {
		port = "587"
	}
	return &Sender{host: host, port: port, user: user, password: password, from: from}
}

// SendInvite sends an invite email to `to`.
// subjectTmpl and bodyTmpl are Go text/template strings; pass empty strings to
// use the built-in defaults.
func (s *Sender) SendInvite(to string, data InviteData, subjectTmpl, bodyTmpl string) error {
	if subjectTmpl == "" {
		subjectTmpl = DefaultSubject
	}
	if bodyTmpl == "" {
		bodyTmpl = DefaultBody
	}

	subject, err := renderTemplate("subject", subjectTmpl, data)
	if err != nil {
		return fmt.Errorf("render subject: %w", err)
	}
	body, err := renderTemplate("body", bodyTmpl, data)
	if err != nil {
		return fmt.Errorf("render body: %w", err)
	}

	msg := buildMessage(s.from, to, subject, body)
	addr := s.host + ":" + s.port
	var auth smtp.Auth
	if s.user != "" {
		auth = smtp.PlainAuth("", s.user, s.password, s.host)
	}
	return smtp.SendMail(addr, auth, s.from, []string{to}, []byte(msg))
}

// ParseTemplate validates a Go text/template string. Returns a non-nil error
// if the template syntax is invalid. Used by the API handler before persisting.
func ParseTemplate(tmplStr string) error {
	_, err := template.New("").Parse(tmplStr)
	return err
}

func renderTemplate(name, tmplStr string, data InviteData) (string, error) {
	tmpl, err := template.New(name).Parse(tmplStr)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return strings.TrimSpace(buf.String()), nil
}

func buildMessage(from, to, subject, body string) string {
	var sb strings.Builder
	sb.WriteString("From: " + from + "\r\n")
	sb.WriteString("To: " + to + "\r\n")
	sb.WriteString("Subject: " + subject + "\r\n")
	sb.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	sb.WriteString("\r\n")
	sb.WriteString(body)
	return sb.String()
}
