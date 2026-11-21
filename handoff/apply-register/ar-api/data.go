package main

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"
)

// Money is how every amount travels: a decimal string and an ISO 4217 code.
type Money struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
}

func eur(v float64) *Money { return &Money{Amount: fmt.Sprintf("%.2f", v), Currency: "EUR"} }

type Category struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Icon  string `json:"icon"`
}

type Org struct {
	Slug       string   `json:"slug"`
	Name       string   `json:"name"`
	Category   string   `json:"category"`
	City       string   `json:"city"`
	Lat        float64  `json:"lat"`
	Lng        float64  `json:"lng"`
	Logo       string   `json:"logo"`
	Cover      string   `json:"cover"`
	ClosesAt   *string  `json:"closesAt"`
	SeatsLeft  *int     `json:"seatsLeft"`
	Fee        *Money   `json:"fee"`
	Verified   bool     `json:"verified"`
	Tags       []string `json:"tags"`
	Summary    string   `json:"summary"`
	Accent     string   `json:"accent"`
	ComingSoon bool     `json:"comingSoon"`
}

type Registration struct {
	ID          string  `json:"id"`
	OrgSlug     string  `json:"orgSlug"`
	OrgName     string  `json:"orgName"`
	FormKey     string  `json:"formKey"`
	Status      string  `json:"status"`
	SubmittedAt string  `json:"submittedAt"`
	EventAt     *string `json:"eventAt"`
	PassCode    string  `json:"passCode"`
	Amount      *Money  `json:"amount"`
	Pinned      bool    `json:"pinned"`
	Owner       string  `json:"-"`
	Name        string  `json:"name"`
}

type Notice struct {
	ID     string            `json:"id"`
	Title  string            `json:"title"`
	Body   string            `json:"body"`
	Route  string            `json:"route"`
	Params map[string]string `json:"params"`
	At     string            `json:"at"`
	Read   bool              `json:"read"`
}

type Programme struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Level    string `json:"level"`
	Fee      Money  `json:"fee"`
	Faculty  string `json:"faculty"`
	Duration string `json:"duration"`
	Seats    int    `json:"seats"`
	Summary  string `json:"summary"`
}

type Session struct {
	ID      string `json:"id"`
	Day     int    `json:"day"`
	Title   string `json:"title"`
	Speaker string `json:"speaker"`
	Room    string `json:"room"`
	Starts  string `json:"starts"`
	Ends    string `json:"ends"`
	Track   string `json:"track"`
}

type Speaker struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Role    string `json:"role"`
	Company string `json:"company"`
	Avatar  string `json:"avatar"`
	Bio     string `json:"bio"`
}

type Workshop struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Capacity int    `json:"capacity"`
	Taken    int    `json:"taken"`
	Starts   string `json:"starts"`
	Level    string `json:"level"`
}

type Product struct {
	SKU     string `json:"sku"`
	Title   string `json:"title"`
	Kind    string `json:"kind"`
	Price   Money  `json:"price"`
	Image   string `json:"image"`
	Badge   string `json:"badge"`
	InStock bool   `json:"inStock"`
}

type Listing struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Area      string   `json:"area"`
	Rent      Money    `json:"rent"`
	Rooms     int      `json:"rooms"`
	SizeM2    int      `json:"sizeM2"`
	Image     string   `json:"image"`
	Lat       float64  `json:"lat"`
	Lng       float64  `json:"lng"`
	Features  []string `json:"features"`
	Available string   `json:"available"`
}

type Donation struct {
	ID     string `json:"id"`
	Amount Money  `json:"amount"`
	At     string `json:"at"`
	Fund   string `json:"fund"`
	Status string `json:"status"`
}

type User struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Email   string `json:"email"`
	Phone   string `json:"phone"`
	Role    string `json:"role"`
	OrgSlug string `json:"orgSlug"`
	Region  string `json:"region"`
	Segment string `json:"segment"`
	Avatar  string `json:"avatar"`
}

// store holds everything in memory, behind one lock; a restart starts over.
type store struct {
	mu            sync.Mutex
	categories    []Category
	orgs          []Org
	registrations []Registration
	notices       []Notice
	programmes    []Programme
	sessions      []Session
	speakers      []Speaker
	workshops     []Workshop
	products      []Product
	listings      []Listing
	donations     []Donation
	users         map[string]*User
	idempotency   map[string][]byte
	uploads       map[string]string
	partners      []map[string]any
	refresh       map[string]string
	otps          map[string]string
	seq           int
}

func (s *store) next(prefix string) string {
	s.seq++
	return fmt.Sprintf("%s_%06d", prefix, s.seq)
}

func iso(t time.Time) string { return t.Format(time.RFC3339) }

func ptr[T any](v T) *T { return &v }

