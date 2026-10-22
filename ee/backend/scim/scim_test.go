// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: LicenseRef-Plux-Enterprise

package scim_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nightCode42/plux3/backend/edition"
	"github.com/nightCode42/plux3/backend/ext/identity"
	"github.com/nightCode42/plux3/ee/backend/scim"
)

const (
	userSchema  = "urn:ietf:params:scim:schemas:core:2.0:User"
	groupSchema = "urn:ietf:params:scim:schemas:core:2.0:Group"
	patchSchema = "urn:ietf:params:scim:api:messages:2.0:PatchOp"
)

// licence is a provider with a fixed status.
type licence edition.Status

// Status returns the fixed status.
func (l licence) Status() edition.Status { return edition.Status(l) }

// world is a SCIM server over a directory of two organisations.
type world struct {
	t   *testing.T
	dir *fakeDirectory
	srv http.Handler
	// logs collects what the server logs about unexpected failures.
	logs *bytes.Buffer
	// licence is the status requests carry.
	licence edition.Status
}

// newWorld returns a server with a valid SCIM licence.
func newWorld(t *testing.T) *world {
	t.Helper()
	dir := newDirectory("acme", "globex")
	logs := &bytes.Buffer{}
	svc := scim.New(scim.Options{Log: slog.New(slog.NewTextHandler(logs, nil))})
	return &world{
		t: t, dir: dir, srv: svc.Handler(dir), logs: logs,
		licence: edition.Status{State: edition.StateValid, Features: []edition.Feature{edition.FeatureSCIM}},
	}
}

// reply is a response with its JSON body decoded.
type reply struct {
	*http.Response
	body map[string]any
	raw  string
}

// request sends a request, as the core's mount would pass it: the prefix
// stripped, the licence in the context.
func (w *world) request(method, path, token, body string, headers ...string) reply {
	w.t.Helper()
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req := httptest.NewRequestWithContext(edition.WithProvider(context.Background(), licence(w.licence)), method, path, rd)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/scim+json")
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	rec := httptest.NewRecorder()
	w.srv.ServeHTTP(rec, req)
	res := rec.Result()
	raw, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	out := reply{Response: res, raw: string(raw)}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &out.body); err != nil {
			w.t.Fatalf("%s %s: the body is not a JSON object: %v\n%s", method, path, err, raw)
		}
	}
	return out
}

// as sends a request with the SCIM token of an organisation.
func (w *world) as(org, method, path, body string, headers ...string) reply {
	w.t.Helper()
	return w.request(method, path, "token-"+org, body, headers...)
}

// expect fails the test unless the reply has the status.
func (r reply) expect(t *testing.T, status int) reply {
	t.Helper()
	if r.StatusCode != status {
		t.Fatalf("status = %d, want %d: %s", r.StatusCode, status, r.raw)
	}
	return r
}

// problem fails the test unless the reply is a SCIM error of the status and
// type whose detail starts with the Plux code.
func (r reply) problem(t *testing.T, status int, scimType, code string) {
	t.Helper()
	r.expect(t, status)
	detail, _ := r.body["detail"].(string)
	if r.body["status"] != itoa(status) || r.body["scimType"] != orNil(scimType) || !strings.HasPrefix(detail, code) {
		t.Errorf("error = %s, want %d %q %s", r.raw, status, scimType, code)
	}
	if got := r.Header.Get("Content-Type"); got != "application/scim+json" {
		t.Errorf("Content-Type = %q", got)
	}
	if schemas, _ := r.body["schemas"].([]any); len(schemas) != 1 || schemas[0] != identity.SCIMErrorSchema {
		t.Errorf("schemas = %v", r.body["schemas"])
	}
}

func itoa(n int) string { b, _ := json.Marshal(n); return string(b) }

