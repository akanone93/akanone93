// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package l10n

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// CalendarDate is a date and time of day in some calendar, already converted
// by the caller: the engine formats the fields it is given and does no
// calendar arithmetic. Era indexes the calendar's era names (Gregorian: 0 is
// BC, 1 is AD), Year is the year within the era, Month starts at 1 (the
// Ethiopic calendar has 13), Weekday is 0 for Sunday to 6 for Saturday, and
// Hour is 0 to 23.
type CalendarDate struct {
	Era, Year, Month, Day, Weekday, Hour, Minute, Second int
}

// GregorianDate returns the CalendarDate of a proleptic Gregorian civil date
// and time of day, with its weekday. Years before 1 are BC (year 0 is 1 BC).
func GregorianDate(year, month, day, hour, minute, second int) CalendarDate {
	d := CalendarDate{Era: 1, Year: year, Month: month, Day: day, Hour: hour, Minute: minute, Second: second}
	if year < 1 {
		d.Era, d.Year = 0, 1-year
	}
	// Days from the civil epoch (Howard Hinnant's algorithm); 1970-01-01 was
	// a Thursday.
	y := year
	if month <= 2 {
		y--
	}
	era := y / 400
	if y < 0 {
		era = (y - 399) / 400
	}
	yoe := y - era*400
	mp := (month + 9) % 12
	doy := (153*mp+2)/5 + day - 1
	doe := yoe*365 + yoe/4 - yoe/100 + doy
	days := era*146097 + doe - 719468
	d.Weekday = ((days+4)%7 + 7) % 7
	return d
}

// DateOptions select the format of FormatDate: DateStyle and TimeStyle
// ("full", "long", "medium", "short"), or else a CLDR Skeleton such as
// "yMMMd" or "yMMMEdhm".
type DateOptions struct {
	DateStyle string
	TimeStyle string
	Skeleton  string
	// NumberingSystem is "" or "latn" for Latin digits, or "default" or the
	// locale's own system id for native digits.
	NumberingSystem string
	// ZoneName is what time zone pattern letters (z, v, V, O, Z, x, X) write;
	// the engine knows no zones.
	ZoneName string
}

// rawCalendar is the JSON shape of one calendar.
type rawCalendar struct {
	Months           map[string]map[string][]string          `json:"months"`
	Days             map[string]map[string][]string          `json:"days"`
	Eras             map[string][]string                     `json:"eras"`
	DayPeriods       map[string]map[string]map[string]string `json:"dayPeriods"`
	DateFormats      map[string]string                       `json:"dateFormats"`
	TimeFormats      map[string]string                       `json:"timeFormats"`
	DateTimeFormats  map[string]string                       `json:"dateTimeFormats"`
	DateTimeAtTime   map[string]string                       `json:"dateTimeAtTime"`
	AvailableFormats map[string]string                       `json:"availableFormats"`
}

// calendar is one calendar's names and formats.
type calendar struct {
	raw   rawCalendar
	avail []availableFormat // sorted by skeleton
}

type availableFormat struct {
	key     string
	fields  skeleton
	pattern string
}

func compileCalendar(r rawCalendar) (*calendar, error) {
	c := &calendar{raw: r}
	for _, k := range []string{"full", "long", "medium", "short"} {
		if r.DateFormats[k] == "" || r.TimeFormats[k] == "" || r.DateTimeFormats[k] == "" {
			return nil, fmt.Errorf("format %s is missing", k)
		}
	}
	if len(r.Eras["abbreviated"]) == 0 || len(r.Months["format"]["wide"]) == 0 || len(r.Days["format"]["wide"]) != 7 {
		return nil, fmt.Errorf("names are missing")
	}
	for key, p := range r.AvailableFormats {
		sk, err := parseSkeleton(key)
		if err != nil {
			return nil, err
		}
		c.avail = append(c.avail, availableFormat{key: key, fields: sk, pattern: p})
	}
	sort.Slice(c.avail, func(i, j int) bool { return c.avail[i].key < c.avail[j].key })
	return c, nil
}

