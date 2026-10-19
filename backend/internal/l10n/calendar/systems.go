// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package calendar

import "sort"

// gregorian is the proleptic Gregorian calendar.
type gregorian struct{}

func (gregorian) ID() string               { return Gregorian }
func (gregorian) MonthsInYear(int) int     { return 12 }
func (gregorian) OutOfTable(int) bool      { return false }
func (gregorian) IsLeapYear(year int) bool { return year%4 == 0 && (year%100 != 0 || year%400 == 0) }

func (g gregorian) DaysInMonth(year, month int) int {
	switch month {
	case 2:
		if g.IsLeapYear(year) {
			return 29
		}
		return 28
	case 4, 6, 9, 11:
		return 30
	case 1, 3, 5, 7, 8, 10, 12:
		return 31
	}
	return 0
}

// EraOf: era 0 is BC (year 0 is 1 BC), era 1 is AD.
func (gregorian) EraOf(year int) (int, int) {
	if year < 1 {
		return 0, 1 - year
	}
	return 1, year
}

func (gregorian) YearOf(era, eraYear int) (int, bool) {
	switch era {
	case 0:
		return 1 - eraYear, true
	case 1:
		return eraYear, true
	}
	return 0, false
}

func (g gregorian) ToJDN(d Date) (int, error) {
	if err := checkDate(g, d); err != nil {
		return 0, err
	}
	a := (14 - d.Month) / 12
	y := d.Year + 4800 - a
	m := d.Month + 12*a - 3
	return d.Day + (153*m+2)/5 + 365*y + floorDiv(y, 4) - floorDiv(y, 100) + floorDiv(y, 400) - 32045, nil
}

func (gregorian) FromJDN(jdn int) Date {
	a := jdn + 32044
	b := floorDiv(4*a+3, 146097)
	c := a - floorDiv(146097*b, 4)
	d := floorDiv(4*c+3, 1461)
	e := c - floorDiv(1461*d, 4)
	m := (5*e + 2) / 153
	return Date{
		Year:  100*b + d - 4800 + m/10,
		Month: m + 3 - 12*(m/10),
		Day:   e - (153*m+2)/5 + 1,
	}
}

// ethiopic is the Ethiopian calendar: Amete Mihret from year 1, Amete Alem
// (year + 5500) before it; twelve months of 30 days and a 13th of 5 or 6.
type ethiopic struct{}

const ethiopicEpochOffset = 1723856

func (ethiopic) ID() string               { return Ethiopic }
func (ethiopic) MonthsInYear(int) int     { return 13 }
func (ethiopic) OutOfTable(int) bool      { return false }
func (ethiopic) IsLeapYear(year int) bool { return floorMod(year, 4) == 3 }
func (e ethiopic) DaysInMonth(year, month int) int {
	switch {
	case month >= 1 && month <= 12:
		return 30
	case month == 13 && e.IsLeapYear(year):
		return 6
	case month == 13:
		return 5
	}
	return 0
}

// EraOf: era 0 is Amete Alem, era 1 is Amete Mihret.
func (ethiopic) EraOf(year int) (int, int) {
	if year <= 0 {
		return 0, year + 5500
	}
	return 1, year
}

func (ethiopic) YearOf(era, eraYear int) (int, bool) {
	switch era {
	case 0:
		return eraYear - 5500, true
	case 1:
		return eraYear, true
	}
	return 0, false
}

func (e ethiopic) ToJDN(d Date) (int, error) {
	if err := checkDate(e, d); err != nil {
		return 0, err
	}
	return ethiopicEpochOffset + 365*d.Year + floorDiv(d.Year, 4) + 30*(d.Month-1) + d.Day - 1, nil
}

func (ethiopic) FromJDN(jdn int) Date {
	days := jdn - ethiopicEpochOffset
	c4 := floorDiv(days, 1461)
	r := floorMod(days, 1461)
	year := 4*c4 + r/365 - r/1460
	doy := r % 365
	if r == 1460 {
		doy = 365
	}
	return Date{Year: year, Month: doy/30 + 1, Day: doy%30 + 1}
}

// civilYearStart is the first day of an Islamic civil year, in days after the
// civil epoch.
func civilYearStart(year int) int {
	return 354*(year-1) + floorDiv(3+11*year, 30)
}

func civilLeap(year int) bool { return floorMod(14+11*year, 30) < 11 }

func civilDaysInMonth(year, month int) int {
	n := 29 + month%2
	if month == 12 && civilLeap(year) {
		n++
	}
	return n
}

// civilFromDays converts days after the civil epoch (tabular Islamic civil).
func civilFromDays(days int) Date {
	year := floorDiv(30*days+10646, 10631)
	ys := civilYearStart(year)
	// Month m (1-based) starts (59*(m-1)+1)/2 days into the year.
	month := -floorDiv(-2*(days-29-ys), 59) + 1
	if month < 1 {
		month = 1
	}
	if month > 12 {
		month = 12
	}
	return Date{Year: year, Month: month, Day: days - ys - (59*(month-1)+1)/2 + 1}
}

// umalqura is the Umm al-Qura calendar: the pinned table for 1300-1600 AH,
// tabular Islamic civil arithmetic outside it. Year 1 AH is the first year of
// era 0 (AH); earlier years are era 1 (BH).
type umalqura struct{}

