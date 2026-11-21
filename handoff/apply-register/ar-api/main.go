// ar-api: the Apply Register back end, as a stand-in. Handlers only: every answer comes from memory, nothing is stored
// past a restart, and there are no tests. Run it with `go run .` (Go 1.23 or later); see README.md.
package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

func main() {
	addr := flag.String("addr", envOr("AR_ADDR", ":8090"), "address to listen on")
	flag.Parse()
	a := &api{db: seed(), jwt: newSigner()}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { reply(w, 200, map[string]string{"status": "ok"}) })

	// Auth.
	mux.HandleFunc("POST /auth/otp/start", a.otpStart)
	mux.HandleFunc("POST /auth/otp/verify", a.otpVerify)
	mux.HandleFunc("POST /auth/refresh", a.refresh)
	mux.HandleFunc("POST /auth/logout", a.logout)
	mux.HandleFunc("DELETE /auth/account", a.deleteAccount)
	mux.HandleFunc("GET /auth/.well-known/jwks.json", func(w http.ResponseWriter, r *http.Request) { reply(w, 200, a.jwt.jwks()) })

	// REST.
	mux.HandleFunc("GET /v1/categories", a.categories)
	mux.HandleFunc("GET /v1/orgs", a.orgs)
	mux.HandleFunc("GET /v1/orgs/{slug}", a.org)
	mux.HandleFunc("GET /v1/orgs/{slug}/status", a.orgStatus)
	mux.HandleFunc("GET /v1/orgs/devconf/products", a.products)
	mux.HandleFunc("GET /v1/orgs/homes/listings", a.listings)
	mux.HandleFunc("GET /v1/orgs/relief/campaign", a.campaign)
	mux.HandleFunc("GET /v1/me", a.me)
	mux.HandleFunc("GET /v1/me/donations", a.donations)
	mux.HandleFunc("GET /v1/me/registrations", a.myRegistrations)
	mux.HandleFunc("GET /v1/me/registrations/{id}", a.myRegistration)
	mux.HandleFunc("DELETE /v1/me/registrations/{id}", a.withdraw)
	mux.HandleFunc("POST /v1/registrations", a.register)
	mux.HandleFunc("PATCH /v1/registrations/{id}", a.setStatus)
	mux.HandleFunc("POST /v1/uploads", a.upload)
	mux.HandleFunc("POST /v1/transfers", a.transfer)
	mux.HandleFunc("POST /v1/accounts", a.openAccount)
	mux.HandleFunc("GET /v1/staff/orgs/{slug}/stats", a.staffStats)
	mux.HandleFunc("GET /v1/staff/orgs/{slug}/submissions", a.staffSubmissions)
	mux.HandleFunc("POST /v1/checkins", a.checkin)
	mux.HandleFunc("POST /v1/partner-requests", a.partnerRequest)

	// GraphQL, SSE, WebSocket.
	mux.HandleFunc("POST /graphql", a.graphql)
	mux.HandleFunc("GET /sse/inbox", a.inbox)
	mux.HandleFunc("GET /sse/clinics/{id}/queue", a.queue)
	mux.HandleFunc("GET /sse/registrations/{id}", a.registrationLive)
	mux.HandleFunc("GET /ws/races/{raceId}/runners/{bib}", a.runner)

	log.Printf("ar-api on %s · demo sign-in: +15555550100 / 000000 · staff: +15555550101", *addr)
	srv := &http.Server{Addr: *addr, Handler: logged(cors(failures(mux))), ReadHeaderTimeout: 10 * time.Second}
	log.Fatal(srv.ListenAndServe())
}

func envOr(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

// failures honours X-AR-Fail: 500 | timeout | 401 | 409 | slow, to show every error path (not for production).
func failures(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Header.Get("X-AR-Fail") {
		case "500":
			fail(w, 500, "server_error", "simulated failure")
			return
		case "401":
			fail(w, 401, "unauthenticated", "simulated expired session")
			return
		case "409":
			fail(w, 409, "conflict", "simulated conflict")
			return
		case "timeout":
			select {
			case <-r.Context().Done():
			case <-time.After(60 * time.Second):
			}
			return
		case "slow":
			time.Sleep(3 * time.Second)
		}
		next.ServeHTTP(w, r)
	})
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Access-Control-Allow-Origin", "*")
		h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Idempotency-Key, X-AR-Fail, X-AR-Key, X-AR-Gateway")
		h.Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type recorder struct {
	http.ResponseWriter
	status int
}

func (r *recorder) WriteHeader(s int) { r.status = s; r.ResponseWriter.WriteHeader(s) }
func (r *recorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func logged(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		if strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
			next.ServeHTTP(w, r) // the WebSocket hijacks the connection: no wrapper
			log.Printf("%s %s websocket", r.Method, r.URL.Path)
			return
		}
		rec := &recorder{ResponseWriter: w, status: 200}
		next.ServeHTTP(rec, r)
		log.Printf("%s %s %d %s", r.Method, r.URL.RequestURI(), rec.status, time.Since(start).Round(time.Millisecond))
	})
}
