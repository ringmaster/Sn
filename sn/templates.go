package sn

import (
	"fmt"
	"io/ioutil"
	"os"
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

func RegisterPartials() {
	templatepath := path.Join(viper.GetString("path"), viper.GetString("template_dir"))
	files, err := os.ReadDir(templatepath)
	if err != nil {
		panic(err)
	}

	for _, file := range files {
		fmt.Println(file.Name(), file.IsDir())
		if !file.IsDir() {
			template, err := ioutil.ReadFile(path.Join(templatepath, file.Name()))
			if err != nil {
				panic(err)
			}
			partialname := regexp.MustCompile(`\.`).Split(file.Name(), 2)[0]
			fmt.Printf("Partial %s built from %s\n", partialname, file.Name())
			raymond.RegisterPartial(partialname, string(template))
		}
	}
}

func RegisterTemplateHelpers() {
	raymond.RegisterHelper("keys", func(obj map[string]interface{}) string {
		result := ``
		for k, v := range obj {
			result += fmt.Sprintf("%s: %#v\n", k, v)
		}
		return result
	})
	raymond.RegisterHelper("string", func(str any) string {
		fmt.Printf("%#v", str)
		fmt.Printf("(string) %s\n", str)
		return fmt.Sprintf("<pre>%s</pre>", str)
	})
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
