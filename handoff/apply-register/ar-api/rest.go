package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type api struct {
	db  *store
	jwt *signer
}

func reply(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, status int, code, message string) {
	reply(w, status, map[string]string{"error": code, "message": message})
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	body := http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(body).Decode(v); err != nil && err != io.EOF {
		fail(w, http.StatusBadRequest, "invalid_json", err.Error())
		return false
	}
	return true
}

// page cuts a list by an opaque cursor (the offset as text) and a limit.
func page[T any](list []T, cursor string, limit int) (items []T, next string, more bool) {
	start, _ := strconv.Atoi(cursor)
	if start < 0 || start > len(list) {
		start = len(list)
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	end := min(start+limit, len(list))
	if end < len(list) {
		next, more = strconv.Itoa(end), true
	}
	return list[start:end], next, more
}

func intParam(r *http.Request, name string, fallback int) int {
	if v, err := strconv.Atoi(r.URL.Query().Get(name)); err == nil {
		return v
	}
	return fallback
}

func (a *api) categories(w http.ResponseWriter, r *http.Request) {
	reply(w, http.StatusOK, a.db.categories)
}

// orgs: GET /v1/orgs?q&category&near=lat,lng&radiusKm&sort&cursor&limit → {items, next, more}.
func (a *api) orgs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	text := strings.ToLower(strings.TrimSpace(q.Get("q")))
	cat := q.Get("category")
	var lat, lng float64
	near := false
	if p := strings.Split(q.Get("near"), ","); len(p) == 2 {
		la, e1 := strconv.ParseFloat(p[0], 64)
		lo, e2 := strconv.ParseFloat(p[1], 64)
		near, lat, lng = e1 == nil && e2 == nil, la, lo
	}
	if v := q.Get("lat"); v != "" { // the `nearby` source names them apart
		lat, _ = strconv.ParseFloat(v, 64)
		lng, _ = strconv.ParseFloat(q.Get("lng"), 64)
		near = true
	}
	radius, _ := strconv.ParseFloat(q.Get("radiusKm"), 64)
	a.db.mu.Lock()
	out := []Org{}
	for _, o := range a.db.orgs {
		if text != "" && !strings.Contains(strings.ToLower(o.Name+" "+o.City+" "+strings.Join(o.Tags, " ")), text) {
			continue
		}
		if cat != "" && o.Category != cat {
			continue
		}
		if near && radius > 0 && distanceKm(lat, lng, o.Lat, o.Lng) > radius {
			continue
		}
		out = append(out, o)
	}
	a.db.mu.Unlock()
	sortOrgs(out, q.Get("sort"), lat, lng, near)
	items, next, more := page(out, q.Get("cursor"), intParam(r, "limit", 20))
	reply(w, http.StatusOK, map[string]any{"items": items, "next": next, "more": more, "total": len(out)})
}

func (a *api) org(w http.ResponseWriter, r *http.Request) {
	a.db.mu.Lock()
	defer a.db.mu.Unlock()
	o := a.db.org(r.PathValue("slug"))
	if o == nil {
		fail(w, http.StatusNotFound, "not_found", "no such organisation")
		return
	}
	reply(w, http.StatusOK, o)
}

func (a *api) orgStatus(w http.ResponseWriter, r *http.Request) {
	a.db.mu.Lock()
	defer a.db.mu.Unlock()
	o := a.db.org(r.PathValue("slug"))
	if o == nil {
		fail(w, http.StatusNotFound, "not_found", "no such organisation")
		return
	}
	open := !o.ComingSoon && (o.SeatsLeft == nil || *o.SeatsLeft > 0)
	reply(w, http.StatusOK, map[string]any{"open": open, "closesAt": o.ClosesAt, "seatsLeft": o.SeatsLeft})
}

func (a *api) me(w http.ResponseWriter, r *http.Request) {
	if u := a.user(w, r); u != nil {
		reply(w, http.StatusOK, u)
	}
}

