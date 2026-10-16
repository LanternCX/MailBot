package mailbot

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestCompatibleAIHandlesUnsupportedJSONFormat(t *testing.T) {
	for _, unsupported := range []bool{true, false} {
		t.Run(fmt.Sprint(unsupported), func(t *testing.T) {
			var calls atomic.Int32
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				attempt := calls.Add(1)
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if body["model"] != "fixture" || body["messages"] == nil || r.Header.Get("Authorization") != "Bearer fixture-key" {
					t.Error("retry lost original request")
				}
				if attempt == 1 {
					if body["response_format"] == nil {
						t.Error("initial format missing")
					}
					w.WriteHeader(http.StatusBadRequest)
					if unsupported {
						fmt.Fprint(w, `{"error":{"param":"response_format","code":"unsupported_parameter","message":"Not supported"}}`)
					} else {
						fmt.Fprint(w, `{"error":{"param":"model","message":"Invalid model"}}`)
					}
					return
				}
				if body["response_format"] != nil {
					t.Error("unsupported format retained")
				}
				fmt.Fprint(w, `{"choices":[{"message":{"content":"{\"summary\":\"Fallback summary\",\"priority\":3}"}}]}`)
			}))
			defer api.Close()
			n, _ := NewNetwork(nil, time.Second)
			defer n.Close()
			s := &Service{store: testStore(t), network: n, configDir: t.TempDir(), config: Config{AI: AIConfig{Enabled: true, Provider: "custom", Model: "fixture", APIKey: "fixture-key", BaseURL: api.URL}}}
			a, err := s.analyze(context.Background(), Email{Body: "test"})
			if unsupported {
				if err != nil || a.Summary != "Fallback summary" || calls.Load() != 2 {
					t.Fatalf("format fallback %+v %v calls=%d", a, err, calls.Load())
				}
			} else if err == nil || calls.Load() != 1 {
				t.Fatalf("unrelated error retried: %v calls=%d", err, calls.Load())
			}
		})
	}
}
