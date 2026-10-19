package mailbot

import (
	"strings"
	"testing"
)

func TestAnalysisValidationMatchesLegacyModel(t *testing.T) {
	for _, input := range []string{
		`{}`, `{"priority":3}`, `{"priority":null}`, `{"priority":9}`, `{"priority":{}}`,
		`{"source_language":null,"extracted_code":null,"translation":null}`,
		`{"Priority":5,"Summary":null}`,
	} {
		t.Run(input, func(t *testing.T) {
			a, err := parseAnalysis(input)
			if err != nil || a.Summary != "Unable to generate summary" || a.Category != "notification" {
				t.Fatalf("legacy valid analysis %+v: %v", a, err)
			}
		})
	}
	for _, input := range []string{
		`null`, `[]`, `true`, `"text"`,
		`{"summary":null}`, `{"category":null}`,
		`{"summary":42}`, `{"category":true}`, `{"source_language":7}`,
		`{"extracted_code":123456}`, `{"translation":false}`,
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

func TestAnalysisCardsOmitMailPriority(t *testing.T) {
	a, err := parseAnalysis(`{"summary":"Your invoice is available","category":"billing","priority":1,"extracted_code":"123456"}`)
	if err != nil {
		t.Fatal(err)
	}
	for _, card := range []Card{summaryCard(a), manualAnalysisCard(a)} {
		if !strings.Contains(card.Text, "Your invoice is available") || !strings.Contains(card.Text, "123456") {
			t.Fatal("lost useful analysis")
		}
		for _, label := range []string{"Priority", "Urgent", "Medium", "🔴", "🟡"} {
			if strings.Contains(card.Text, label) {
				t.Fatalf("priority still displayed: %s", card.Text)
			}
		}
	}
}
