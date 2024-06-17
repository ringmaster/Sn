package sn

import (
	"fmt"
	"os"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/aymerick/raymond"
	"github.com/spf13/viper"
)

func GetTemplateFilesFromConfig(configPath string) []string {
	var templates []string
	templateList := viper.GetStringSlice(configPath)
	for _, template := range templateList {
		templates = append(templates, path.Join(ConfigPath("template_dir", MustExist()), template))
	}
	return templates
}

func RenderTemplateFiles(filenames []string, context map[string]interface{}) (string, error) {
	concat := ""
	for _, filename := range filenames {
		file, err := os.ReadFile(filename)
		if err != nil {
			return string(file), err
		}
		concat = concat + string(file)
	}

	context["now"] = time.Now()
	var result string
	result, err := raymond.Render(concat, context)

	return result, err
}

func RegisterPartials() {
	templatepath := ConfigPath("template_dir", MustExist())
	files, err := os.ReadDir(templatepath)
	if err != nil {
		panic(err)
	}

	for _, file := range files {
		if !file.IsDir() {
			template, err := os.ReadFile(path.Join(templatepath, file.Name()))
			if err != nil {
				panic(err)
			}
			partialname := regexp.MustCompile(`\.`).Split(file.Name(), 2)[0]
			raymond.RegisterPartial(partialname, string(template))
		}
	}
}

func RegisterTemplateHelpers() {
	var onchange interface{}
	raymond.RegisterHelper("keys", func(obj map[string]interface{}) string {
		result := ``
		for k, v := range obj {
			result += fmt.Sprintf("%s: %#v\n", k, v)
		}
		return result
	})
	raymond.RegisterHelper("string", func(str any) string {
		return fmt.Sprintf("<pre>%s</pre>", str)
	})
	raymond.RegisterHelper("debug", func(str any, options *raymond.Options) string {
		return fmt.Sprintf(`<pre style="">%s</pre>`, str)
	})
	raymond.RegisterHelper("s3", func(src string, options *raymond.Options) string {
		regex := regexp.MustCompile(`s3://(?P<bucket>[^/]+)/(?P<filename>.+)`)

		match := regex.FindStringSubmatch(src)
		if match != nil {
			bucket := match[1]
			filename := match[2]
			cdnURL := viper.GetString(fmt.Sprintf("s3.%s.cdn", bucket))
			newSrc := cdnURL + filename
			return newSrc
		}

		return src
	})
	raymond.RegisterHelper("d", func(options *raymond.Options) string {
		return fmt.Sprintf(`<pre style="">%s</pre>`, options.Ctx())
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
	raymond.RegisterHelper("summary", func(html string, options *raymond.Options) string {
		summary := ""

		doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
		if err != nil {
			return "<p>NewDocument() error</p>" + html
		}

		doc.Find("p").EachWithBreak(func(i int, sel *goquery.Selection) bool {
			if sel.Text() == "" {
				return true
			}
			summary = sel.Text()
			return false
		})

		return summary
	})
	raymond.RegisterHelper("paginate", func(pagelist ItemResult, distance int, options *raymond.Options) raymond.SafeString {
		pagelist.Page = MaxOf(1, pagelist.Page)
		min := MaxOf(pagelist.Page-distance, 1)
		max := MinOf(pagelist.Page+distance, pagelist.Pages)
		paginator := fmt.Sprintf("<!-- Paginator  min: %d  max: %d  pages: %d  page: %d  distance: %d -->", min, max, pagelist.Pages, pagelist.Page, distance)
		for pg := min; pg <= max; pg++ {
			ctx := map[string]interface{}{"page": pg, "active": pg == pagelist.Page}
			paginator += options.FnWith(ctx)
		}
		return raymond.SafeString(paginator)
	})
	raymond.RegisterHelper("withfirst", func(pagelist ItemResult, options *raymond.Options) raymond.SafeString {
		var ctx interface{}
		if len(pagelist.Items) > 0 {
			ctx = pagelist.Items[0]
		} else {
			ctx = nil
		}
		return raymond.SafeString(options.FnWith(ctx))
	})
	raymond.RegisterHelper("define", func(name string, options *raymond.Options) raymond.SafeString {
		content := options.Fn()
		options.DataFrame().Set(name, content)
		return ""
	})
	raymond.RegisterHelper("block", func(name string, options *raymond.Options) raymond.SafeString {
		content := options.DataFrame().Get(name)

		retval := raymond.SafeString(options.HashStr("prefix"))

		if content == nil {
			if options.Fn() != "" {
				retval += raymond.SafeString(options.Fn())
			} else {
				retval = ""
			}
		} else {
			retval += raymond.SafeString(content.(string))
		}
		return retval
	})
	raymond.RegisterHelper("delimit", func(items []string, delimiter string, options *raymond.Options) raymond.SafeString {
		return raymond.SafeString(strings.Join(items, delimiter))
	})
	raymond.RegisterHelper("onchange", func(value any, options *raymond.Options) raymond.SafeString {
		if onchange != value {
			onchange = value
			return raymond.SafeString(options.Fn())
		}
		return ""
	})
}
