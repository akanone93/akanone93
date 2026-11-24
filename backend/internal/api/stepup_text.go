// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

package api

import (
	"strconv"
	"strings"
)

// stepUpText is the step-up page's words in one of Studio's languages (en,
// de, ar), with the page's language tag and direction.
type stepUpText struct {
	Lang, Dir                                 string
	Title, Heading, Lead, Button              string
	Waiting, Confirmed, NotConfirmed, Refused string
	SignIn, Session, NoRequest                string
}

// stepUpTextOf returns the page's words in a language, and whether the page
// has it; Studio's catalogues hold the same languages (STU-012).
func stepUpTextOf(lang string) (stepUpText, bool) {
	switch lang {
	case "en":
		return stepUpText{
			Lang: "en", Dir: "ltr",
			Title:        "Security key confirmation",
			Heading:      "Confirm with your security key",
			Lead:         "The Plux command line asks you to confirm an action with your security key. Continue only if you just started it.",
			Button:       "Use my security key",
			Waiting:      "Waiting for your security key…",
			Confirmed:    "Confirmed. You can return to the terminal.",
			NotConfirmed: "Not confirmed:",
			Refused:      "The request was refused.",
			SignIn:       "Sign in to Plux Studio in this browser, then open this link again.",
			Session:      "Your Studio session is not valid. Sign in again, then open this link again.",
			NoRequest:    "There is no step-up request waiting for you here. Run the command again.",
		}, true
	case "de":
		return stepUpText{
			Lang: "de", Dir: "ltr",
			Title:        "Bestätigung mit Sicherheitsschlüssel",
			Heading:      "Mit Ihrem Sicherheitsschlüssel bestätigen",
			Lead:         "Die Plux-Befehlszeile bittet Sie, eine Aktion mit Ihrem Sicherheitsschlüssel zu bestätigen. Fahren Sie nur fort, wenn Sie sie gerade selbst gestartet haben.",
			Button:       "Meinen Sicherheitsschlüssel verwenden",
			Waiting:      "Warte auf Ihren Sicherheitsschlüssel …",
			Confirmed:    "Bestätigt. Sie können zum Terminal zurückkehren.",
			NotConfirmed: "Nicht bestätigt:",
			Refused:      "Die Anfrage wurde abgelehnt.",
			SignIn:       "Melden Sie sich in diesem Browser bei Plux Studio an und öffnen Sie diesen Link dann erneut.",
			Session:      "Ihre Studio-Sitzung ist nicht gültig. Melden Sie sich erneut an und öffnen Sie diesen Link dann erneut.",
			NoRequest:    "Hier wartet keine Bestätigungsanfrage auf Sie. Führen Sie den Befehl erneut aus.",
		}, true
	case "ar":
		return stepUpText{
			Lang: "ar", Dir: "rtl",
			Title:        "التأكيد بمفتاح الأمان",
			Heading:      "أكّد باستخدام مفتاح الأمان",
			Lead:         "يطلب منك سطر أوامر Plux تأكيد إجراء باستخدام مفتاح الأمان. تابع فقط إذا كنت قد بدأته للتو.",
			Button:       "استخدام مفتاح الأمان",
			Waiting:      "في انتظار مفتاح الأمان…",
			Confirmed:    "تم التأكيد. يمكنك العودة إلى الطرفية.",
			NotConfirmed: "لم يتم التأكيد:",
			Refused:      "رُفض الطلب.",
			SignIn:       "سجّل الدخول إلى Plux Studio في هذا المتصفح، ثم افتح هذا الرابط مرة أخرى.",
			Session:      "جلسة Studio غير صالحة. سجّل الدخول مرة أخرى، ثم افتح هذا الرابط مرة أخرى.",
			NoRequest:    "لا يوجد هنا طلب تأكيد بانتظارك. شغّل الأمر مرة أخرى.",
		}, true
	}
	return stepUpText{}, false
}

// stepUpTextFor picks the page's language from an Accept-Language header:
// the most preferred of en, de and ar, by quality and then by order; English
// when none is asked for.
func stepUpTextFor(header string) stepUpText {
	best, bestQ := "en", -1.0
	for i, part := range strings.Split(header, ",") {
		tag, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		lang := strings.ToLower(strings.SplitN(strings.TrimSpace(tag), "-", 2)[0])
		if _, ok := stepUpTextOf(lang); !ok {
			continue
		}
		q := 1.0
		if v, ok := strings.CutPrefix(strings.TrimSpace(params), "q="); ok {
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				q = f
			}
		}
		// Earlier entries win ties: a small penalty by position keeps the order.
		if score := q - float64(i)*1e-6; q > 0 && score > bestQ {
			best, bestQ = lang, score
		}
	}
	text, _ := stepUpTextOf(best)
	return text
}
