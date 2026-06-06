package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/smtp"
	"os"
	"strings"
	"time"
)

type emailConfig struct {
	SMTPHost string
	SMTPPort string
	SMTPUser string
	SMTPPass string
	FromAddr string
}

var emailCfg *emailConfig

var webhookSigningKey string
var webhookClient = &http.Client{Timeout: 10 * time.Second}

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
	webhookSigningKey = os.Getenv("WEBHOOK_SIGNING_KEY")
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
			log.Printf("email: new client failed: %v", err)
			return
		}
		if auth != nil {
			client.Auth(auth)
		}
		if err := client.Mail(emailCfg.FromAddr); err != nil {
			log.Printf("email: mail from failed: %v", err)
			return
		}
		if err := client.Rcpt(to); err != nil {
			log.Printf("email: rcpt to failed: %v", err)
			return
		}
		w, err := client.Data()
		if err != nil {
			log.Printf("email: data failed: %v", err)
			return
		}
		w.Write([]byte(msg))
		w.Close()
		client.Quit()
	} else {
		err := smtp.SendMail(addr, auth, emailCfg.FromAddr, []string{to}, []byte(msg))
		if err != nil {
			log.Printf("email: send mail failed to %s: %v", to, err)
		}
	}
}

func signAndPost(url string, payload []byte) (*http.Response, error) {
	req, err := http.NewRequest("POST", url, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if webhookSigningKey != "" {
		mac := hmac.New(sha256.New, []byte(webhookSigningKey))
		mac.Write(payload)
		sig := hex.EncodeToString(mac.Sum(nil))
		req.Header.Set("X-TokenSentinel-Signature", sig)
	}
	req.Header.Set("X-TokenSentinel-Timestamp", time.Now().UTC().Format(time.RFC3339))
	return webhookClient.Do(req)
}
