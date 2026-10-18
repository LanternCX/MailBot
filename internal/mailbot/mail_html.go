package mailbot

import (
	"bytes"
	"fmt"
	"html"
	"net/url"
	"strings"

	"github.com/aymerick/douceur/css"
	"github.com/aymerick/douceur/parser"
	"github.com/microcosm-cc/bluemonday"
	xhtml "golang.org/x/net/html"
)

func safeImageType(kind string) bool {
	return kind == "image/png" || kind == "image/jpeg" || kind == "image/gif" || kind == "image/webp"
}

// Use the same CSS value validation for style attributes and stylesheet rules.
// Keep layout properties, but never permit resource fetching through CSS.
var mailStyleProperties = strings.Fields(`color background-color font-family font-size font-weight font-style font-variant line-height text-align text-decoration text-indent text-transform letter-spacing word-spacing white-space word-break overflow-wrap vertical-align display visibility opacity width min-width max-width height min-height max-height margin margin-top margin-right margin-bottom margin-left padding padding-top padding-right padding-bottom padding-left border border-top border-right border-bottom border-left border-color border-width border-style border-radius border-collapse border-spacing table-layout float clear box-sizing overflow overflow-x overflow-y list-style-type list-style-position`)

var mailStylePolicy = func() *bluemonday.Policy {
	p := bluemonday.NewPolicy()
	p.AllowElements("span")
	p.AllowStyles(mailStyleProperties...).Globally()
	return p
}()

func safeMailDeclarations(declarations []*css.Declaration) string {
	var out strings.Builder
	for _, d := range declarations {
		// Reject escapes and comments before value validation; they can disguise
		// URLs and legacy browser expressions. Resource CSS is intentionally absent.
		value := strings.ToLower(d.Value)
		if strings.ContainsAny(value, `\<>`) || strings.Contains(value, "/*") || strings.Contains(value, "url(") || strings.Contains(value, "expression") {
			continue
		}
		clean := mailStylePolicy.Sanitize(`<span style="` + html.EscapeString(d.String()) + `">x</span>`)
		doc, _ := xhtml.Parse(strings.NewReader(clean))
		var walk func(*xhtml.Node)
		walk = func(n *xhtml.Node) {
			for _, a := range n.Attr {
				if n.Data == "span" && a.Key == "style" {
					out.WriteString(a.Val)
					if d.Important {
						out.WriteString(" !important")
					}
					out.WriteByte(';')
				}
			}
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				walk(c)
			}
		}
		walk(doc)
	}
	return out.String()
}

func safeMailStylesheet(text string) string {
	sheet, err := parser.Parse(text)
	if err != nil {
		return ""
	}
	var clean func([]*css.Rule, int) string
	clean = func(rules []*css.Rule, depth int) string {
		if depth > 8 {
			return ""
		}
		var out strings.Builder
		for _, r := range rules {
			if strings.ContainsAny(r.Prelude, `\<>@`) || strings.Contains(r.Prelude, "/*") {
				continue
			}
			if r.Kind == css.QualifiedRule {
				if declarations := safeMailDeclarations(r.Declarations); declarations != "" {
					out.WriteString(r.Prelude + "{" + declarations + "}")
				}
			} else if strings.EqualFold(r.Name, "@media") && !strings.ContainsAny(r.Prelude, "{};") {
				out.WriteString("@media " + r.Prelude + "{" + clean(r.Rules, depth+1) + "}")
			}
		}
		return out.String()
	}
	return clean(sheet.Rules, 0)
}

// Preserve mail layout without mixing its CSS with the surrounding app. HTML
// sanitation is followed by a script-free sandbox and resource-restricting CSP.
func renderMailHTML(e Email, token string, external bool) string {
	doc, err := xhtml.Parse(strings.NewReader(e.HTMLBody))
	if err != nil {
		return "<pre>" + html.EscapeString(e.Body) + "</pre>"
	}
	var styles strings.Builder
	var body *xhtml.Node
	var walk func(*xhtml.Node)
	walk = func(n *xhtml.Node) {
		if n.Type == xhtml.ElementNode {
			if n.Data == "body" {
				body = n
			}
			if n.Data == "style" {
				var text strings.Builder
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					if c.Type == xhtml.TextNode {
						text.WriteString(c.Data)
					}
				}
				styles.WriteString(safeMailStylesheet(text.String()))
			}
			attrs := make([]xhtml.Attribute, 0, len(n.Attr))
			for _, a := range n.Attr {
				if a.Key == "style" {
					// Douceur needs a final semicolon, but rejects an empty
					// declaration produced by appending one to an existing ';'.
					value := strings.TrimSpace(a.Val)
					if !strings.HasSuffix(value, ";") {
						value += ";"
					}
					d, err := parser.ParseDeclarations(value)
					if err != nil {
						continue
					}
					a.Val = safeMailDeclarations(d)
				}
				if a.Key == "srcset" || a.Key == "background" {
					continue
				}
				if a.Key == "src" {
					if n.Data != "img" {
						continue
					}
					src := ""
					if strings.HasPrefix(strings.ToLower(a.Val), "cid:") {
						cid, _ := url.PathUnescape(a.Val[4:])
						for i, img := range e.Images {
							if img.CID == cid && safeImageType(img.Type) {
								src = fmt.Sprintf("/read/%s/image/%d", token, i)
								break
							}
						}
					} else if external {
						if _, err := imageURL(a.Val); err == nil {
							src = "/read/" + token + "/remote/" + imageKey(a.Val)
						}
					}
					if src == "" {
						continue
					}
					a.Val = src
				}
				attrs = append(attrs, a)
			}
			n.Attr = attrs
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	p := bluemonday.UGCPolicy()
	p.AllowAttrs("class", "id", "dir", "lang").Globally()
	p.AllowStyles(mailStyleProperties...).MatchingHandler(func(string) bool { return true }).Globally() // Already checked above.
	p.AllowAttrs("width", "height", "align", "valign", "bgcolor", "border", "cellpadding", "cellspacing").OnElements("table", "tbody", "tr", "td", "th", "img", "div")
	p.AddTargetBlankToFullyQualifiedLinks(true)
	var markup bytes.Buffer
	if body != nil {
		for c := body.FirstChild; c != nil; c = c.NextSibling {
			_ = xhtml.Render(&markup, c)
		}
	}
	bodyAttrs := ""
	if body != nil {
		clone := &xhtml.Node{Type: xhtml.ElementNode, Data: "div", Attr: body.Attr}
		var b bytes.Buffer
		_ = xhtml.Render(&b, clone)
		clean := p.Sanitize(b.String())
		if end := strings.Index(clean, ">"); end >= 0 {
			bodyAttrs = strings.TrimPrefix(clean[:end], "<div")
		}
	}
	return `<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><style>body{display:flow-root;margin:0;overflow-wrap:anywhere}img{max-width:100%;height:auto}pre{white-space:pre-wrap}</style><style>` + styles.String() + `</style></head><body` + bodyAttrs + `>` + p.Sanitize(markup.String()) + `</body></html>`
}