// bestFormat returns the index in avail of the format closest to want, the
// first of equals in key order; false when none has the requested fields.
func bestFormat(avail []availableFormat, want skeleton) (int, bool) {
	best := -1
	bestDist := 0
	for i, a := range avail {
		dist, ok := distance(want, a.fields)
		if !ok {
			continue
		}
		if best < 0 || dist < bestDist {
			best, bestDist = i, dist
		}
	}
	return best, best >= 0
}

// FormatDate formats d in a calendar ("gregorian", "ethiopic",
// "islamic-umalqura" or "persian") from the locale's date and time styles or
// skeleton (I18N-001). Pattern letters G y M L d E c a h H K k m s are
// implemented; Y reads as y, B as a, and the zone letters write ZoneName.
func (l *Locale) FormatDate(d CalendarDate, calendarID string, opts DateOptions) (string, error) {
	cal := l.calendars[calendarID]
	if cal == nil {
		return "", fmt.Errorf("%w: calendar %q for locale %s", ErrUnsupported, calendarID, l.id)
	}
	sysID, _, err := l.system(opts.NumberingSystem)
	if err != nil {
		return "", err
	}
	pat, err := cal.pattern(opts)
	if err != nil {
		return "", err
	}
	toks, err := tokenize(pat)
	if err != nil {
		return "", err
	}
	f := dateFormatter{l: l, cal: cal, sys: sysID, zone: opts.ZoneName, d: d}
	return f.format(toks)
}

// pattern returns the date-time pattern the options select.
func (c *calendar) pattern(opts DateOptions) (string, error) {
	if opts.DateStyle != "" || opts.TimeStyle != "" {
		return c.stylePattern(opts)
	}
	if opts.Skeleton == "" {
		return "", fmt.Errorf("%w: neither a style nor a skeleton", ErrInvalidOption)
	}
	want, err := parseSkeleton(opts.Skeleton)
	if err != nil {
		return "", err
	}
	dateSk, timeSk := want.split()
	switch {
	case len(timeSk) == 0:
		return c.match(dateSk)
	case len(dateSk) == 0:
		return c.match(timeSk)
	}
	date, err := c.match(dateSk)
	if err != nil {
		return "", err
	}
	tm, err := c.match(timeSk)
	if err != nil {
		return "", err
	}
	style := "short"
	if m := dateSk.count('M'); m >= 4 {
		style = "long"
		if dateSk.has('E') {
			style = "full"
		}
	} else if m == 3 {
		style = "medium"
	}
	return fillGlue(c.glue(style), date, tm), nil
}

// stylePattern returns the pattern for the date and time styles in opts.
func (c *calendar) stylePattern(opts DateOptions) (string, error) {
	if opts.Skeleton != "" {
		return "", fmt.Errorf("%w: both styles and a skeleton", ErrInvalidOption)
	}
	var date, tm string
	if opts.DateStyle != "" {
		if date = c.raw.DateFormats[opts.DateStyle]; date == "" {
			return "", fmt.Errorf("%w: date style %q", ErrUnsupported, opts.DateStyle)
		}
	}
	if opts.TimeStyle != "" {
		if tm = c.raw.TimeFormats[opts.TimeStyle]; tm == "" {
			return "", fmt.Errorf("%w: time style %q", ErrUnsupported, opts.TimeStyle)
		}
	}
	if date == "" || tm == "" {
		return date + tm, nil
	}
	return fillGlue(c.glue(opts.DateStyle), date, tm), nil
}

// glue returns the pattern joining a date and a time of the given style.
func (c *calendar) glue(style string) string {
	if g := c.raw.DateTimeAtTime[style]; g != "" {
		return g
	}
	return c.raw.DateTimeFormats[style]
}

func fillGlue(glue, date, tm string) string {
	return strings.NewReplacer("{0}", tm, "{1}", date).Replace(glue)
}

