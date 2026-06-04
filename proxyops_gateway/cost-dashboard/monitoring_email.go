package main

import (
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/smtp"
	"os"
	"strings"
)

type emailConfig struct {
	SMTPHost string
	SMTPPort string
	SMTPUser string
	SMTPPass string
	FromAddr string
}

var emailCfg *emailConfig

func initEmailConfig() {
	host := os.Getenv("SMTP_HOST")
	port := os.Getenv("SMTP_PORT")
	if host == "" || port == "" {
		return
	}
	emailCfg = &emailConfig{
		SMTPHost: host,
		SMTPPort: port,
		SMTPUser: os.Getenv("SMTP_USER"),
		SMTPPass: os.Getenv("SMTP_PASS"),
		FromAddr: os.Getenv("FROM_ADDR"),
	}
	if emailCfg.FromAddr == "" {
		emailCfg.FromAddr = fmt.Sprintf("tokensentinel@%s", host)
	}
}

func sendAlertEmail(to string, a Alert) {
	if emailCfg == nil {
		return
	}

	subject := fmt.Sprintf("[TokenSentinel] %s: %s alert for %s", strings.ToUpper(a.Severity), a.AlertType, a.Model)
	body := fmt.Sprintf(`TokenSentinel Alert

Type: %s
Severity: %s
Model: %s
Time: %s

%s

Current value: $%.2f
Threshold: $%.2f

---
TokenSentinel Continuous Optimization
`, a.AlertType, a.Severity, a.Model, a.CreatedAt, a.Message, a.CurrentValue, a.ThresholdValue)

	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s",
		emailCfg.FromAddr, to, subject, body)

	addr := net.JoinHostPort(emailCfg.SMTPHost, emailCfg.SMTPPort)
	var auth smtp.Auth
	if emailCfg.SMTPUser != "" {
		auth = smtp.PlainAuth("", emailCfg.SMTPUser, emailCfg.SMTPPass, emailCfg.SMTPHost)
	}

	if emailCfg.SMTPPort == "465" {
		tlsCfg := &tls.Config{ServerName: emailCfg.SMTPHost}
		conn, err := tls.Dial("tcp", addr, tlsCfg)
		if err != nil {
			log.Printf("email: tls dial failed: %v", err)
			return
		}
		client, err := smtp.NewClient(conn, emailCfg.SMTPHost)
		if err != nil {
			conn.Close()
			log.Printf("email: new client failed: %v", err)
			return
		}
		defer client.Close()
		if auth != nil {
			client.Auth(auth)
		}
		client.Mail(emailCfg.FromAddr)
		client.Rcpt(to)
		w, err := client.Data()
		if err != nil {
			log.Printf("email: data failed: %v", err)
			return
		}
		w.Write([]byte(msg))
		w.Close()
	} else {
		if err := smtp.SendMail(addr, auth, emailCfg.FromAddr, []string{to}, []byte(msg)); err != nil {
			log.Printf("email: send failed to %s: %v", to, err)
			return
		}
	}
	log.Printf("email: alert sent to %s", to)
}
