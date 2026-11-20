package site

import (
	"encoding/json"
	"os"
	"slices"
	"testing"
)

// The website's cases (web/src/kit/chart-language.test.ts): a link card names the song as the
// page does.
func TestChartTextIsTheWebsites(t *testing.T) {
	data, err := os.ReadFile("../../contracts/chart-text.json")
	if err != nil {
		t.Fatal(err)
	}
	var contract struct {
		Charts map[string]localizedChart
		Cases  []struct {
			Name, Chart, Title, Subtitle string
			Language                     *string
			System                       []string
		}
	}
	if err = json.Unmarshal(data, &contract); err != nil || len(contract.Cases) == 0 {
		t.Fatal(err)
	}
	for _, c := range contract.Cases {
		language := ""
		if c.Language != nil {
			language = *c.Language
		}
		title, subtitle := chartText(contract.Charts[c.Chart], language, c.System)
		if title != c.Title || subtitle != c.Subtitle {
			t.Errorf("%s: %q, %q; want %q, %q", c.Name, title, subtitle, c.Title, c.Subtitle)
		}
	}
}

func TestAcceptLanguagesAreTheBrowsers(t *testing.T) {
	for header, want := range map[string][]string{
		"":                                    {},
		"ja":                                  {"ja"},
		"zh-TW,zh;q=0.9,en-US;q=0.8,en;q=0.7": {"zh-TW", "zh", "en-US", "en"},
		"en;q=0.5, ja , ko;q=0.5, *;q=0.1, fr;q=0": {"ja", "en", "ko"},
	} {
		if got := acceptLanguages(header); !slices.Equal(got, want) {
			t.Errorf("%q: %q, want %q", header, got, want)
		}
	}
}