// skeleton is a parsed CLDR skeleton: the normalised letter and count of each
// field. L reads as M, c as E, Y as y, and K as h, k as H (the 12 and 24 hour
// classes); a (the day period) is implied by the hour and not kept.
type skeleton map[byte]skelField

type skelField struct {
	letter byte
	count  int
}

func parseSkeleton(s string) (skeleton, error) {
	out := skeleton{}
	for i := 0; i < len(s); {
		j := i
		for j < len(s) && s[j] == s[i] {
			j++
		}
		letter, n := s[i], j-i
		i = j
		var key byte
		switch letter {
		case 'G', 'y', 'M', 'E', 'd', 'm', 's':
			key = letter
		case 'L':
			key, letter = 'M', 'M'
		case 'c':
			key, letter = 'E', 'E'
		case 'Y':
			key, letter = 'y', 'y'
		case 'h', 'K', 'H', 'k':
			key = 'h'
		case 'a':
			continue
		default:
			return nil, fmt.Errorf("%w: skeleton field %q in %q", ErrUnsupported, letter, s)
		}
		if _, dup := out[key]; dup {
			return nil, fmt.Errorf("%w: skeleton %q repeats a field", ErrInvalidOption, s)
		}
		out[key] = skelField{letter: letter, count: n}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%w: empty skeleton", ErrInvalidOption)
	}
	return out, nil
}

func (s skeleton) has(k byte) bool { _, ok := s[k]; return ok }

func (s skeleton) count(k byte) int { return s[k].count }

// split separates the date fields from the time fields.
func (s skeleton) split() (date, tm skeleton) {
	date, tm = skeleton{}, skeleton{}
	for k, f := range s {
		switch k {
		case 'h', 'm', 's':
			tm[k] = f
		default:
			date[k] = f
		}
	}
	return date, tm
}

// match finds the pattern for a skeleton of date fields or of time fields:
// the available format whose fields are the same, preferring the closest
// widths, with its field widths adjusted to the request the way CLDR's
// DateTimePatternGenerator does.
func (c *calendar) match(want skeleton) (string, error) {
	best, ok := bestFormat(c.avail, want)
	if !ok {
		return "", fmt.Errorf("%w: no format for the requested fields", ErrUnsupported)
	}
	a := c.avail[best]
	toks, err := tokenize(a.pattern)
	if err != nil {
		return "", err
	}
	for i, t := range toks {
		if t.letter == 0 {
			continue
		}
		key, ok := fieldOf(t.letter)
		if !ok {
			continue
		}
		req, have := want[key]
		if !have {
			continue
		}
		toks[i].n = adjustedWidth(key, t, req, a.fields[key])
		if key == 'h' {
			toks[i].letter = req.letter // the requested hour cycle letter: h for K, as ICU does
		}
	}
	return untokenize(toks), nil
}

// fieldOf maps a pattern letter to its skeleton field.
func fieldOf(letter byte) (byte, bool) {
	switch letter {
	case 'G', 'y', 'M', 'E', 'd', 'm', 's':
		return letter, true
	case 'L':
		return 'M', true
	case 'c':
		return 'E', true
	case 'Y':
		return 'y', true
	case 'h', 'H', 'K', 'k':
		return 'h', true
	}
	return 0, false
}

// distance compares a request with an available format's skeleton; it is not
// ok when their fields differ.
func distance(want, have skeleton) (int, bool) {
	if len(want) != len(have) {
		return 0, false
	}
	dist := 0
	for k, w := range want {
		h, ok := have[k]
		if !ok {
			return 0, false
		}
		if k == 'h' && is24Hour(w.letter) != is24Hour(h.letter) {
			return 0, false
		}
		d := w.count - h.count
		if d < 0 {
			d = -d
		}
		if k == 'M' && (w.count <= 2) != (h.count <= 2) {
			d += 100
		}
		dist += d
	}
	return dist, true
}

