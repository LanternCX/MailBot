package mailbot

import (
	"regexp"
	"strings"
)

// Classify locally so authentication mail never waits for a model response.
var verificationPattern = regexp.MustCompile(`(?i)(验证码|驗證碼|校验码|校驗碼|动态码|動態碼|一次性密码|登录码|登入碼|認証コード|確認コード|\bverification\s+code\b|\bauthentication\s+code\b|\bsecurity\s+code\b|\blogin\s+code\b|\bsign.in\s+code\b|\bone.time\s+(?:password|code)\b|\bpasscode\b|\botp\b)`)

func verificationMail(e Email) bool {
	return verificationPattern.MatchString(e.Subject + "\n" + truncate(e.Body, 4000))
}

var verificationToken = regexp.MustCompile(`\b[A-Za-z0-9]{4,10}\b`)

// Match near the same local keywords that select the verification delivery
// lane, including subject-only codes and multiline HTML-to-text bodies.
func verificationCode(e Email) string {
	text := e.Subject + "\n" + truncate(e.Body, 4000)
	labels := verificationPattern.FindAllStringIndex(text, -1)
	best, score := "", 121
	for _, token := range verificationToken.FindAllStringIndex(text, -1) {
		code := text[token[0]:token[1]]
		digits := 0
		for _, r := range code {
			if r >= '0' && r <= '9' {
				digits++
			}
		}
		if digits == 0 || (digits == len(code) && len(code) > 8) || (digits < len(code) && len(code) < 6) {
			continue
		}
		for _, label := range labels {
			distance := 121
			if token[0] >= label[1] {
				distance = token[0] - label[1]
			} else if token[1] <= label[0] {
				distance = label[0] - token[1] + 10
			}
			if distance < score {
				best, score = code, distance
			}
		}
	}
	return strings.TrimSpace(best)
}