func orNil(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// str reads a string attribute at a path of keys.
func (r reply) str(keys ...string) string {
	var v any = r.body
	for _, k := range keys {
		m, _ := v.(map[string]any)
		v = m[k]
	}
	s, _ := v.(string)
	return s
}

// userBody is a create or replace body.
func userBody(name, extra string) string {
	if extra != "" {
		extra = "," + extra
	}
	return `{"schemas":["` + userSchema + `"],"userName":"` + name + `"` + extra + `}`
}

// patchBody is a PATCH body of operations.
func patchBody(ops ...string) string {
	return `{"schemas":["` + patchSchema + `"],"Operations":[` + strings.Join(ops, ",") + `]}`
}

// createUser makes a user and returns its id.
func (w *world) createUser(name string) string {
	w.t.Helper()
	return w.as("acme", "POST", "/Users", userBody(name, `"displayName":"`+name+`","externalId":"ext-`+name+`"`)).expect(w.t, 201).str("id")
}

// Verifies: GOV-004.
// A user is created with a 201, a Location and an ETag, rendered with its
// email, name and meta; the same user name again is a uniqueness conflict.
func TestCreateUser_GOV_004(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	r := w.as("acme", "POST", "/Users", userBody("ada@acme.example", `"externalId":"idp-1","displayName":"Ada","active":true`)).expect(t, 201)
	id := r.str("id")
	if r.Header.Get("Location") != "/scim/v2/Users/"+id || r.Header.Get("ETag") != `W/"1"` || r.str("meta", "version") != `W/"1"` {
		t.Errorf("headers = %v, meta = %v", r.Header, r.body["meta"])
	}
	if r.str("userName") != "ada@acme.example" || r.str("externalId") != "idp-1" || r.str("displayName") != "Ada" || r.body["active"] != true {
		t.Errorf("user = %s", r.raw)
	}
	if r.str("meta", "resourceType") != "User" || r.str("meta", "location") != "/scim/v2/Users/"+id || r.Header.Get("Content-Type") != "application/scim+json" {
		t.Errorf("meta = %v", r.body["meta"])
	}
	if emails, _ := r.body["emails"].([]any); len(emails) != 1 {
		t.Errorf("emails = %v", r.body["emails"])
	}
	w.as("acme", "POST", "/Users", userBody("ADA@acme.example", "")).problem(t, 409, "uniqueness", "PLX-6220")
	w.as("globex", "POST", "/Users", userBody("ada@acme.example", "")).expect(t, 201)
}

// Verifies: GOV-004.
// A request the server cannot accept is a 400 with a SCIM error whose detail
// begins PLX-6220: a body that is not JSON, one of the wrong schema, a missing
// user name, a value the directory refuses.
func TestCreateUser_Invalid_GOV_004(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	for name, c := range map[string]struct{ body, scimType string }{
		"not json":         {"{not json", "invalidSyntax"},
		"trailing content": {userBody("a@acme.example", "") + `{}`, "invalidSyntax"},
		"wrong schema":     {`{"schemas":["urn:other"],"userName":"a@acme.example"}`, "invalidSyntax"},
		"no schemas":       {`{"userName":"a@acme.example"}`, "invalidSyntax"},
		"no user name":     {`{"schemas":["` + userSchema + `"]}`, "invalidValue"},
		"a blank name":     {userBody("  ", ""), "invalidValue"},
		"active not bool":  {userBody("a@acme.example", `"active":"maybe"`), "invalidSyntax"},
		"not an address":   {userBody("not-an-address", ""), "invalidValue"},
	} {
		t.Log(name)
		r := w.as("acme", "POST", "/Users", c.body)
		r.problem(t, 400, c.scimType, "PLX-6220")
	}
	if n := len(w.dir.users); n != 0 {
		t.Errorf("%d users were made from invalid requests", n)
	}
}

// Verifies: GOV-004, SEC-103.
// Every request needs a valid bearer token of an organisation, including a
// request for a path that does not exist, and an organisation sees only its
// own resources.
func TestAuthentication_GOV_004(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	id := w.createUser("ada@acme.example")
	for name, headers := range map[string][]string{
		"no header":      nil,
		"basic":          {"Authorization", "Basic dXNlcjpwYXNz"},
		"empty bearer":   {"Authorization", "Bearer "},
		"unknown token":  {"Authorization", "Bearer token-nobody"},
		"a user's token": {"Authorization", "Bearer personal"},
	} {
		for _, path := range []string{"/Users", "/Users/" + id, "/Nowhere"} {
			r := w.request("GET", path, "", "", headers...)
			r.problem(t, 401, "", "PLX-6220")
			if r.Header.Get("WWW-Authenticate") == "" {
				t.Errorf("%s %s: no WWW-Authenticate", name, path)
			}
		}
	}
	w.as("globex", "GET", "/Users/"+id, "").problem(t, 404, "", "PLX-6220")
	w.as("globex", "PUT", "/Users/"+id, userBody("x@acme.example", "")).problem(t, 404, "", "PLX-6220")
	w.as("globex", "DELETE", "/Users/"+id, "").problem(t, 404, "", "PLX-6220")
	w.as("acme", "GET", "/Users/"+id, "").expect(t, 200)
	if r := w.as("globex", "GET", "/Users", "").expect(t, 200); r.body["totalResults"] != float64(0) {
		t.Errorf("globex sees %s", r.raw)
	}
}

// Verifies: GOV-004.
// Filters are an equality on a searchable attribute; anything else is an
// invalidFilter error.
func TestFilter_GOV_004(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	ada := w.createUser("ada@acme.example")
	w.createUser("bob@acme.example")
	list := func(filter string) reply {
		return w.as("acme", "GET", "/Users?filter="+urlEscape(filter), "")
	}
	for filter, want := range map[string]string{
		`userName eq "ada@acme.example"`:               ada,
		`userName eq "ADA@ACME.EXAMPLE"`:               ada,
		`USERNAME EQ "ada@acme.example"`:               ada,
		`externalId eq "ext-ada@acme.example"`:         ada,
		`displayName eq "ada@acme.example"`:            ada,
		userSchema + `:userName eq "ada@acme.example"`: ada,
		`userName eq "ada@acme.example" `:              ada,
		`userName eq "nobody@acme.example"`:            "",
		`userName eq "a \"quoted\" name"`:              "",
	} {
		r := list(filter).expect(t, 200)
		resources, _ := r.body["Resources"].([]any)
		if want == "" && len(resources) != 0 || want != "" && (len(resources) != 1 || resources[0].(map[string]any)["id"] != want) {
			t.Errorf("filter %q: %s", filter, r.raw)
		}
	}
	for _, filter := range []string{
		`userName co "ada"`, `userName eq ada@acme.example`, `userName eq`, `eq "x"`, `nonsense`,
		`userName eq "a" and displayName eq "b"`, `emails.value eq "a@acme.example"`, `active eq "true"`, `userName eq "a" or userName eq "b"`,
		`userName pr`, `userName eq "unterminated`,
	} {
		list(filter).problem(t, 400, "invalidFilter", "PLX-6220")
	}
	w.as("acme", "GET", "/Groups?filter="+urlEscape(`userName eq "x"`), "").problem(t, 400, "invalidFilter", "PLX-6220")
}

// urlEscape escapes a query value.
func urlEscape(s string) string {
	return strings.NewReplacer("%", "%25", " ", "%20", `"`, "%22", "&", "%26", "+", "%2B", "#", "%23").Replace(s)
}

// Verifies: GOV-004.
// A list is paged with startIndex and count, clamped to what the directory
// allows; count 0 returns the total alone; a page parameter that is not a
// number is refused.
func TestPagination_GOV_004(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	for _, n := range []string{"a", "b", "c", "d", "e"} {
		w.createUser(n + "@acme.example")
	}
	names := func(r reply) []string {
		var out []string
		for _, u := range r.body["Resources"].([]any) {
			out = append(out, u.(map[string]any)["userName"].(string)[:1])
		}
		return out
	}
	for query, want := range map[string]string{
		"":                       "abc",
		"?count=2":               "ab",
		"?startIndex=2&count=2":  "bc",
		"?startIndex=4":          "de",
		"?startIndex=9":          "",
		"?startIndex=0&count=1":  "a",
		"?startIndex=-3&count=1": "a",
		"?count=99":              "abc",
		"?count=-1":              "",
		"?count=0":               "",
	} {
		r := w.as("acme", "GET", "/Users"+query, "").expect(t, 200)
		if got := strings.Join(names(r), ""); got != want {
			t.Errorf("%q: got %q, want %q", query, got, want)
		}
		if r.body["totalResults"] != float64(5) || r.body["itemsPerPage"] != float64(len(want)) {
			t.Errorf("%q: %v of %v", query, r.body["itemsPerPage"], r.body["totalResults"])
		}
		if schemas := r.body["schemas"].([]any); schemas[0] != "urn:ietf:params:scim:api:messages:2.0:ListResponse" {
			t.Errorf("schemas = %v", schemas)
		}
	}
	w.as("acme", "GET", "/Users?count=lots", "").problem(t, 400, "invalidValue", "PLX-6220")
	w.as("acme", "GET", "/Users?startIndex=one", "").problem(t, 400, "invalidValue", "PLX-6220")
}

// Verifies: GOV-004.
// ETags are the weak version: a replace or delete that names a stale version
// is a 412 and changes nothing, one that names the current version goes
// through, and a conditional GET of the current version is a 304.
func TestETags_GOV_004(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	id := w.createUser("ada@acme.example")
	body := userBody("ada@acme.example", `"displayName":"Ada Lovelace"`)
	w.as("acme", "PUT", "/Users/"+id, body, "If-Match", `W/"7"`).problem(t, 412, "", "PLX-6220")
	w.as("acme", "PATCH", "/Users/"+id, patchBody(`{"op":"replace","path":"displayName","value":"Changed"}`), "If-Match", `"7"`).problem(t, 412, "", "PLX-6220")
	w.as("acme", "DELETE", "/Users/"+id, "", "If-Match", `W/"7"`).problem(t, 412, "", "PLX-6220")
	if got := w.as("acme", "GET", "/Users/"+id, "").str("displayName"); got != "ada@acme.example" {
		t.Fatalf("a refused change was applied: %q", got)
	}
	r := w.as("acme", "PUT", "/Users/"+id, body, "If-Match", `W/"1"`).expect(t, 200)
	if r.str("displayName") != "Ada Lovelace" || r.Header.Get("ETag") != `W/"2"` {
		t.Errorf("replace = %s, ETag %s", r.raw, r.Header.Get("ETag"))
	}
	w.as("acme", "PUT", "/Users/"+id, body, "If-Match", `W/"1"`).problem(t, 412, "", "PLX-6220")
	w.as("acme", "PUT", "/Users/"+id, body, "If-Match", "*").expect(t, 200)
	if r := w.as("acme", "GET", "/Users/"+id, "", "If-None-Match", `W/"3"`); r.StatusCode != 304 || r.raw != "" || r.Header.Get("ETag") != `W/"3"` {
		t.Errorf("conditional GET = %d %q", r.StatusCode, r.raw)
	}
	w.as("acme", "GET", "/Users/"+id, "", "If-None-Match", `W/"2"`).expect(t, 200)
	w.as("acme", "DELETE", "/Users/"+id, "", "If-Match", `W/"3"`).expect(t, 204)
}

// Verifies: GOV-004, GOV-005.
// PATCH replaces one attribute or several in the forms identity providers
// send; setting active to false deprovisions the person: the directory ends
// their tokens and sessions, and the token stops working at once.
func TestPatchUser_GOV_004(t *testing.T) {
	t.Parallel()
	for name, op := range map[string]string{
		"a path and a boolean":    `{"op":"replace","path":"active","value":false}`,
		"an upper-case operation": `{"op":"Replace","path":"active","value":false}`,
		"a string boolean":        `{"op":"Replace","path":"active","value":"False"}`,
		"no path":                 `{"op":"replace","value":{"active":false}}`,
		"a schema-qualified path": `{"op":"replace","path":"` + userSchema + `:active","value":false}`,
		"an add of the attribute": `{"op":"add","path":"active","value":false}`,
	} {
		w := newWorld(t)
		id := w.createUser("ada@acme.example")
		w.dir.signIn(id, "ada-api-token")
		r := w.as("acme", "PATCH", "/Users/"+id, patchBody(op)).expect(t, 200)
		if r.body["active"] != false || r.str("meta", "version") != `W/"2"` {
			t.Errorf("%s: %s", name, r.raw)
		}
		if tokens, sessions := w.dir.access(id); tokens != 0 || sessions != 0 {
			t.Errorf("%s: %d tokens and %d sessions survive deprovisioning", name, tokens, sessions)
		}
		if _, err := w.dir.Authenticate(context.Background(), "ada-api-token"); !errors.Is(err, identity.ErrUnauthorized) {
			t.Errorf("%s: the person's token still works: %v", name, err)
		}
		w.request("GET", "/Users", "ada-api-token", "").problem(t, 401, "", "PLX-6220")
	}
}

// Verifies: GOV-004.
// PATCH changes the other attributes, removes optional ones, accepts and
// ignores the attributes of the core schema Plux does not keep, and refuses
// the rest, without applying any of an invalid request.
func TestPatchUser_Attributes_GOV_004(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	id := w.createUser("ada@acme.example")
	patch := func(ops ...string) reply { return w.as("acme", "PATCH", "/Users/"+id, patchBody(ops...)) }
	r := patch(
		`{"op":"replace","path":"displayName","value":"Ada L."}`,
		`{"op":"add","path":"externalId","value":"idp-9"}`,
		`{"op":"replace","path":"name.givenName","value":"Ada"}`,
		`{"op":"replace","path":"emails[type eq \"work\"].value","value":"other@acme.example"}`,
		`{"op":"replace","value":{"title":"Engineer","userName":"ada2@acme.example"}}`,
	).expect(t, 200)
	if r.str("displayName") != "Ada L." || r.str("externalId") != "idp-9" || r.str("userName") != "ada2@acme.example" {
		t.Errorf("patched = %s", r.raw)
	}
	r = patch(`{"op":"remove","path":"externalId"}`, `{"op":"remove","path":"displayName"}`).expect(t, 200)
	if r.str("externalId") != "" || r.str("displayName") != "" {
		t.Errorf("removed = %s", r.raw)
	}
	before := r.str("meta", "version")
	for name, c := range map[string]struct{ body, scimType string }{
		"an attribute Plux has not":      {patchBody(`{"op":"replace","path":"favouriteColour","value":"red"}`), "invalidPath"},
		"a bad filter in a path":         {patchBody(`{"op":"remove","path":"members[value eq"}`), "invalidPath"},
		"remove the user name":           {patchBody(`{"op":"remove","path":"userName"}`), "mutability"},
		"remove active":                  {patchBody(`{"op":"remove","path":"active"}`), "mutability"},
		"an empty user name":             {patchBody(`{"op":"replace","path":"userName","value":""}`), "invalidValue"},
		"a number for the name":          {patchBody(`{"op":"replace","path":"displayName","value":7}`), "invalidValue"},
		"active that is not a boolean":   {patchBody(`{"op":"replace","path":"active","value":"perhaps"}`), "invalidValue"},
		"remove with no path":            {patchBody(`{"op":"remove"}`), "noTarget"},
		"no path and no object":          {patchBody(`{"op":"replace","value":"x"}`), "invalidValue"},
		"an unknown operation":           {patchBody(`{"op":"move","path":"displayName","value":"x"}`), "invalidSyntax"},
		"no operations":                  {patchBody(), "invalidSyntax"},
		"the wrong schema":               {`{"schemas":["urn:other"],"Operations":[{"op":"add","path":"displayName","value":"x"}]}`, "invalidSyntax"},
		"a good operation and a bad one": {patchBody(`{"op":"replace","path":"displayName","value":"Applied?"}`, `{"op":"replace","path":"nope","value":"x"}`), "invalidPath"},
	} {
		t.Log(name)
		w.as("acme", "PATCH", "/Users/"+id, c.body).problem(t, 400, c.scimType, "PLX-6220")
	}
	if got := w.as("acme", "GET", "/Users/"+id, "").str("meta", "version"); got != before {
		t.Errorf("version %s after refused patches, was %s", got, before)
	}
}

// Verifies: GOV-004.
// DELETE removes a user, and ends their access first.
func TestDeleteUser_GOV_004(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	id := w.createUser("ada@acme.example")
	w.dir.signIn(id, "ada-api-token")
	if r := w.as("acme", "DELETE", "/Users/"+id, "").expect(t, 204); r.raw != "" {
		t.Errorf("body = %q", r.raw)
	}
	if tokens, sessions := w.dir.access(id); tokens != 0 || sessions != 0 {
		t.Errorf("%d tokens and %d sessions survive deletion", tokens, sessions)
	}
	w.as("acme", "GET", "/Users/"+id, "").problem(t, 404, "", "PLX-6220")
	w.as("acme", "DELETE", "/Users/"+id, "").problem(t, 404, "", "PLX-6220")
	w.as("acme", "GET", "/Users/not-an-id", "").problem(t, 404, "", "PLX-6220")
}

// Verifies: GOV-004.
// A group is a team: it is created with members, rendered with them, and a
// user shows the groups they are in.
func TestGroups_GOV_004(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	ada, bob := w.createUser("ada@acme.example"), w.createUser("bob@acme.example")
	body := `{"schemas":["` + groupSchema + `"],"displayName":"Engineering","externalId":"g-1","members":[{"value":"` + ada + `"},{"value":"` + ada + `"}]}`
	r := w.as("acme", "POST", "/Groups", body).expect(t, 201)
	id := r.str("id")
	members := r.body["members"].([]any)
	if len(members) != 1 || members[0].(map[string]any)["value"] != ada || members[0].(map[string]any)["$ref"] != "/scim/v2/Users/"+ada {
		t.Errorf("members = %v", members)
	}
	if r.Header.Get("Location") != "/scim/v2/Groups/"+id || r.str("meta", "resourceType") != "Group" || r.str("displayName") != "Engineering" {
		t.Errorf("group = %s", r.raw)
	}
	groups := w.as("acme", "GET", "/Users/"+ada, "").body["groups"].([]any)
	if len(groups) != 1 || groups[0].(map[string]any)["value"] != id || groups[0].(map[string]any)["display"] != "Engineering" {
		t.Errorf("the user's groups = %v", groups)
	}
	for name, c := range map[string]struct{ body, scimType string }{
		"no name":        {`{"schemas":["` + groupSchema + `"]}`, "invalidValue"},
		"a stranger":     {`{"schemas":["` + groupSchema + `"],"displayName":"X","members":[{"value":"user-9999"}]}`, "invalidValue"},
		"a blank member": {`{"schemas":["` + groupSchema + `"],"displayName":"X","members":[{"value":""}]}`, "invalidValue"},
		"wrong schema":   {`{"schemas":["` + userSchema + `"],"displayName":"X"}`, "invalidSyntax"},
	} {
		t.Log(name)
		w.as("acme", "POST", "/Groups", c.body).problem(t, 400, c.scimType, "PLX-6220")
	}
	w.as("acme", "POST", "/Groups", body).problem(t, 409, "uniqueness", "PLX-6220")
	list := w.as("acme", "GET", "/Groups?filter="+urlEscape(`displayName eq "Engineering"`), "").expect(t, 200)
	if list.body["totalResults"] != float64(1) {
		t.Errorf("list = %s", list.raw)
	}
	put := `{"schemas":["` + groupSchema + `"],"displayName":"Platform","members":[{"value":"` + bob + `"}]}`
	r = w.as("acme", "PUT", "/Groups/"+id, put).expect(t, 200)
	if r.str("displayName") != "Platform" || len(r.body["members"].([]any)) != 1 || r.str("meta", "version") != `W/"2"` {
		t.Errorf("replaced = %s", r.raw)
	}
	w.as("acme", "PUT", "/Groups/"+id, put, "If-Match", `W/"1"`).problem(t, 412, "", "PLX-6220")
	w.as("globex", "GET", "/Groups/"+id, "").problem(t, 404, "", "PLX-6220")
	w.as("acme", "DELETE", "/Groups/"+id, "").expect(t, 204)
	w.as("acme", "GET", "/Groups/"+id, "").problem(t, 404, "", "PLX-6220")
}

// Verifies: GOV-004.
// Group PATCH adds, removes and replaces members in the forms identity
// providers send, and changes the name.
func TestPatchGroup_GOV_004(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	ids := []string{w.createUser("a@acme.example"), w.createUser("b@acme.example"), w.createUser("c@acme.example")}
	group := w.as("acme", "POST", "/Groups", `{"schemas":["`+groupSchema+`"],"displayName":"Team"}`).expect(t, 201).str("id")
	member := func(id string) string { return `{"value":"` + id + `"}` }
	current := func() string {
		g := w.as("acme", "GET", "/Groups/"+group, "").expect(t, 200)
		var out []string
		for _, m := range g.body["members"].([]any) {
			out = append(out, m.(map[string]any)["value"].(string))
		}
		return strings.Join(out, ",")
	}
	patch := func(ops ...string) reply { return w.as("acme", "PATCH", "/Groups/"+group, patchBody(ops...)) }

	patch(`{"op":"add","path":"members","value":[`+member(ids[0])+`,`+member(ids[1])+`]}`).expect(t, 200)
	patch(`{"op":"Add","path":"members","value":[`+member(ids[1])+`,`+member(ids[2])+`]}`).expect(t, 200)
	if got, want := current(), strings.Join(ids, ","); got != want {
		t.Errorf("after adds: %s, want %s", got, want)
	}
	patch(`{"op":"remove","path":"members[value eq \"`+ids[1]+`\"]"}`).expect(t, 200)
	if got, want := current(), ids[0]+","+ids[2]; got != want {
		t.Errorf("after a filtered remove: %s, want %s", got, want)
	}
	patch(`{"op":"remove","path":"members","value":[`+member(ids[0])+`]}`).expect(t, 200)
	if got := current(); got != ids[2] {
		t.Errorf("after a remove by value: %s, want %s", got, ids[2])
	}
	patch(`{"op":"replace","path":"members","value":[`+member(ids[0])+`]}`).expect(t, 200)
	if got := current(); got != ids[0] {
		t.Errorf("after a replace: %s, want %s", got, ids[0])
	}
	patch(`{"op":"add","value":{"members":[`+member(ids[1])+`],"displayName":"Renamed"}}`).expect(t, 200)
	if got := current(); got != ids[0]+","+ids[1] {
		t.Errorf("after an add with no path: %s", got)
	}
	patch(`{"op":"remove","path":"members"}`).expect(t, 200)
	if got := current(); got != "" {
		t.Errorf("after removing all: %q", got)
	}
	if r := w.as("acme", "GET", "/Groups/"+group, ""); r.str("displayName") != "Renamed" {
		t.Errorf("name = %q", r.str("displayName"))
	}
	patch(`{"op":"add","path":"members","value":[{"value":"user-9999"}]}`).problem(t, 400, "invalidValue", "PLX-6220")
	patch(`{"op":"add","path":"members","value":"nobody"}`).problem(t, 400, "invalidValue", "PLX-6220")
	patch(`{"op":"replace","path":"members[value eq \"x\"]","value":[]}`).problem(t, 400, "invalidPath", "PLX-6220")
	patch(`{"op":"remove","path":"members[display eq \"x\"]"}`).problem(t, 400, "invalidPath", "PLX-6220")
	patch(`{"op":"replace","path":"displayName","value":""}`).problem(t, 400, "invalidValue", "PLX-6220")
	patch(`{"op":"replace","path":"colour","value":"red"}`).problem(t, 400, "invalidPath", "PLX-6220")
}

// Verifies: GOV-004, GOV-033.
// Writes need the licence to allow SCIM to be written; reads follow the
// core's check, which the server does not repeat.
func TestWritesAreLicensed_GOV_033(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	id := w.createUser("ada@acme.example")
	for name, status := range map[string]edition.Status{
		"no licence":         {State: edition.StateAbsent},
		"another feature":    {State: edition.StateValid, Features: []edition.Feature{edition.FeatureSAML}},
		"an expired licence": {State: edition.StateExpired, Features: []edition.Feature{edition.FeatureSCIM}},
	} {
		t.Log(name)
		w.licence = status
		for _, call := range [][3]string{
			{"POST", "/Users", userBody("new@acme.example", "")},
			{"PUT", "/Users/" + id, userBody("ada@acme.example", "")},
			{"PATCH", "/Users/" + id, patchBody(`{"op":"replace","path":"active","value":false}`)},
			{"DELETE", "/Users/" + id, ""},
			{"POST", "/Groups", `{"schemas":["` + groupSchema + `"],"displayName":"G"}`},
		} {
			w.as("acme", call[0], call[1], call[2]).problem(t, 403, "", "PLX-8600")
		}
		w.as("acme", "GET", "/Users/"+id, "").expect(t, 200)
	}
	if u := w.dir.users[id]; !u.Active || u.Version != 1 || len(w.dir.users) != 1 || len(w.dir.groups) != 0 {
		t.Errorf("an unlicensed write changed the directory: %+v", u)
	}
	w.as("acme", "POST", "/Users", userBody("new@acme.example", "")).problem(t, 403, "", "PLX-8600")
}

// Verifies: GOV-004.
// The discovery endpoints describe what the server does: patch, filter,
// ETags and a bearer token; no bulk, sorting or password change.
func TestDiscovery_GOV_004(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	cfg := w.as("acme", "GET", "/ServiceProviderConfig", "").expect(t, 200)
	flag := func(k string) any { return cfg.body[k].(map[string]any)["supported"] }
	if flag("patch") != true || flag("etag") != true || flag("filter") != true || flag("bulk") != false || flag("sort") != false || flag("changePassword") != false {
		t.Errorf("config = %s", cfg.raw)
	}
	if cfg.body["filter"].(map[string]any)["maxResults"] != float64(3) {
		t.Errorf("filter = %v", cfg.body["filter"])
	}
	if schemes := cfg.body["authenticationSchemes"].([]any); len(schemes) != 1 || schemes[0].(map[string]any)["type"] != "oauthbearertoken" {
		t.Errorf("authenticationSchemes = %v", cfg.body["authenticationSchemes"])
	}
	list := w.as("acme", "GET", "/Schemas", "").expect(t, 200)
	if list.body["totalResults"] != float64(2) {
		t.Errorf("schemas = %s", list.raw)
	}
	for _, id := range []string{userSchema, groupSchema} {
		if r := w.as("acme", "GET", "/Schemas/"+id, "").expect(t, 200); r.str("id") != id {
			t.Errorf("schema = %s", r.raw)
		}
	}
	w.as("acme", "GET", "/Schemas/urn:nope", "").problem(t, 404, "", "PLX-6220")
	types := w.as("acme", "GET", "/ResourceTypes", "").expect(t, 200)
	if types.body["totalResults"] != float64(2) {
		t.Errorf("resource types = %s", types.raw)
	}
	if r := w.as("acme", "GET", "/ResourceTypes/User", "").expect(t, 200); r.str("endpoint") != "/Users" || r.str("schema") != userSchema {
		t.Errorf("resource type = %s", r.raw)
	}
	w.as("acme", "GET", "/ResourceTypes/Widget", "").problem(t, 404, "", "PLX-6220")
}

// Verifies: GOV-004.
// An unknown path is a SCIM 404, a known path with the wrong method a 405
// that lists the methods it serves, and a write to a read-only endpoint is
// refused.
func TestRouting_GOV_004(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	id := w.createUser("ada@acme.example")
	for _, path := range []string{"/", "/Nowhere", "/Users/a/b", "/Users//x", "/users", "/Users/" + id + "/emails"} {
		w.as("acme", "GET", path, "").problem(t, 404, "", "PLX-6220")
	}
	r := w.as("acme", "PUT", "/Users", userBody("x@acme.example", ""))
	r.problem(t, 405, "", "PLX-6220")
	if r.Header.Get("Allow") != "GET, POST" {
		t.Errorf("Allow = %q", r.Header.Get("Allow"))
	}
	w.as("acme", "POST", "/Users/"+id, userBody("x@acme.example", "")).problem(t, 405, "", "PLX-6220")
	w.as("acme", "POST", "/Schemas", "{}").problem(t, 405, "", "PLX-6220")
	w.as("acme", "DELETE", "/ServiceProviderConfig", "").problem(t, 405, "", "PLX-6220")
}

// Verifies: GOV-004.
// An unexpected failure of the directory is a 500 that says nothing of its
// cause to the client and is logged.
func TestUnexpectedFailure_GOV_004(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	id := w.createUser("ada@acme.example")
	w.dir.failWith = errors.New("pq: connection to 10.0.0.7 refused, password=hunter2")
	r := w.as("acme", "GET", "/Users/"+id, "")
	r.expect(t, 500)
	if strings.Contains(r.raw, "10.0.0.7") || strings.Contains(r.raw, "hunter2") || r.str("detail") != "the request could not be processed" {
		t.Errorf("the client was told: %s", r.raw)
	}
	if !strings.Contains(w.logs.String(), "10.0.0.7") {
		t.Errorf("the failure was not logged: %q", w.logs.String())
	}
}
