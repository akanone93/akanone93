// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Verifies: SEC-103.
// The step-up page looks like Studio in the visitor's language: a visitor
// without a Studio session sees a notice, under the same strict CSP, in
// English, German or right-to-left Arabic as the browser asks (build log row
// 204).
func TestStepUpPageLanguageAndPolicy(t *testing.T) {
	t.Parallel()
	routes, err := NewStepUpRoutes(StepUpRoutes{})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		accept, lang, dir, words string
	}{
		{"", "en", "ltr", "Sign in to Plux Studio in this browser"},
		{"fr-CA, de-DE;q=0.8, en;q=0.5", "de", "ltr", "Melden Sie sich in diesem Browser"},
		{"ar-EG", "ar", "rtl", "سجّل الدخول"},
		{"de;q=0, ar;q=0.4", "ar", "rtl", "سجّل الدخول"},
	} {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, StepUpPagePath+"?request=r1", nil)
		if tc.accept != "" {
			req.Header.Set("Accept-Language", tc.accept)
		}
		rec := httptest.NewRecorder()
		routes.Page(rec, req)
		body := rec.Body.String()
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%q: status %d", tc.accept, rec.Code)
		}
		if !strings.Contains(body, `<html lang="`+tc.lang+`" dir="`+tc.dir+`">`) || !strings.Contains(body, tc.words) {
			t.Errorf("%q: the page is not in %s: %.300s", tc.accept, tc.lang, body)
		}
		csp := rec.Header().Get("Content-Security-Policy")
		if !strings.HasPrefix(csp, "default-src 'none'; script-src 'nonce-") || !strings.Contains(csp, "frame-ancestors 'none'") {
			t.Errorf("%q: CSP %q", tc.accept, csp)
		}
		if !strings.Contains(body, "Plux Studio") || !strings.Contains(body, "prefers-color-scheme:dark") {
			t.Errorf("%q: no Studio brand or dark theme", tc.accept)
		}
	}
}

// Verifies: SEC-103.
// The ceremony page carries its words in escaped data attributes; its script
// reads them and holds no text of its own.
func TestStepUpPageCarriesItsWordsEscaped(t *testing.T) {
	t.Parallel()
	routes, err := NewStepUpRoutes(StepUpRoutes{})
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	data := pageData{Nonce: "n", Request: `r"><script>`, CSRF: "c", T: stepUpTextFor("de")}
	if err := routes.page.Execute(&b, data); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if strings.Contains(out, `r"><script>`) {
		t.Error("the request id is not escaped")
	}
	for _, want := range []string{`data-waiting="Warte auf Ihren Sicherheitsschlüssel …"`, "app.dataset.confirmed", "Meinen Sicherheitsschlüssel verwenden"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q", want)
		}
	}
}