func seed() *store {
	now := time.Now().UTC().Truncate(time.Minute)
	s := &store{
		users:       map[string]*User{},
		idempotency: map[string][]byte{},
		uploads:     map[string]string{},
		refresh:     map[string]string{},
		otps:        map[string]string{},
	}
	s.categories = []Category{
		{"finance", "Finance", "account_balance"},
		{"education", "Education", "school"},
		{"sport", "Sport", "directions_run"},
		{"tech", "Tech & events", "devices"},
		{"health", "Health", "local_hospital"},
		{"charity", "Charity", "volunteer_activism"},
		{"housing", "Housing", "home_work"},
		{"culture", "Culture & arenas", "stadium"},
	}
	type o struct {
		slug, name, cat, city, summary, accent string
		lat, lng                               float64
		days                                   int
		seats                                  int
		fee                                    float64
		tags                                   []string
	}
	main := []o{
		{"bank", "Nordvia Bank", "finance", "Oslo", "Open an account in ten minutes, with your ID and a selfie.", "#0B3D91", 59.9139, 10.7522, 12, 0, 0, []string{"account", "kyc", "sca"}},
		{"campus", "Aster University", "education", "Addis Ababa", "Apply to undergraduate and graduate programmes in four steps.", "#7C2D12", 9.0300, 38.7400, 30, 220, 35, []string{"admissions", "scholarship"}},
		{"cityrun", "Riverton City Run", "sport", "Riverton", "5 km to marathon along the river; live tracking on race day.", "#EA580C", 52.5200, 13.4050, 45, 1200, 39, []string{"race", "live"}},
		{"devconf", "DevHarbor Conf", "tech", "Rotterdam", "Two days of talks and workshops on mobile and server-driven UI.", "#22D3EE", 51.9244, 4.4777, 21, 380, 249, []string{"conference", "workshops"}},
		{"clinic", "Willow Clinic", "health", "Lisbon", "Book a visit, check in from home and follow the queue live.", "#0F766E", 38.7223, -9.1393, 3, 14, 0, []string{"appointments", "queue"}},
		{"relief", "Hands Together", "charity", "Amman", "Monthly giving for families affected by the flood.", "#BE123C", 31.9454, 35.9284, 60, 0, 0, []string{"donation", "volunteer"}},
		{"homes", "Brickyard Homes", "housing", "Leeds", "Rent a flat with no agency fee; apply with one form.", "#78350F", 53.8008, -1.5491, 14, 9, 0, []string{"rental", "apply"}},
		{"arena", "Pulse Arena", "culture", "Milan", "Membership for concerts, sport nights and family days.", "#DB2777", 45.4642, 9.1900, 10, 60, 89, []string{"membership", "tickets"}},
	}
	for i, m := range main {
		org := Org{
			Slug: m.slug, Name: m.name, Category: m.cat, City: m.city, Lat: m.lat, Lng: m.lng,
			Logo: "asset:org-" + m.slug + "-logo.png", Cover: "asset:org-" + m.slug + "-cover.webp",
			ClosesAt: ptr(iso(now.Add(time.Duration(m.days) * 24 * time.Hour))), Verified: i%3 != 2,
			Tags: m.tags, Summary: m.summary, Accent: m.accent,
		}
		if m.seats > 0 {
			org.SeatsLeft = ptr(m.seats)
		}
		if m.fee > 0 {
			org.Fee = eur(m.fee)
		}
		s.orgs = append(s.orgs, org)
	}
	cities := []string{"Berlin", "Vienna", "Prague", "Madrid", "Dublin", "Nairobi", "Tunis", "Kraków", "Porto", "Gdańsk"}
	for i := 0; i < 30; i++ {
		cat := s.categories[i%len(s.categories)]
		s.orgs = append(s.orgs, Org{
			Slug: fmt.Sprintf("soon-%02d", i+1), Name: fmt.Sprintf("%s Collective %d", cat.Label, i+1), Category: cat.Key,
			City: cities[i%len(cities)], Lat: 48 + float64(i%7), Lng: 2 + float64(i%11),
			Logo: "asset:avatar-placeholder.png", Cover: "asset:pattern.png", Verified: false,
			Tags: []string{"coming-soon"}, Summary: "Opening for registration soon.", Accent: "#64748B", ComingSoon: true,
		})
	}
	demo := &User{ID: "usr_demo", Name: "Selam Bekele", Email: "selam@example.org", Phone: "+15555550100", Role: "member", Region: "eu", Segment: "beta", Avatar: "asset:avatar-placeholder.png"}
	staff := &User{ID: "usr_staff", Name: "Jonas Lind", Email: "jonas@nordvia.example", Phone: "+15555550101", Role: "staff", OrgSlug: "bank", Region: "eu", Segment: "general"}
	keeper := &User{ID: "usr_maint", Name: "Ama Owusu", Email: "ama@example.org", Phone: "+15555550102", Role: "maintainer", Region: "eu", Segment: "beta"}
	for _, u := range []*User{demo, staff, keeper} {
		s.users[u.ID] = u
	}
	statuses := []string{"submitted", "approved", "draft", "done", "withdrawn", "approved"}
	for i, slug := range []string{"cityrun", "devconf", "campus", "clinic", "homes", "arena"} {
		org := s.org(slug)
		r := Registration{
			ID: s.next("reg"), OrgSlug: slug, OrgName: org.Name, FormKey: slug + "-main", Status: statuses[i],
			SubmittedAt: iso(now.Add(-time.Duration(i+1) * 26 * time.Hour)), PassCode: fmt.Sprintf("AR-%04d-%s", 1200+i*37, strings.ToUpper(slug[:3])),
			Owner: demo.ID, Pinned: i == 0, Name: demo.Name,
		}
		if org.Fee != nil {
			r.Amount = org.Fee
		}
		if org.ClosesAt != nil {
			r.EventAt = org.ClosesAt
		}
		s.registrations = append(s.registrations, r)
	}
	for i := 0; i < 64; i++ {
		s.registrations = append(s.registrations, Registration{
			ID: s.next("reg"), OrgSlug: "bank", OrgName: "Nordvia Bank", FormKey: "bank-account",
			Status: []string{"submitted", "approved", "submitted", "rejected"}[i%4], SubmittedAt: iso(now.Add(-time.Duration(i) * 3 * time.Hour)),
			PassCode: fmt.Sprintf("NB-%05d", 30000+i*13), Owner: fmt.Sprintf("usr_%03d", i), Name: fmt.Sprintf("Applicant %d", i+1),
		})
	}
	s.notices = []Notice{
		{ID: s.next("ntc"), Title: "Your pass is ready", Body: "Riverton City Run: show it at the start line.", Route: "pass", Params: map[string]string{"id": s.registrations[0].ID}, At: iso(now.Add(-2 * time.Hour))},
		{ID: s.next("ntc"), Title: "DevHarbor workshops open", Body: "Pick two workshops before Friday.", Route: "ex-org", Params: map[string]string{"slug": "devconf"}, At: iso(now.Add(-26 * time.Hour)), Read: true},
	}
	levels := []string{"Bachelor", "Master", "Diploma", "Certificate"}
	faculties := []string{"Engineering", "Health Sciences", "Business", "Arts", "Natural Sciences"}
	subjects := []string{"Software Engineering", "Public Health", "Economics", "Architecture", "Data Science", "Nursing", "Civil Engineering", "Fine Arts", "Agronomy", "Law"}
	for i := 0; i < 40; i++ {
		lvl := levels[i%len(levels)]
		s.programmes = append(s.programmes, Programme{
			ID: fmt.Sprintf("prg-%02d", i+1), Name: fmt.Sprintf("%s %s", lvl, subjects[i%len(subjects)]), Level: strings.ToLower(lvl),
			Fee: Money{Amount: fmt.Sprintf("%d.00", 900+i*45), Currency: "EUR"}, Faculty: faculties[i%len(faculties)],
			Duration: []string{"3 years", "2 years", "1 year", "6 months"}[i%4], Seats: 20 + (i*7)%60,
			Summary: "Taught in English and Amharic; applications close at the end of the month.",
		})
	}
	speakers := [][4]string{{"Ines Duarte", "Staff Engineer", "Quanta Mobile", "Server-driven UI at scale"}, {"Kwame Mensah", "Principal Designer", "Fieldnote", "Design tokens that survive"}, {"Mira Halvorsen", "CTO", "Nordvia", "Signing every pixel"}, {"Tomás Rivera", "Developer Advocate", "Flutterhouse", "Rendering without rebuilds"}, {"Aiko Tanaka", "Security Lead", "Kitsune Pay", "Hardware keys in the wild"}, {"Leila Haddad", "Product Lead", "Sahel Health", "Offline first, really"}}
	for i, sp := range speakers {
		s.speakers = append(s.speakers, Speaker{ID: fmt.Sprintf("spk-%d", i+1), Name: sp[0], Role: sp[1], Company: sp[2], Avatar: "asset:avatar-placeholder.png", Bio: sp[3]})
	}
	rooms := []string{"Harbour Hall", "Dock 2", "Lighthouse"}
	tracks := []string{"Platform", "Design", "Security"}
	for d := 0; d < 2; d++ {
		for i := 0; i < 6; i++ {
			start := now.Add(time.Duration(21*24+d*24) * time.Hour).Truncate(24 * time.Hour).Add(time.Duration(9+i) * time.Hour)
			sp := s.speakers[(i+d)%len(s.speakers)]
			s.sessions = append(s.sessions, Session{ID: fmt.Sprintf("ses-%d-%d", d, i), Day: d, Title: speakers[(i+d)%len(speakers)][3], Speaker: sp.Name, Room: rooms[i%3], Starts: iso(start), Ends: iso(start.Add(50 * time.Minute)), Track: tracks[i%3]})
		}
	}
	for i, w := range []string{"Build a plugin in 60 minutes", "Accessible forms", "PXL deep dive", "Offline sync patterns", "Theming with tokens"} {
		s.workshops = append(s.workshops, Workshop{ID: fmt.Sprintf("wks-%d", i+1), Title: w, Capacity: 24, Taken: []int{24, 18, 9, 22, 3}[i], Starts: iso(now.Add(time.Duration(21*24+13+i) * time.Hour)), Level: []string{"beginner", "intermediate", "advanced"}[i%3]})
	}
	for i, p := range [][3]string{{"tkt-std", "Standard ticket", "ticket"}, {"tkt-vip", "Supporter ticket", "ticket"}, {"tkt-stu", "Student ticket", "ticket"}, {"mrc-tee", "Harbour tee", "merch"}, {"mrc-hood", "Lighthouse hoodie", "merch"}, {"mrc-cap", "Dock cap", "merch"}, {"mrc-mug", "Night shift mug", "merch"}, {"mrc-stk", "Sticker pack", "merch"}, {"mrc-bag", "Tote bag", "merch"}} {
		price := []float64{249, 499, 99, 29, 59, 22, 15, 6, 18}[i]
		s.products = append(s.products, Product{SKU: p[0], Title: p[1], Kind: p[2], Price: Money{Amount: fmt.Sprintf("%.2f", price), Currency: "EUR"}, Image: "asset:merch-" + fmt.Sprint(i%6+1) + ".webp", Badge: []string{"", "Popular", "", "New", "", "", "", "", "Eco"}[i], InStock: i != 6})
	}
	areas := []string{"Holbeck", "Headingley", "Chapel Allerton", "Kirkstall", "Roundhay", "Hunslet", "Meanwood", "Armley", "Burley"}
	for i, a := range areas {
		s.listings = append(s.listings, Listing{
			ID: fmt.Sprintf("lst-%d", i+1), Title: fmt.Sprintf("%d-bed flat, %s", 1+i%3, a), Area: a,
			Rent: Money{Amount: fmt.Sprintf("%d.00", 720+i*65), Currency: "GBP"}, Rooms: 1 + i%3, SizeM2: 42 + i*7,
			Image: fmt.Sprintf("asset:listing-%d.webp", i+1), Lat: 53.80 + float64(i)*0.01, Lng: -1.55 + float64(i%3)*0.02,
			Features: [][]string{{"light", "quiet"}, {"transport", "balcony"}, {"quiet", "garden"}}[i%3], Available: iso(now.Add(time.Duration(7+i*5) * 24 * time.Hour)),
		})
	}
	for i := 0; i < 5; i++ {
		s.donations = append(s.donations, Donation{ID: s.next("don"), Amount: Money{Amount: fmt.Sprintf("%d.00", []int{25, 25, 50, 25, 100}[i]), Currency: "EUR"}, At: iso(now.Add(-time.Duration(i*30) * 24 * time.Hour)), Fund: []string{"monthly", "monthly", "school-kits", "monthly", "water"}[i], Status: "paid"})
	}
	return s
}