// register: POST /v1/registrations (auth, Idempotency-Key) → {id, status, passCode}; 409 for the same form twice.
func (a *api) register(w http.ResponseWriter, r *http.Request) {
	u := a.user(w, r)
	if u == nil {
		return
	}
	key := r.Header.Get("Idempotency-Key")
	a.db.mu.Lock()
	if prior, ok := a.db.idempotency[u.ID+"|"+key]; ok && key != "" {
		a.db.mu.Unlock()
		w.Header().Set("Idempotent-Replayed", "true")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write(prior)
		return
	}
	a.db.mu.Unlock()
	var in struct {
		OrgSlug string         `json:"orgSlug"`
		FormKey string         `json:"formKey"`
		Values  map[string]any `json:"values"`
	}
	if !decode(w, r, &in) {
		return
	}
	a.db.mu.Lock()
	defer a.db.mu.Unlock()
	o := a.db.org(in.OrgSlug)
	if o == nil {
		fail(w, http.StatusUnprocessableEntity, "unknown_org", "no such organisation")
		return
	}
	if in.FormKey == "" {
		in.FormKey = o.Slug + "-main"
	}
	for _, x := range a.db.registrations {
		if x.Owner == u.ID && x.FormKey == in.FormKey && x.Status != "withdrawn" {
			fail(w, http.StatusConflict, "duplicate", "you already registered with this form")
			return
		}
	}
	reg := Registration{
		ID: a.db.next("reg"), OrgSlug: o.Slug, OrgName: o.Name, FormKey: in.FormKey, Status: "submitted",
		SubmittedAt: iso(time.Now().UTC()), EventAt: o.ClosesAt, PassCode: fmt.Sprintf("AR-%06d", a.db.seq*7919%1000000),
		Amount: o.Fee, Owner: u.ID, Name: u.Name,
	}
	a.db.registrations = append(a.db.registrations, reg)
	if o.SeatsLeft != nil && *o.SeatsLeft > 0 {
		*o.SeatsLeft--
	}
	body, _ := json.Marshal(map[string]any{"id": reg.ID, "status": reg.Status, "passCode": reg.PassCode})
	if key != "" {
		a.db.idempotency[u.ID+"|"+key] = body
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write(body)
}

// myRegistrations: cursor (cursor, limit) or offset (offset, pageSize) paging, optional status filter.
func (a *api) myRegistrations(w http.ResponseWriter, r *http.Request) {
	u := a.user(w, r)
	if u == nil {
		return
	}
	status := r.URL.Query().Get("status")
	a.db.mu.Lock()
	mine := []Registration{}
	for _, x := range a.db.registrations {
		if x.Owner == u.ID && (status == "" || x.Status == status) {
			mine = append(mine, x)
		}
	}
	a.db.mu.Unlock()
	if r.URL.Query().Has("offset") {
		off := intParam(r, "offset", 0)
		items, _, more := page(mine, strconv.Itoa(off), intParam(r, "pageSize", 25))
		reply(w, http.StatusOK, map[string]any{"items": items, "more": more, "total": len(mine)})
		return
	}
	items, next, more := page(mine, r.URL.Query().Get("cursor"), intParam(r, "limit", 20))
	reply(w, http.StatusOK, map[string]any{"items": items, "next": next, "more": more})
}

func (a *api) findMine(w http.ResponseWriter, r *http.Request) (*User, int) {
	u := a.user(w, r)
	if u == nil {
		return nil, -1
	}
	for i, x := range a.db.registrations {
		if x.ID == r.PathValue("id") && x.Owner == u.ID {
			return u, i
		}
	}
	fail(w, http.StatusNotFound, "not_found", "no such registration")
	return u, -1
}

func (a *api) myRegistration(w http.ResponseWriter, r *http.Request) {
	a.db.mu.Lock()
	defer a.db.mu.Unlock()
	if _, i := a.findMine(w, r); i >= 0 {
		reply(w, http.StatusOK, a.db.registrations[i])
	}
}

func (a *api) withdraw(w http.ResponseWriter, r *http.Request) {
	a.db.mu.Lock()
	defer a.db.mu.Unlock()
	if _, i := a.findMine(w, r); i >= 0 {
		a.db.registrations[i].Status = "withdrawn"
		reply(w, http.StatusOK, a.db.registrations[i])
	}
}

// upload: multipart, at most 10 MiB, PDF or an image → {fileId}.
func (a *api) upload(w http.ResponseWriter, r *http.Request) {
	if a.user(w, r) == nil {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		fail(w, http.StatusRequestEntityTooLarge, "too_large", "files are at most 10 MiB")
		return
	}
	f, h, err := r.FormFile("file")
	if err != nil {
		fail(w, http.StatusBadRequest, "no_file", "send the file in the field \"file\"")
		return
	}
	defer f.Close()
	kind := h.Header.Get("Content-Type")
	if kind != "application/pdf" && !strings.HasPrefix(kind, "image/") {
		fail(w, http.StatusUnsupportedMediaType, "unsupported", "a PDF or an image")
		return
	}
	a.db.mu.Lock()
	id := a.db.next("file")
	a.db.uploads[id] = h.Filename
	a.db.mu.Unlock()
	reply(w, http.StatusCreated, map[string]any{"fileId": id, "name": h.Filename, "size": h.Size})
}

// transfer: an SCA transaction; amounts over 10000 are refused so the error path can be shown.
func (a *api) transfer(w http.ResponseWriter, r *http.Request) {
	if a.user(w, r) == nil {
		return
	}
	var in struct {
		Amount    json.Number       `json:"amount"`
		Currency  string            `json:"currency"`
		To        map[string]string `json:"to"`
		Reference string            `json:"reference"`
	}
	if !decode(w, r, &in) {
		return
	}
	v, err := strconv.ParseFloat(string(in.Amount), 64)
	switch {
	case err != nil || v <= 0:
		fail(w, http.StatusUnprocessableEntity, "invalid_amount", "the amount must be above zero")
	case v > 10000:
		fail(w, http.StatusUnprocessableEntity, "limit_exceeded", "transfers above 10,000 need a branch visit")
	case in.To["iban"] == "":
		fail(w, http.StatusUnprocessableEntity, "invalid_payee", "the payee's IBAN is missing")
	default:
		a.db.mu.Lock()
		id := a.db.next("trf")
		a.db.mu.Unlock()
		reply(w, http.StatusCreated, map[string]any{"id": id, "status": "completed", "amount": map[string]string{"amount": fmt.Sprintf("%.2f", v), "currency": in.Currency}})
	}
}

func (a *api) openAccount(w http.ResponseWriter, r *http.Request) {
	if a.user(w, r) == nil {
		return
	}
	var in map[string]any
	if !decode(w, r, &in) {
		return
	}
	a.db.mu.Lock()
	id := a.db.next("acc")
	a.db.mu.Unlock()
	reply(w, http.StatusCreated, map[string]any{"id": id, "status": "review", "iban": "NO93 8601 1117 947", "message": "We check your documents within one working day."})
}

func (a *api) staff(w http.ResponseWriter, r *http.Request) *User {
	u := a.user(w, r)
	if u == nil {
		return nil
	}
	if u.Role != "staff" && u.Role != "maintainer" {
		fail(w, http.StatusForbidden, "forbidden", "staff only")
		return nil
	}
	if u.Role == "staff" && u.OrgSlug != r.PathValue("slug") && r.PathValue("slug") != "" {
		fail(w, http.StatusForbidden, "forbidden", "another organisation")
		return nil
	}
	return u
}

func (a *api) staffStats(w http.ResponseWriter, r *http.Request) {
	if a.staff(w, r) == nil {
		return
	}
	slug := r.PathValue("slug")
	a.db.mu.Lock()
	byForm := map[string]int{}
	revenue := 0.0
	for _, x := range a.db.registrations {
		if x.OrgSlug == slug {
			byForm[x.FormKey]++
			if x.Amount != nil {
				v, _ := strconv.ParseFloat(x.Amount.Amount, 64)
				revenue += v
			}
		}
	}
	a.db.mu.Unlock()
	forms := []map[string]any{}
	for k, v := range byForm {
		forms = append(forms, map[string]any{"label": k, "value": v})
	}
	daily := []int{12, 18, 9, 22, 30, 27, 35, 31, 40, 38, 44, 29, 51, 47}
	reply(w, http.StatusOK, map[string]any{"daily": daily, "byForm": forms, "revenue": eur(revenue + 1840)})
}

// staffSubmissions: page paging (page, pageSize), status filter, sort=newest|oldest|name.
func (a *api) staffSubmissions(w http.ResponseWriter, r *http.Request) {
	if a.staff(w, r) == nil {
		return
	}
	slug, status := r.PathValue("slug"), r.URL.Query().Get("status")
	a.db.mu.Lock()
	out := []Registration{}
	for _, x := range a.db.registrations {
		if x.OrgSlug == slug && (status == "" || x.Status == status) {
			out = append(out, x)
		}
	}
	a.db.mu.Unlock()
	if r.URL.Query().Get("sort") == "oldest" {
		for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
			out[i], out[j] = out[j], out[i]
		}
	}
	size := intParam(r, "pageSize", 50)
	p := max(intParam(r, "page", 1), 1)
	items, _, more := page(out, strconv.Itoa((p-1)*size), size)
	reply(w, http.StatusOK, map[string]any{"items": items, "page": p, "more": more, "total": len(out)})
}

