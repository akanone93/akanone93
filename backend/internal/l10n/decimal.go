// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package l10n

import (
	"fmt"
	"strings"
)

// Decimal is an exact base-10 number: sign, integer digits and the fraction
// digits as written, trailing zeros included ("1.50" keeps its two digits,
// which plural rules and formatting see). The zero value is 0. Money and
// every other quantity the engine formats use Decimal, never float64.
type Decimal struct {
	neg  bool
	ints string // integer digits without leading zeros; "" is 0
	frac string // fraction digits as written
}

// maxExponent bounds the exponent ParseDecimal accepts so a hostile string
// cannot ask for gigabytes of zeros.
const maxExponent = 400

// ParseDecimal parses an optionally signed decimal such as "-1234.50" or
// "1.5e3".
func ParseDecimal(s string) (Decimal, error) {
	bad := func() (Decimal, error) { return Decimal{}, fmt.Errorf("%w: %q", ErrInvalidDecimal, s) }
	var d Decimal
	rest := s
	if rest != "" && (rest[0] == '-' || rest[0] == '+') {
		d.neg = rest[0] == '-'
		rest = rest[1:]
	}
	exp := 0
	if i := strings.IndexAny(rest, "eE"); i >= 0 {
		var ok bool
		exp, ok = parseExponent(rest[i+1:])
		if !ok {
			return bad()
		}
		rest = rest[:i]
	}
	ip, fp, _ := strings.Cut(rest, ".")
	if (ip == "" && fp == "") || !allDigits(ip) || !allDigits(fp) || strings.Contains(fp, ".") {
		return bad()
	}
	d.ints = strings.TrimLeft(ip, "0")
	d.frac = fp
	if exp != 0 {
		d = d.shift(exp)
		d.neg = s[0] == '-'
	}
	return d, nil
}

// parseExponent parses the optionally signed exponent digits after the "e"
// and reports false when they are malformed or exceed maxExponent.
func parseExponent(e string) (int, bool) {
	sign := 1
	if e != "" && (e[0] == '-' || e[0] == '+') {
		if e[0] == '-' {
			sign = -1
		}
		e = e[1:]
	}
	if e == "" || len(e) > 3 || !allDigits(e) {
		return 0, false
	}
	exp := 0
	for _, c := range e {
		exp = exp*10 + int(c-'0')
	}
	if exp > maxExponent {
		return 0, false
	}
	return exp * sign, true
}

func allDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// DecimalFromInt returns n as a Decimal.
func DecimalFromInt(n int64) Decimal {
	if n == 0 {
		return Decimal{}
	}
	neg := n < 0
	u := uint64(n) //nolint:gosec // G115: two's complement; negating gives |n|, also for MinInt64.
	if neg {
		u = -u
	}
	return Decimal{neg: neg, ints: fmt.Sprint(u)}
}

// String returns the plain notation ("-1234.50"), keeping the fraction
// digits as written.
func (d Decimal) String() string {
	var b strings.Builder
	if d.neg {
		b.WriteByte('-')
	}
	b.WriteString(d.intDigits())
	if d.frac != "" {
		b.WriteByte('.')
		b.WriteString(d.frac)
	}
	return b.String()
}

// IsNegative reports whether the sign is negative; "-0" is negative.
func (d Decimal) IsNegative() bool { return d.neg }

// IsZero reports whether every digit is zero.
func (d Decimal) IsZero() bool { return d.ints == "" && strings.Trim(d.frac, "0") == "" }

// intDigits returns the integer digits, "0" for none.
func (d Decimal) intDigits() string {
	if d.ints == "" {
		return "0"
	}
	return d.ints
}

func (d Decimal) abs() Decimal {
	d.neg = false
	return d
}

// shift multiplies d by 10^n by moving the decimal point; the fraction
// digits are kept as far as they still exist.
func (d Decimal) shift(n int) Decimal {
	if n == 0 {
		return d
	}
	all := d.ints + d.frac
	point := len(d.ints) + n
	if point < 0 {
		all = strings.Repeat("0", -point) + all
		point = 0
	}
	if point > len(all) {
		all += strings.Repeat("0", point-len(all))
	}
	return Decimal{neg: d.neg, ints: strings.TrimLeft(all[:point], "0"), frac: all[point:]}
}

// roundFrac rounds d to n fraction digits, half to even, and returns it with
// exactly n digits; n may be negative to round to tens, hundreds and so on.
func (d Decimal) roundFrac(n int) Decimal {
	if n < 0 {
		r := d.shift(n).roundFrac(0)
		return r.shift(-n)
	}
	if len(d.frac) <= n {
		d.frac += strings.Repeat("0", n-len(d.frac))
		return d
	}
	keep, rest := d.frac[:n], d.frac[n:]
	digits := []byte(d.ints + keep)
	up := false
	switch {
	case rest[0] > '5':
		up = true
	case rest[0] == '5':
		if strings.Trim(rest[1:], "0") != "" {
			up = true
		} else {
			last := byte('0')
			if len(digits) > 0 {
				last = digits[len(digits)-1]
			}
			up = (last-'0')%2 == 1
		}
	}
	if up {
		i := len(digits) - 1
		for ; i >= 0; i-- {
			if digits[i] == '9' {
				digits[i] = '0'
				continue
			}
			digits[i]++
			break
		}
		if i < 0 {
			digits = append([]byte{'1'}, digits...)
		}
	}
	ip := string(digits[:len(digits)-n])
	return Decimal{neg: d.neg, ints: strings.TrimLeft(ip, "0"), frac: string(digits[len(digits)-n:])}
}

// roundSignificant rounds d to sig significant digits, half to even.
func (d Decimal) roundSignificant(sig int) Decimal {
	if d.IsZero() {
		return d
	}
	return d.roundFrac(sig - 1 - d.exponent())
}

// exponent returns the power of ten of the first non-zero digit.
func (d Decimal) exponent() int {
	if d.ints != "" {
		return len(d.ints) - 1
	}
	for i := 0; i < len(d.frac); i++ {
		if d.frac[i] != '0' {
			return -(i + 1)
		}
	}
	return 0
}

// trimFrac removes trailing zero fraction digits down to min digits.
func (d Decimal) trimFrac(min int) Decimal {
	for len(d.frac) > min && d.frac[len(d.frac)-1] == '0' {
		d.frac = d.frac[:len(d.frac)-1]
	}
	return d
}

// intLen returns the number of integer digits, 0 when |d| < 1.
func (d Decimal) intLen() int { return len(d.ints) }

// satMax is what operands saturate to; no plural rule compares with a value
// this large.
const satMax = int64(1) << 62

// digitsValue returns the digit string as a number, modulo mod when mod > 0,
// and saturated at satMax otherwise.
func digitsValue(s string, mod int64) int64 {
	var v int64
	for i := 0; i < len(s); i++ {
		c := int64(s[i] - '0')
		if mod > 0 {
			v = (v*10 + c) % mod
			continue
		}
		if v > (satMax-c)/10 {
			return satMax
		}
		v = v*10 + c
	}
	return v
}
