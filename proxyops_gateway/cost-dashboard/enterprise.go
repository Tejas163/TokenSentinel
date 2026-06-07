package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/smtp"
	"net/http"
	"os"
)

type enterpriseInquiry struct {
	Name       string `json:"name"`
	Email      string `json:"email"`
	Company    string `json:"company"`
	Phone      string `json:"phone"`
	Deployment string `json:"deployment"`
	Volume     string `json:"volume"`
	Models     string `json:"models"`
	Message    string `json:"message"`
}

var enterpriseEmailTo string

func init() {
	enterpriseEmailTo = os.Getenv("ENTERPRISE_EMAIL_TO")
	if enterpriseEmailTo == "" {
		enterpriseEmailTo = "tejaskrshna@gmail.com"
	}
}

func handleEnterprisePage(w http.ResponseWriter, r *http.Request) {
	html, err := enterpriseHTML.ReadFile("enterprise.html")
	if err != nil {
		http.Error(w, "page not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(html)
}

func handleEnterpriseInquiry(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var inq enterpriseInquiry
	if err := json.NewDecoder(r.Body).Decode(&inq); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	if inq.Name == "" || inq.Email == "" || inq.Company == "" {
		http.Error(w, "name, email, and company are required", http.StatusBadRequest)
		return
	}

		slog.Info("enterprise inquiry received", "name", inq.Name, "email", inq.Email, "company", inq.Company)

	if emailCfg != nil {
		subject := fmt.Sprintf("[TokenSentinel] Enterprise Inquiry from %s (%s)", inq.Name, inq.Company)
		body := fmt.Sprintf("Name: %s\nEmail: %s\nCompany: %s\nPhone: %s\nDeployment: %s\nMonthly Volume: %s\nModels Used: %s\n\nMessage:\n%s\n",
			inq.Name, inq.Email, inq.Company, inq.Phone, inq.Deployment, inq.Volume, inq.Models, inq.Message)
		msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s",
			emailCfg.FromAddr, enterpriseEmailTo, subject, body)
		addr := net.JoinHostPort(emailCfg.SMTPHost, emailCfg.SMTPPort)
		var auth smtp.Auth
		if emailCfg.SMTPUser != "" {
			auth = smtp.PlainAuth("", emailCfg.SMTPUser, emailCfg.SMTPPass, emailCfg.SMTPHost)
		}
		if emailCfg.SMTPPort == "465" {
			tlsCfg := &tls.Config{ServerName: emailCfg.SMTPHost}
			conn, err := tls.Dial("tcp", addr, tlsCfg)
			if err == nil {
				client, err := smtp.NewClient(conn, emailCfg.SMTPHost)
				if err == nil {
					if auth != nil {
						client.Auth(auth)
					}
					client.Mail(emailCfg.FromAddr)
					client.Rcpt(enterpriseEmailTo)
					w, err := client.Data()
					if err == nil {
						w.Write([]byte(msg))
						w.Close()
						slog.Info("enterprise inquiry emailed", "to", enterpriseEmailTo)
					}
					client.Quit()
				}
				conn.Close()
			}
		} else {
			if err := smtp.SendMail(addr, auth, emailCfg.FromAddr, []string{enterpriseEmailTo}, []byte(msg)); err != nil {
				slog.Error("enterprise email send failed", "err", err)
			} else {
				slog.Info("enterprise inquiry emailed", "to", enterpriseEmailTo)
			}
		}
	} else {
		slog.Info("email not configured — enterprise inquiry received", "name", inq.Name, "email", inq.Email, "company", inq.Company)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
