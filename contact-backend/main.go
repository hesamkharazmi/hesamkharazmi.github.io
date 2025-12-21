package main

import (
	"net/http"
	"net/smtp"
	"os"
	"regexp"
	"sync"
	"time"
)

var (
	mu       sync.Mutex
	requests = make(map[string][]time.Time)
)

var emailRegex = regexp.MustCompile(
	`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`,
)

// rate limit: 5 req / minute / IP
func rateLimit(ip string) bool {
	mu.Lock()
	defer mu.Unlock()

	now := time.Now()
	window := now.Add(-1 * time.Minute)

	times := requests[ip]
	var fresh []time.Time
	for _, t := range times {
		if t.After(window) {
			fresh = append(fresh, t)
		}
	}

	if len(fresh) >= 5 {
		return false
	}

	fresh = append(fresh, now)
	requests[ip] = fresh
	return true
}

func contactHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	ip := r.RemoteAddr
	if !rateLimit(ip) {
		w.WriteHeader(http.StatusTooManyRequests)
		return
	}

	if err := r.ParseForm(); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// honeypot
	if r.PostFormValue("website") != "" {
		w.WriteHeader(http.StatusOK)
		return
	}

	name := r.PostFormValue("name")
	email := r.PostFormValue("email")
	subject := r.PostFormValue("subject")
	message := r.PostFormValue("message")

	// basic validation
	if name == "" || subject == "" || len(message) < 10 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// email validation (regex)
	if !emailRegex.MatchString(email) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	gmailUser := os.Getenv("GMAIL_USER")
	gmailPass := os.Getenv("GMAIL_PASS")
	toEmail := os.Getenv("TO_EMAIL")

	if gmailUser == "" || gmailPass == "" || toEmail == "" {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	auth := smtp.PlainAuth("", gmailUser, gmailPass, "smtp.gmail.com")

	body := "Name: " + name +
		"\nEmail: " + email +
		"\nIP: " + ip +
		"\n\n" + message

	msg := []byte(
		"Reply-To: " + email + "\r\n" +
			"Subject: " + subject + "\r\n\r\n" +
			body,
	)

	err := smtp.SendMail(
		"smtp.gmail.com:587",
		auth,
		gmailUser,
		[]string{toEmail},
		msg,
	)

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func main() {
	http.HandleFunc("/contact", contactHandler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	http.ListenAndServe(":"+port, nil)
}