func (umalqura) ID() string           { return IslamicUmalqura }
func (umalqura) MonthsInYear(int) int { return 12 }

func (umalqura) OutOfTable(year int) bool {
	return year < umalquraYearStart || year > umalquraYearEnd
}

func (u umalqura) IsLeapYear(year int) bool {
	if u.OutOfTable(year) {
		return civilLeap(year)
	}
	return u.yearLength(year) == 355
}

func (umalqura) yearLength(year int) int {
	i := year - umalquraYearStart
	return int(umalquraYearStarts[i+1] - umalquraYearStarts[i])
}

func (u umalqura) DaysInMonth(year, month int) int {
	if month < 1 || month > 12 {
		return 0
	}
	if u.OutOfTable(year) {
		return civilDaysInMonth(year, month)
	}
	return 29 + int(umalquraMonthMasks[year-umalquraYearStart]>>(12-month))&1
}

func (umalqura) EraOf(year int) (int, int) {
	if year < 1 {
		return 1, 1 - year
	}
	return 0, year
}

func (umalqura) YearOf(era, eraYear int) (int, bool) {
	switch era {
	case 0:
		return eraYear, true
	case 1:
		return 1 - eraYear, true
	}
	return 0, false
}

func (u umalqura) ToJDN(d Date) (int, error) {
	if err := checkDate(u, d); err != nil {
		return 0, err
	}
	if u.OutOfTable(d.Year) {
		return civilEpochJDN + civilYearStart(d.Year) + (59*(d.Month-1)+1)/2 + d.Day - 1, nil
	}
	days := int(umalquraYearStarts[d.Year-umalquraYearStart])
	for m := 1; m < d.Month; m++ {
		days += u.DaysInMonth(d.Year, m)
	}
	return civilEpochJDN + days + d.Day - 1, nil
}

func (u umalqura) FromJDN(jdn int) Date {
	days := jdn - civilEpochJDN
	first, end := int(umalquraYearStarts[0]), int(umalquraYearStarts[len(umalquraYearStarts)-1])
	if days < first || days >= end {
		return civilFromDays(days)
	}
	i := sort.Search(len(umalquraYearStarts), func(i int) bool { return int(umalquraYearStarts[i]) > days }) - 1
	year := umalquraYearStart + i
	rest := days - int(umalquraYearStarts[i])
	month := 1
	for rest >= u.DaysInMonth(year, month) {
		rest -= u.DaysInMonth(year, month)
		month++
	}
	return Date{Year: year, Month: month, Day: rest + 1}
}

// persian is the Persian (Solar Hijri) calendar with ICU's 33-year
// arithmetic and its corrections for 1502 onward.
type persian struct{}

var persianDaysBefore = [12]int{0, 31, 62, 93, 124, 155, 186, 216, 246, 276, 306, 336}

func (persian) ID() string           { return Persian }
func (persian) MonthsInYear(int) int { return 12 }
func (persian) OutOfTable(int) bool  { return false }

func persianNonLeap(year int) bool {
	i := sort.Search(len(persianNonLeapYears), func(i int) bool { return int(persianNonLeapYears[i]) >= year })
	return i < len(persianNonLeapYears) && int(persianNonLeapYears[i]) == year
}

func (persian) IsLeapYear(year int) bool {
	if year >= int(persianNonLeapYears[0]) && persianNonLeap(year) {
		return false
	}
	if year > int(persianNonLeapYears[0]) && persianNonLeap(year-1) {
		return true
	}
	return floorMod(year*25+11, 33) < 8
}

func (p persian) DaysInMonth(year, month int) int {
	switch {
	case month >= 1 && month <= 6:
		return 31
	case month >= 7 && month <= 11:
		return 30
	case month == 12 && p.IsLeapYear(year):
		return 30
	case month == 12:
		return 29
	}
	return 0
}

func (persian) EraOf(year int) (int, int) { return 0, year }

func (persian) YearOf(era, eraYear int) (int, bool) { return eraYear, era == 0 }

// persianFirstDay is the first day of a year, in days after the epoch.
func persianFirstDay(year int) int {
	n := 365*(year-1) + floorDiv(8*year+21, 33)
	if year > int(persianNonLeapYears[0]) && persianNonLeap(year-1) {
		n--
	}
	return n
}

func (p persian) ToJDN(d Date) (int, error) {
	if err := checkDate(p, d); err != nil {
		return 0, err
	}
	return persianEpochJDN + persianFirstDay(d.Year) + persianDaysBefore[d.Month-1] + d.Day - 1, nil
}

func (persian) FromJDN(jdn int) Date {
	days := jdn - persianEpochJDN
	year := floorDiv(33*days+3, 12053) + 1
	doy := days - persianFirstDay(year)
	if doy == 365 && year >= int(persianNonLeapYears[0]) && persianNonLeap(year) {
		year++
		doy = 0
	}
	month := (doy-6)/30 + 1
	if doy < 216 {
		month = doy/31 + 1
	}
	return Date{Year: year, Month: month, Day: doy + 1 - persianDaysBefore[month-1]}
}
