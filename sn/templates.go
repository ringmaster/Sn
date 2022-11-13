package sn

import (
	"io/ioutil"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/aymerick/raymond"
	"github.com/spf13/viper"
)

func GetTemplateFileFromConfig(configPath string, alternative string) string {
	var template string
	if template = viper.GetString(configPath); template == "" {
		template = alternative
	}
	return path.Join(viper.GetString("path"), viper.GetString("template_dir"), template)
}

func RenderTemplateFile(filename string, context map[string]interface{}) (string, error) {
	file, err := ioutil.ReadFile(filename)
	if err != nil {
		return string(file), err
	}

	return raymond.Render(string(file), context)
}

func RegisterTemplateHelpers() {
	raymond.RegisterHelper("dateformat", func(t time.Time, format string) string {
		return t.Format(format)
	})
	raymond.RegisterHelper("more", func(html string, pcount int, options *raymond.Options) string {
		more := ""
		re := regexp.MustCompile(`<!--\s*more\s*-->`)
		split := re.Split(html, -1)
		if len(split) > 1 {
			return split[0] + options.Fn()
		}

		doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
		if err != nil {
			return "<p>NewDocument() error</p>" + html
		}

		doc.Find("p").EachWithBreak(func(i int, sel *goquery.Selection) bool {
			tp, err := goquery.OuterHtml(sel)
			if err == nil {
				if sel.Text() != "" {
					more = more + tp
					pcount--
				}
			}
			if pcount <= 0 {
				return false
			}
			return true
		})

		return more + options.Fn()
	})
	raymond.RegisterHelper("paginate", func(context interface{}, paragraphs int, options *raymond.Options) string {
		return options.FnWith(context)
	})
}
