package site

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// localizedChart is what web/src/kit/chart-language.ts's chartText reads of a chart.
type localizedChart struct {
	Title    string `json:"title"`
	Subtitle string `json:"subtitle"`
	// As the TJA has them: "origin" is TITLE/SUBTITLE itself, the other keys locales.
	TitleTranslations    map[string]string `json:"titleTranslations"`
	SubtitleTranslations map[string]string `json:"subtitleTranslations"`
}

var traditionalChinese = regexp.MustCompile(`-(tw|hk|mo|hant)\b`)

// normalizeLanguage is web/src/kit/i18n.ts's: one of the site's languages, Chinese for any
// other.
func normalizeLanguage(value string) string {
	code := strings.ReplaceAll(strings.ToLower(jsTrim(value)), "_", "-")
	switch code {
	case "zh", "zh-cn", "zh-sg", "zh-hans":
		return "zh-Hans"
	}
	switch base, _, _ := strings.Cut(code, "-"); base {
	case "en", "ja", "ko":
		return base
	}
	return "zh-Hans"
}

// translationKeys is chart-language.ts's keysOf: the dictionary's keys for a language tag,
// best first; none for a language it has no key for.
func translationKeys(language string) []string {
	code := strings.ReplaceAll(strings.ToLower(jsTrim(language)), "_", "-")
	switch base, _, _ := strings.Cut(code, "-"); base {
	case "zh":
		if traditionalChinese.MatchString(code) {
			return []string{"zh_tw", "zh"}
		}
		return []string{"zh"}
	case "en", "ja", "ko":
		return []string{base}
	}
	return nil
}

// chartText is chart-language.ts's: the chart's names in language (empty is the site's own,
// Chinese), else in the first of the system's languages that has them, else the original.
// The two must agree: contracts/chart-text.json holds the cases both run.
func chartText(c localizedChart, language string, system []string) (title, subtitle string) {
	keys := translationKeys(normalizeLanguage(language))
	for _, l := range system {
		keys = append(keys, translationKeys(l)...)
	}
	pick := func(translations map[string]string, original string) string {
		for _, key := range keys {
			if v := translations[key]; jsTrim(v) != "" {
				return v
			}
		}
		for _, v := range []string{translations["origin"], original} {
			if jsTrim(v) != "" {
				return v
			}
		}
		return ""
	}
	subtitle = pick(c.SubtitleTranslations, c.Subtitle)
	if strings.HasPrefix(subtitle, "--") || strings.HasPrefix(subtitle, "++") {
		subtitle = subtitle[2:]
	}
	return pick(c.TitleTranslations, c.Title), subtitle
}

// acceptLanguages is the languages an Accept-Language header asks for, most wanted first, as
// a browser's navigator.languages lists them: without "*" and those it refuses (q=0).
func acceptLanguages(header string) []string {
	type wanted struct {
		tag string
		q   float64
	}
	var all []wanted
	for _, part := range strings.Split(header, ",") {
		tag, params, _ := strings.Cut(part, ";")
		tag = strings.TrimSpace(tag)
		q := 1.0
		for _, p := range strings.Split(params, ";") {
			if v, ok := strings.CutPrefix(strings.TrimSpace(p), "q="); ok {
				if n, err := strconv.ParseFloat(v, 64); err == nil {
					q = n
				}
			}
		}
		if tag != "" && tag != "*" && q > 0 {
			all = append(all, wanted{tag, q})
		}
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].q > all[j].q })
	tags := make([]string, len(all))
	for i, w := range all {
		tags[i] = w.tag
	}
	return tags
}