func (s *store) org(slug string) *Org {
	for i := range s.orgs {
		if s.orgs[i].Slug == slug {
			return &s.orgs[i]
		}
	}
	return nil
}

// distanceKm is the great-circle distance between two points.
func distanceKm(lat1, lng1, lat2, lng2 float64) float64 {
	const r = 6371.0
	p1, p2 := lat1*math.Pi/180, lat2*math.Pi/180
	dp, dl := (lat2-lat1)*math.Pi/180, (lng2-lng1)*math.Pi/180
	a := math.Sin(dp/2)*math.Sin(dp/2) + math.Cos(p1)*math.Cos(p2)*math.Sin(dl/2)*math.Sin(dl/2)
	return 2 * r * math.Asin(math.Sqrt(a))
}

func sortOrgs(list []Org, by string, lat, lng float64, near bool) {
	sort.SliceStable(list, func(i, j int) bool {
		a, b := list[i], list[j]
		switch by {
		case "name":
			return a.Name < b.Name
		case "closing":
			if a.ClosesAt == nil || b.ClosesAt == nil {
				return a.ClosesAt != nil
			}
			return *a.ClosesAt < *b.ClosesAt
		case "distance":
			if near {
				return distanceKm(lat, lng, a.Lat, a.Lng) < distanceKm(lat, lng, b.Lat, b.Lng)
			}
		}
		return !a.ComingSoon && b.ComingSoon
	})
}
