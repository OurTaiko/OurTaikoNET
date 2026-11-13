package httpapi

import (
	"encoding/json"
	"testing"
)

// A chart's JSON has the fields of its own source and none of the other's, however the
// chart is held: alone, by pointer, or in a list.
func TestChartFieldsOfItsSource(t *testing.T) {
	for _, official := range []bool{false, true} {
		c := Chart{ID: "a", official: official, PreviewPath: "clip.ogg", AudioPreview: &AudioPreview{URL: "/audio"}, TJASize: 7, HasFiles: true}
		c.Title = "Song"
		for holder, value := range map[string]any{"value": c, "pointer": &c, "list": map[string]any{"items": []Chart{c}}} {
			data, e := json.Marshal(value)
			if e != nil {
				t.Fatal(e)
			}
			if holder == "list" {
				var list struct{ Items []json.RawMessage }
				if e = json.Unmarshal(data, &list); e != nil || len(list.Items) != 1 {
					t.Fatal(e, string(data))
				}
				data = list.Items[0]
			}
			var fields map[string]json.RawMessage
			if e = json.Unmarshal(data, &fields); e != nil {
				t.Fatal(e)
			}
			for name, want := range map[string]bool{
				"officialId": official, "hasFiles": official, "tjaSize": official,
				"previewPath": !official, "audioPreview": !official,
				"id": true, "nijiiroId": true, "title": true, "difficulties": true, "categoryIds": true,
			} {
				if _, has := fields[name]; has != want {
					t.Errorf("official %v, %s: %s present %v, want %v", official, holder, name, has, want)
				}
			}
			if official && (string(fields["officialId"]) != "null" || string(fields["hasFiles"]) != "true" || string(fields["tjaSize"]) != "7") {
				t.Errorf("official fields: %s", data)
			}
			if !official && string(fields["previewPath"]) != `"clip.ogg"` {
				t.Errorf("fanmade fields: %s", data)
			}
			// What was written reads back as the same chart.
			var back Chart
			if e = json.Unmarshal(data, &back); e != nil || back.ID != c.ID || back.Title != c.Title {
				t.Errorf("read back %+v %v", back, e)
			}
		}
	}
}