func (a *api) setStatus(w http.ResponseWriter, r *http.Request) {
	if a.staff(w, r) == nil {
		return
	}
	var in struct{ Status string }
	if !decode(w, r, &in) {
		return
	}
	a.db.mu.Lock()
	defer a.db.mu.Unlock()
	for i, x := range a.db.registrations {
		if x.ID == r.PathValue("id") {
			a.db.registrations[i].Status = in.Status
			a.db.notices = append(a.db.notices, Notice{ID: a.db.next("ntc"), Title: "Registration " + in.Status, Body: x.OrgName + ": your registration is " + in.Status + ".", Route: "pass", Params: map[string]string{"id": x.ID}, At: iso(time.Now().UTC())})
			reply(w, http.StatusOK, a.db.registrations[i])
			return
		}
	}
	fail(w, http.StatusNotFound, "not_found", "no such registration")
}

func (a *api) checkin(w http.ResponseWriter, r *http.Request) {
	if a.staff(w, r) == nil {
		return
	}
	var in struct{ PassCode string }
	if !decode(w, r, &in) {
		return
	}
	a.db.mu.Lock()
	defer a.db.mu.Unlock()
	for i, x := range a.db.registrations {
		if x.PassCode == in.PassCode {
			if x.Status == "done" {
				reply(w, http.StatusConflict, map[string]any{"ok": false, "name": x.Name, "message": "already checked in"})
				return
			}
			a.db.registrations[i].Status = "done"
			reply(w, http.StatusOK, map[string]any{"ok": true, "name": x.Name})
			return
		}
	}
	reply(w, http.StatusNotFound, map[string]any{"ok": false, "message": "no pass with this code"})
}

