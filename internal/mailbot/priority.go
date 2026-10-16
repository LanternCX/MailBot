package mailbot

import "regexp"

// Classify locally so authentication mail never waits for a model response.
var verificationPattern = regexp.MustCompile(`(?i)(验证码|驗證碼|校验码|校驗碼|动态码|動態碼|一次性密码|登录码|登入碼|認証コード|確認コード|\bverification\s+code\b|\bauthentication\s+code\b|\bsecurity\s+code\b|\blogin\s+code\b|\bsign.in\s+code\b|\bone.time\s+(?:password|code)\b|\bpasscode\b|\botp\b)`)

func verificationMail(e Email) bool {
	return verificationPattern.MatchString(e.Subject + "\n" + truncate(e.Body, 4000))
}
