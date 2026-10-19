package mailbot

import "fmt"

// UI language is independent of the language used to summarize and translate mail.
func (s *Service) uiLanguage() string {
	v := s.store.Setting("ui_language", s.config.UILanguage)
	if v != "en" && v != "ja" {
		return "zh"
	}
	return v
}

var uiText = map[string][3]string{
	"mail":              {"原邮件", "Original Email", "元のメール"},
	"from":              {"发件人", "From", "差出人"},
	"subject":           {"主题", "Subject", "件名"},
	"date":              {"时间", "Date", "日時"},
	"content":           {"正文", "Content", "本文"},
	"ai_summary":        {"AI 概括", "AI Summary", "AI 要約"},
	"ai_analysis":       {"AI 分析", "AI Analysis", "AI 分析"},
	"category":          {"类别", "Category", "分類"},
	"already":           {"已使用此语言", "Already using this language", "この言語は選択済みです"},
	"reply_required":    {"请回复一条消息后使用 /ai。", "Reply to a message before using /ai.", "メッセージに返信して /ai を使用してください。"},
	"no_text":           {"回复的消息没有可分析的文字。", "The replied message has no text to analyze.", "返信先のメッセージに分析するテキストがありません。"},
	"unknown":           {"未知操作", "Unknown action", "不明な操作"},
	"loading":           {"正在加载…", "Loading…", "読み込み中…"},
	"expired":           {"邮件缓存已过期，无法加载。", "Email cache expired; cannot load this email.", "メールのキャッシュが期限切れです。"},
	"ai_error":          {"AI 分析失败，请稍后重试。", "AI analysis failed. Try again later.", "AI 分析に失敗しました。後で再試行してください。"},
	"ai_busy":           {"AI 正忙，请稍后重试。", "AI is busy. Try again later.", "AI が使用中です。後で再試行してください。"},
	"translation":       {"翻译", "Translation", "翻訳"},
	"no_translation":    {"暂无译文。", "No translation available.", "翻訳はありません。"},
	"never":             {"暂无", "never", "未実施"},
	"last_check":        {"最近成功检查", "last successful check", "最終確認成功"},
	"last_delivery":     {"目标 %d 最近投递：%s", "Target %d last delivery: %s", "送信先 %d 最終配信：%s"},
	"settings":          {"设置", "Settings", "設定"},
	"help":              {"使用帮助", "Help", "ヘルプ"},
	"status":            {"服务状态", "Service status", "サービス状態"},
	"ui":                {"界面语言", "Interface language", "表示言語"},
	"ai":                {"AI 主要语言", "AI output language", "AI 出力言語"},
	"back":              {"返回", "Back", "戻る"},
	"close":             {"关闭", "Close", "閉じる"},
	"choose":            {"请选择语言", "Choose a language", "言語を選択"},
	"enabled":           {"自动概括 · 阅读页翻译", "Automatic summaries · Translation in reader", "自動要約 · 閲覧ページで翻訳"},
	"disabled":          {"AI 未启用，显示邮件预览", "AI disabled; showing mail previews", "AI 無効：メールのプレビューを表示"},
	"intro":             {"自动概括邮件，点击「查看正文」阅读原邮件或翻译。验证码可一键复制。\n\n/settings — 界面和 AI 语言\n/status — 服务状态\n\nAI 不可用时仍正常发送邮件通知。", "Mail is summarized automatically. Open the original to read or translate it. Copy verification codes with one tap.\n\n/settings — Interface and AI languages\n/status — Service status\n\nNotifications still arrive when AI is unavailable.", "メールを自動要約します。本文を開くと原文の閲覧と翻訳ができます。認証コードはワンタップでコピーできます。\n\n/settings — 表示言語と AI 言語\n/status — サービス状態\n\nAI が使えない場合も通知を送信します。"},
	"retired":           {"无需选择模式或设置规则，启用 AI 后自动概括。", "No modes or rules needed; AI summarizes automatically when enabled.", "モードやルールの設定は不要です。AI を有効にすると自動要約します。"},
	"invalid":           {"不支持的语言", "Unsupported language", "未対応の言語"},
	"saved":             {"语言已保存", "Language saved", "言語を保存しました"},
	"save_error":        {"保存失败，请重试", "Unable to save; please retry", "保存できません。再試行してください"},
	"original":          {"查看原邮件", "View original email", "原文を表示"},
	"body":              {"查看正文", "View email", "本文を表示"},
	"copy":              {"复制验证码", "Copy code", "コードをコピー"},
	"code":              {"验证码", "Verification code", "認証コード"},
	"translate":         {"翻译为 %s", "Translate to %s", "%sに翻訳"},
	"translating":       {"正在翻译…", "Translating…", "翻訳中…"},
	"note":              {"AI 翻译 · %s，请以原邮件为准", "AI translation · %s. Refer to the original email for accuracy.", "AI 翻訳 · %s。原文を確認してください。"},
	"private":           {"请勿转发此邮件的专属链接", "Do not share this private email link", "このメール専用リンクを共有しないでください"},
	"translation_error": {"暂时无法翻译，请稍后重试。原邮件仍可正常阅读。", "Translation is unavailable. Please retry later; the original email remains readable.", "翻訳できません。後で再試行してください。原文は閲覧できます。"},
	"too_long":          {"这封邮件过长，暂不支持翻译。请查看原邮件。", "This email is too long to translate. Please read the original.", "メールが長すぎるため翻訳できません。原文を確認してください。"},
	"origin_error":      {"请求来源无效。", "Invalid request origin.", "リクエスト元が無効です。"},
	"summary":           {"自动概括", "Automatic summaries", "自動要約"},
	"pending":           {"待投递邮件", "Pending deliveries", "配信待ち"},
	"database_error":    {"状态数据库不可用。", "State database unavailable.", "状態データベースが利用できません。"},
}

func uiString(language, key string, args ...any) string {
	i := 0
	if language == "en" {
		i = 1
	} else if language == "ja" {
		i = 2
	}
	v, ok := uiText[key]
	if !ok {
		return key
	}
	if len(args) > 0 {
		return fmt.Sprintf(v[i], args...)
	}
	return v[i]
}

func (s *Service) tr(key string, args ...any) string { return uiString(s.uiLanguage(), key, args...) }