// adjustedWidth is the field width the matched pattern gets (ICU's
// adjustFieldTypes with the hour-length matching ECMA-402 asks for): minutes
// and seconds keep the pattern's width; so does any field whose skeleton
// already had the requested width, or whose pattern and skeleton disagree on
// numeric against text. Otherwise the requested width wins.
func adjustedWidth(key byte, t token, req, have skelField) int {
	if key == 'm' || key == 's' {
		return t.n
	}
	if have.count == req.count {
		return t.n
	}
	if isNumeric(key, t.n) != isNumeric(key, have.count) {
		return t.n
	}
	return req.count
}

// is24Hour reports whether an hour pattern letter counts hours of a 24-hour
// clock (H, k) rather than a 12-hour one (h, K).
func is24Hour(letter byte) bool { return letter == 'H' || letter == 'k' }

func isNumeric(key byte, n int) bool {
	switch key {
	case 'y', 'd', 'h', 'm', 's':
		return true
	case 'M':
		return n <= 2
	}
	return false
}

// token is a pattern field (letter and repeat count) or literal text.
type token struct {
	letter byte
	n      int
	text   string
}

// tokenize splits a CLDR date pattern into fields and literals; quotes
// protect text and ” is an apostrophe.
func tokenize(p string) ([]token, error) {
	var out []token
	var text strings.Builder
	flush := func() {
		if text.Len() > 0 {
			out = append(out, token{text: text.String()})
			text.Reset()
		}
	}
	for i := 0; i < len(p); {
		c := p[i]
		switch {
		case c == '\'':
			if i+1 < len(p) && p[i+1] == '\'' {
				text.WriteByte('\'')
				i += 2
				continue
			}
			end, ok := readQuoted(p, i, &text)
			if !ok {
				return nil, fmt.Errorf("%w: unterminated quote in %q", ErrInvalidData, p)
			}
			i = end
		case (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z'):
			flush()
			j := i
			for j < len(p) && p[j] == c {
				j++
			}
			out = append(out, token{letter: c, n: j - i})
			i = j
		default:
			text.WriteByte(c)
			i++
		}
	}
	flush()
	return out, nil
}

// readQuoted appends the text of the quote that opens at p[i] to text and
// returns the index after its closing quote; false when it is unterminated.
func readQuoted(p string, i int, text *strings.Builder) (int, bool) {
	j := i + 1
	for j < len(p) {
		if p[j] == '\'' {
			if j+1 < len(p) && p[j+1] == '\'' {
				text.WriteByte('\'') // '' inside a quote is an apostrophe
				j += 2
				continue
			}
			break
		}
		text.WriteByte(p[j])
		j++
	}
	if j == len(p) {
		return 0, false
	}
	return j + 1, true
}

// untokenize writes tokens back as a pattern, quoting literal text that
// holds letters or apostrophes.
func untokenize(toks []token) string {
	var b strings.Builder
	for _, t := range toks {
		if t.letter != 0 {
			b.WriteString(strings.Repeat(string(t.letter), t.n))
			continue
		}
		text := strings.ReplaceAll(t.text, "'", "''")
		if strings.ContainsAny(t.text, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ") {
			text = "'" + text + "'"
		}
		b.WriteString(text)
	}
	return b.String()
}

// dateFormatter writes the tokens of one pattern for one date.
type dateFormatter struct {
	l    *Locale
	cal  *calendar
	sys  string
	zone string
	d    CalendarDate
}

func (f *dateFormatter) format(toks []token) (string, error) {
	d := f.d
	if d.Day < 1 || d.Day > 31 || d.Weekday < 0 || d.Weekday > 6 || d.Hour < 0 || d.Hour > 23 ||
		d.Minute < 0 || d.Minute > 59 || d.Second < 0 || d.Second > 59 || d.Year < 0 {
		return "", fmt.Errorf("%w: date %+v", ErrInvalidOption, d)
	}
	var b strings.Builder
	for _, t := range toks {
		if t.letter == 0 {
			b.WriteString(t.text)
			continue
		}
		s, err := f.field(t)
		if err != nil {
			return "", err
		}
		b.WriteString(s)
	}
	return b.String(), nil
}

func (f *dateFormatter) num(v, width int) string {
	s := strconv.Itoa(v)
	if len(s) < width {
		s = strings.Repeat("0", width-len(s)) + s
	}
	return f.l.mapDigits(s, f.sys)
}

// name returns names[i] of a width, falling back to wider widths when a
// calendar carries no narrow names.
func name(byWidth map[string][]string, width string, i int) (string, bool) {
	for _, w := range []string{width, "abbreviated", "wide"} {
		if n := byWidth[w]; i >= 0 && i < len(n) {
			return n[i], true
		}
	}
	return "", false
}

func (f *dateFormatter) field(t token) (string, error) {
	d, raw := f.d, f.cal.raw
	switch t.letter {
	case 'G':
		if s, ok := name(raw.Eras, widthOf(t.n), d.Era); ok {
			return s, nil
		}
		return f.missing(t)
	case 'y', 'Y':
		if t.n == 2 {
			return f.num(d.Year%100, 2), nil
		}
		return f.num(d.Year, t.n), nil
	case 'M', 'L':
		return f.month(t)
	case 'd':
		return f.num(d.Day, t.n), nil
	case 'E', 'c':
		return f.weekday(t)
	case 'a', 'B':
		return f.dayPeriod(t)
	case 'h':
		h := d.Hour % 12
		if h == 0 {
			h = 12
		}
		return f.num(h, t.n), nil
	case 'H':
		return f.num(d.Hour, t.n), nil
	case 'K':
		return f.num(d.Hour%12, t.n), nil
	case 'k':
		h := d.Hour
		if h == 0 {
			h = 24
		}
		return f.num(h, t.n), nil
	case 'm':
		return f.num(d.Minute, t.n), nil
	case 's':
		return f.num(d.Second, t.n), nil
	case 'z', 'v', 'V', 'O', 'Z', 'x', 'X':
		return f.zone, nil
	}
	return "", fmt.Errorf("%w: pattern letter %q", ErrUnsupported, t.letter)
}

// missing reports that the date has no value for the field t.
func (f *dateFormatter) missing(t token) (string, error) {
	return "", fmt.Errorf("%w: date %+v has no value for %s", ErrInvalidOption, f.d, strings.Repeat(string(t.letter), t.n))
}

// month writes the M or L field: a number, or a format or stand-alone name.
func (f *dateFormatter) month(t token) (string, error) {
	if t.n <= 2 {
		return f.num(f.d.Month, t.n), nil
	}
	ctx := "format"
	if t.letter == 'L' {
		ctx = "standAlone"
	}
	if s, ok := name(f.cal.raw.Months[ctx], widthOf(t.n), f.d.Month-1); ok {
		return s, nil
	}
	return f.missing(t)
}

// weekday writes the E or c field as a format or stand-alone name.
func (f *dateFormatter) weekday(t token) (string, error) {
	if t.letter == 'c' && t.n < 3 {
		return "", fmt.Errorf("%w: numeric local weekday", ErrUnsupported)
	}
	ctx := "format"
	if t.letter == 'c' {
		ctx = "standAlone"
	}
	w := widthOf(t.n)
	if t.n == 6 {
		w = "short"
	}
	if s, ok := name(f.cal.raw.Days[ctx], w, f.d.Weekday); ok {
		return s, nil
	}
	return f.missing(t)
}

// dayPeriod writes the a or B field, falling back to the abbreviated width.
func (f *dateFormatter) dayPeriod(t token) (string, error) {
	key := "am"
	if f.d.Hour >= 12 {
		key = "pm"
	}
	periods := f.cal.raw.DayPeriods["format"]
	if s := periods[widthOf(t.n)][key]; s != "" {
		return s, nil
	}
	if s := periods["abbreviated"][key]; s != "" {
		return s, nil
	}
	return f.missing(t)
}

// widthOf maps a count of pattern letters to a name width (E, M, G, a).
func widthOf(n int) string {
	switch {
	case n == 4:
		return "wide"
	case n >= 5:
		return "narrow"
	}
	return "abbreviated"
}
