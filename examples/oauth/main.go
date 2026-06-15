// Command oauth demonstrates the OAuth 2.1 authorization-code flow with PKCE.
//
// Set WARMBLY_CLIENT_ID and WARMBLY_CLIENT_SECRET, run the program, then open
// http://localhost:8080/login in a browser.
//
//	WARMBLY_CLIENT_ID=... WARMBLY_CLIENT_SECRET=... go run ./examples/oauth
//
// This single-user example keeps the PKCE verifier and state in package
// variables for brevity; a real application stores them per session.
package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/warmbly/warmbly-go"
)

var (
	cfg = &warmbly.OAuth2Config{
		ClientID:     os.Getenv("WARMBLY_CLIENT_ID"),
		ClientSecret: os.Getenv("WARMBLY_CLIENT_SECRET"),
		RedirectURL:  "http://localhost:8080/callback",
		Scopes:       []string{"campaigns:read", "contacts:read"},
	}
	verifier = warmbly.GenerateVerifier()
	state    = "demo-state"
)

func main() {
	http.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, cfg.AuthCodeURL(state, warmbly.S256ChallengeOption(verifier)), http.StatusFound)
	})

	http.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != state {
			http.Error(w, "state mismatch", http.StatusBadRequest)
			return
		}
		tok, err := cfg.Exchange(r.Context(), r.URL.Query().Get("code"), warmbly.VerifierOption(verifier))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		client, err := cfg.NewClient(r.Context(), tok)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		page, err := client.Campaigns.List(r.Context(), nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		fmt.Fprintf(w, "authorized! you have %d campaigns on this page\n", len(page.Data))
	})

	log.Println("listening on http://localhost:8080 (open /login)")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
