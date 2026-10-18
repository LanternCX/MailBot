package mailbot

import (
	"strings"
	"testing"
)

func TestVerificationCardKeepsCopyableCode(t *testing.T) {
	for _, tc := range []struct{ subject, body, code string }{
		{"Your verification code", "Your verification code is 123456.", "123456"},
		{"登录验证码", "验证码：\n\n482731\n请勿分享。", "482731"},
		{"Verification code: 482731", "Use this to sign in.", "482731"},
		{"Your OTP", "482731\nThis expires in 10 minutes.", "482731"},
		{"登录验证码", "482731 是您的验证码，请勿泄露。", "482731"},
		{"Authentication code", "Your authentication code is A7B2C9.", "A7B2C9"},
	} {
		t.Run(tc.subject+tc.body, func(t *testing.T) {
			card := readingCard(Email{Subject: tc.subject, Body: tc.body}, "https://mail.example/read/test")
			if !card.HTML || !strings.Contains(card.Text, "<code>"+tc.code+"</code>") {
				t.Fatalf("missing copyable code: %s", card.Text)
			}
			if len(card.Buttons) != 1 || len(card.Buttons[0]) != 2 || card.Buttons[0][1].CopyText == nil || card.Buttons[0][1].CopyText.Text != tc.code {
				t.Fatal("missing copy button")
			}
			original := originalCard(Email{Subject: tc.subject, Body: tc.body})
			if !strings.Contains(original.Text, "<code>"+tc.code+"</code>") {
				t.Fatal("legacy notification lost copyable code")
			}
		})
	}
}

func TestOrdinaryMailDoesNotInventVerificationCode(t *testing.T) {
	for _, e := range []Email{{Subject: "Invoice 482731", Body: "Total 1234"}, {Subject: "Verification code", Body: "Your code could not be generated."}} {
		if verificationCode(e) != "" {
			t.Fatal("invented verification code")
		}
	}
}
