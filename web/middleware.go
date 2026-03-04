package web

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
)

type contextKey int

const nonceKey contextKey = 1

func cspMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nonce := generateNonce()
		w.Header().Set("Content-Security-Policy",
			"default-src 'self'; script-src 'nonce-"+nonce+"'; style-src 'self'; frame-ancestors 'none'; base-uri 'self'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("X-CSP-Nonce", nonce)

		r = r.WithContext(context.WithValue(r.Context(), nonceKey, nonce))
		next.ServeHTTP(w, r)
	})
}

// nonceFromRequest returns the CSP nonce stored in the request context.
func nonceFromRequest(r *http.Request) string {
	v, _ := r.Context().Value(nonceKey).(string)
	return v
}

func generateNonce() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// csrfTokenFromCookie reads the CSRF token from the double-submit cookie.
func csrfTokenFromCookie(r *http.Request) string {
	c, err := r.Cookie("csrf_token")
	if err != nil {
		return ""
	}
	return c.Value
}

// setCSRFCookie sets the double-submit cookie if not already present.
func setCSRFCookie(w http.ResponseWriter, r *http.Request) string {
	existing := csrfTokenFromCookie(r)
	if existing != "" {
		return existing
	}
	token := generateNonce()
	http.SetCookie(w, &http.Cookie{
		Name:     "csrf_token",
		Value:    token,
		Path:     "/",
		HttpOnly: false, // JS needs to read it for htmx headers
		SameSite: http.SameSiteStrictMode,
		Secure:   false, // set true behind TLS termination
	})
	return token
}

// validateCSRF checks the double-submit cookie against the form/header value.
func validateCSRF(r *http.Request) bool {
	cookie := csrfTokenFromCookie(r)
	if cookie == "" {
		return false
	}
	// Check header first (htmx), then form value
	header := r.Header.Get("X-CSRF-Token")
	if header != "" {
		return header == cookie
	}
	if err := r.ParseForm(); err != nil {
		return false
	}
	return r.FormValue("csrf_token") == cookie
}
