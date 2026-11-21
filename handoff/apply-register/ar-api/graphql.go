package main

import (
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

// graphql answers the five queries campus and devconf send. It is not a GraphQL engine: it finds the root field the
// query asks for and returns whole objects, which every client reads the fields it selected from.
func (a *api) graphql(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Query     string         `json:"query"`
		Variables map[string]any `json:"variables"`
	}
	if !decode(w, r, &in) {
		return
	}
	vars := in.Variables
	if vars == nil {
		vars = map[string]any{}
	}
	arg := func(name string) string {
		if v, ok := vars[name]; ok && v != nil {
			switch t := v.(type) {
			case string:
				return t
			case float64:
				return strconv.Itoa(int(t))
			}
		}
		if m := regexp.MustCompile(name + `\s*:\s*"?([^",)\s]+)"?`).FindStringSubmatch(in.Query); m != nil && !strings.HasPrefix(m[1], "$") {
			return m[1]
		}
		return ""
	}
	has := func(field string) bool { return regexp.MustCompile(`\b` + field + `\s*[({]`).MatchString(in.Query) }
	data := map[string]any{}
	a.db.mu.Lock()
	defer a.db.mu.Unlock()
	switch {
	case has("programmes"):
		q := strings.ToLower(arg("q"))
		list := []Programme{}
		for _, p := range a.db.programmes {
			if q == "" || strings.Contains(strings.ToLower(p.Name+" "+p.Faculty), q) {
				list = append(list, p)
			}
		}
		items, next, more := page(list, arg("cursor"), 10)
		data["programmes"] = map[string]any{"items": items, "next": next, "more": more}
	case has("programme"):
		for _, p := range a.db.programmes {
			if p.ID == arg("id") {
				data["programme"] = p
			}
		}
		if data["programme"] == nil {
			data["programme"] = nil
		}
	case has("agenda"):
		day, _ := strconv.Atoi(arg("day"))
		list := []Session{}
		for _, s := range a.db.sessions {
			if s.Day == day {
				list = append(list, s)
			}
		}
		data["agenda"] = list
	case has("speakers"):
		data["speakers"] = a.db.speakers
	case has("workshops"):
		data["workshops"] = a.db.workshops
	default:
		reply(w, http.StatusOK, map[string]any{"errors": []map[string]string{{"message": "unknown query: ask programmes, programme, agenda, speakers or workshops"}}})
		return
	}
	reply(w, http.StatusOK, map[string]any{"data": data})
}