func (a *api) partnerRequest(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("X-AR-Key") != "dev-partner-key" {
		fail(w, http.StatusUnauthorized, "bad_key", "X-AR-Key is missing or wrong")
		return
	}
	var in map[string]any
	if !decode(w, r, &in) {
		return
	}
	a.db.mu.Lock()
	ref := fmt.Sprintf("PR-%04d", len(a.db.partners)+1)
	in["reference"] = ref
	a.db.partners = append(a.db.partners, in)
	a.db.mu.Unlock()
	reply(w, http.StatusCreated, map[string]any{"reference": ref})
}

func (a *api) products(w http.ResponseWriter, r *http.Request) {
	reply(w, http.StatusOK, a.db.products)
}
func (a *api) listings(w http.ResponseWriter, r *http.Request) {
	reply(w, http.StatusOK, a.db.listings)
}
func (a *api) donations(w http.ResponseWriter, r *http.Request) {
	if a.user(w, r) != nil {
		reply(w, http.StatusOK, a.db.donations)
	}
}

func (a *api) campaign(w http.ResponseWriter, r *http.Request) {
	reply(w, http.StatusOK, map[string]any{
		"goal": eur(120000), "raised": eur(83450),
		"monthly":    []map[string]any{{"label": "Jun", "value": 6200}, {"label": "Jul", "value": 8100}, {"label": "Aug", "value": 11900}, {"label": "Sep", "value": 15400}, {"label": "Oct", "value": 19800}, {"label": "Nov", "value": 22050}},
		"allocation": []map[string]any{{"label": "Water", "value": 40}, {"label": "School kits", "value": 25}, {"label": "Shelter", "value": 25}, {"label": "Running costs", "value": 10}},
		"donors":     []map[string]any{{"name": "Layla", "amount": eur(50)}, {"name": "Omar", "amount": eur(25)}, {"name": "Anonymous", "amount": eur(200)}, {"name": "Nadia", "amount": eur(10)}},
	})
}
