package main

import (
	"embed"
	"log/slog"
	"net/http"
)

//go:embed login.html
var loginContent embed.FS

func handleLogin(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		data, _ := loginContent.ReadFile("login.html")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(data)

	case "POST":
		key := r.FormValue("api_key")
		if key == "" {
			http.Error(w, "api_key is required", http.StatusBadRequest)
			return
		}

		if authAPIKey != "" {
			if key != authAPIKey {
				slog.Warn("login failed: wrong static key", "remote", r.RemoteAddr)
				http.Error(w, "invalid API key", http.StatusUnauthorized)
				return
			}
		} else {
			ak, err := validateDashboardKey(r.Context(), key)
			if err != nil {
				slog.Error("login: redis error", "error", err)
				http.Error(w, "auth unavailable", http.StatusInternalServerError)
				return
			}
			if ak == nil {
				slog.Warn("login failed: invalid key", "remote", r.RemoteAddr)
				http.Error(w, "invalid API key", http.StatusUnauthorized)
				return
			}
			if ak.Status != "active" {
				slog.Warn("login failed: inactive key", "status", ak.Status, "remote", r.RemoteAddr)
				http.Error(w, "key is inactive", http.StatusUnauthorized)
				return
			}
		}

		http.SetCookie(w, &http.Cookie{
			Name:     "dashboard_api_key",
			Value:    key,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   86400 * 7,
		})

		slog.Info("login succeeded", "remote", r.RemoteAddr)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "dashboard_api_key",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
