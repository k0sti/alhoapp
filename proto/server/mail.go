package main

import (
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

type Mailer interface {
	Send(to, subject, body string) error
}

// SMTPMailer sends plain-text mail. Port 465 uses implicit TLS; any other port
// uses STARTTLS when the server offers it (net/smtp does this on SendMail).
type SMTPMailer struct {
	Host, Username, Password, From string
	Port                           int
}

func (m *SMTPMailer) Send(to, subject, body string) error {
	addr := net.JoinHostPort(m.Host, strconv.Itoa(m.Port))
	msg := strings.Join([]string{
		"From: " + m.From,
		"To: " + to,
		"Subject: " + subject,
		"Date: " + time.Now().Format(time.RFC1123Z),
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=utf-8",
		"",
		body,
	}, "\r\n")
	var auth smtp.Auth
	if m.Username != "" {
		auth = smtp.PlainAuth("", m.Username, m.Password, m.Host)
	}
	if m.Port == 465 {
		return sendImplicitTLS(addr, m.Host, auth, m.From, to, []byte(msg))
	}
	return smtp.SendMail(addr, auth, m.From, []string{to}, []byte(msg))
}

// LogMailer stands in when no SMTP server is configured, so that local
// development works; the confirmation link appears in the server log.
type LogMailer struct{}

func (LogMailer) Send(to, subject, body string) error {
	log.Printf("mail (SMTP not configured) to=%s subject=%q\n%s", to, subject, body)
	return nil
}

func confirmationMail(name, link string) (subject, body string) {
	subject = "Confirm your email for Alhovuori"
	body = fmt.Sprintf(`Hello %s,

welcome to the Alhovuori community. Please confirm your email address by opening this link:

%s

We use email for important community announcements. If you did not register, you can ignore this message.
`, name, link)
	return subject, body
}

func sendImplicitTLS(addr, host string, auth smtp.Auth, from, to string, msg []byte) error {
	conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: host})
	if err != nil {
		return err
	}
	c, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	defer c.Close()
	if auth != nil {
		if err := c.Auth(auth); err != nil {
			return err
		}
	}
	if err := c.Mail(from); err != nil {
		return err
	}
	if err := c.Rcpt(to); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}
