package mailbot

import "testing"

func TestAnalysisValidationMatchesLegacyModel(t *testing.T) {
	for _, tc := range []struct {
		input    string
		priority int
	}{
		{`{}`, 3}, {`{"priority":3}`, 3}, {`{"priority":3.0}`, 3},
		{`{"priority":"3"}`, 3}, {`{"priority":"+3.00"}`, 3},
		{`{"priority":" 03 "}`, 3}, {`{"priority":"0_3"}`, 3},
		{`{"priority":true}`, 1}, {`{"priority":3e0}`, 3},
		{`{"source_language":null,"extracted_code":null,"translation":null}`, 3},
		{`{"Priority":5,"Summary":null}`, 3},
	} {
		t.Run(tc.input, func(t *testing.T) {
			a, err := parseAnalysis(tc.input)
			if err != nil || a.Priority != tc.priority || a.Summary != "Unable to generate summary" || a.Category != "notification" {
				t.Fatalf("legacy valid analysis %+v: %v", a, err)
			}
		})
	}
	for _, input := range []string{
		`null`, `[]`, `true`, `"text"`,
		`{"summary":null}`, `{"category":null}`, `{"priority":null}`,
		`{"summary":42}`, `{"category":true}`, `{"source_language":7}`,
		`{"extracted_code":123456}`, `{"translation":false}`,
		`{"priority":false}`, `{"priority":0}`, `{"priority":6}`, `{"priority":3.5}`,
		`{"priority":"3e0"}`, `{"priority":"3."}`, `{"priority":"3.0000000000000001"}`,
		`{"priority":"3_0"}`, `{"priority":{}}`,
	} {
		t.Run(input, func(t *testing.T) {
			if _, err := parseAnalysis(input); err == nil {
				t.Fatalf("invalid analysis accepted: %s", input)
			}
		})
	}
	a, err := parseAnalysis(`{"summary":"summary","category":"custom category","extracted_code":"123456","translation":"translation","source_language":"en","unknown":true}`)
	if err != nil || a.Summary != "summary" || a.Category != "custom category" || a.Code == nil || *a.Code != "123456" || a.Translation == nil || *a.Translation != "translation" || a.SourceLanguage != "en" {
		t.Fatalf("legacy string fields lost: %+v %v", a, err)
	}
}
