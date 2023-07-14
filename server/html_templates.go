package server

import (
	"encoding/json"
	"fmt"
	"html/template"
	"math/big"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/trezor/blockbook/api"
	"github.com/trezor/blockbook/common"
)

type tpl int

const (
	noTpl = tpl(iota)
	errorTpl
	errorInternalTpl
)

// htmlTemplateHandler is a handle to public http server
type htmlTemplates[TD any] struct {
	metrics                  *common.Metrics
	templates                []*template.Template
	debug                    bool
	newTemplateData          func(r *http.Request) *TD
	newTemplateDataWithError func(error *api.APIError, r *http.Request) *TD
	parseTemplates           func() []*template.Template
	postHtmlTemplateHandler  func(data *TD, w http.ResponseWriter, r *http.Request)
}

func (s *htmlTemplates[TD]) htmlTemplateHandler(handler func(w http.ResponseWriter, r *http.Request) (tpl, *TD, error)) func(w http.ResponseWriter, r *http.Request) {
	handlerName := getFunctionName(handler)
	return func(w http.ResponseWriter, r *http.Request) {
		var t tpl
		var data *TD
		var err error
		defer func() {
			if e := recover(); e != nil {
				glog.Error(handlerName, " recovered from panic: ", e)
				debug.PrintStack()
				t = errorInternalTpl
				if s.debug {
					data = s.newTemplateDataWithError(&api.APIError{Text: fmt.Sprint("Internal server error: recovered from panic ", e)}, r)
				} else {
					data = s.newTemplateDataWithError(&api.APIError{Text: "Internal server error"}, r)
				}
			}
			// noTpl means the handler completely handled the request
			if t != noTpl {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				// return 500 Internal Server Error with errorInternalTpl
				if t == errorInternalTpl {
					w.WriteHeader(http.StatusInternalServerError)
				}
				if err := s.templates[t].ExecuteTemplate(w, "base.html", data); err != nil {
					glog.Error(err)
				}
			}
			if s.metrics != nil {
				s.metrics.ExplorerPendingRequests.With((common.Labels{"method": handlerName})).Dec()
			}
		}()
		if s.metrics != nil {
			s.metrics.ExplorerPendingRequests.With((common.Labels{"method": handlerName})).Inc()
		}
		if s.debug {
			// reload templates on each request
			// to reflect changes during development
			s.templates = s.parseTemplates()
		}
		t, data, err = handler(w, r)
		if err != nil || (data == nil && t != noTpl) {
			t = errorInternalTpl
			if apiErr, ok := err.(*api.APIError); ok {
				data = s.newTemplateDataWithError(apiErr, r)
				if apiErr.Public {
					t = errorTpl
				}
			} else {
				if err != nil {
					glog.Error(handlerName, " error: ", err)
				}
				if s.debug {
					data = s.newTemplateDataWithError(&api.APIError{Text: fmt.Sprintf("Internal server error: %v, data %+v", err, data)}, r)
				} else {
					data = s.newTemplateDataWithError(&api.APIError{Text: "Internal server error"}, r)
				}
			}
		}
		if s.postHtmlTemplateHandler != nil {
			s.postHtmlTemplateHandler(data, w, r)
		}

	}
}

func relativeTimeUnit(d int64) string {
	var u string
	if d < 60 {
		if d == 1 {
			u = " sec"
		} else {
			u = " secs"
		}
	} else if d < 3600 {
		d /= 60
		if d == 1 {
			u = " min"
		} else {
			u = " mins"
		}
	} else if d < 3600*24 {
		d /= 3600
		if d == 1 {
			u = " hour"
		} else {
			u = " hours"
		}
	} else {
		d /= 3600 * 24
		if d == 1 {
			u = " day"
		} else {
			u = " days"
		}
	}
	return strconv.FormatInt(d, 10) + u
}

func relativeTime(d int64) string {
	r := relativeTimeUnit(d)
	if d > 3600*24 {
		d = d % (3600 * 24)
		if d >= 3600 {
			r += " " + relativeTimeUnit(d)
		}
	} else if d > 3600 {
		d = d % 3600
		if d >= 60 {
			r += " " + relativeTimeUnit(d)
		}
	}
	return r
}

func unixTimeSpan(ut int64) template.HTML {
	t := time.Unix(ut, 0)
	return timeSpan(&t)
}

var timeNow = time.Now

func timeSpan(t *time.Time) template.HTML {
	if t == nil {
		return ""
	}
	u := t.Unix()
	if u <= 0 {
		return ""
	}
	d := timeNow().Unix() - u
	f := t.UTC().Format("2006-01-02 15:04:05")
	if d < 0 {
		return template.HTML(f)
	}
	r := relativeTime(d)
	return template.HTML(`<span tt="` + f + `">` + r + " ago</span>")
}

func toJSON(data interface{}) string {
	json, err := json.Marshal(data)
	if err != nil {
		return ""
	}
	return string(json)
}

func formatAmountWithDecimals(a *api.Amount, d int) string {
	if a == nil {
		return "0"
	}
	return a.DecimalString(d)
}

func appendAmountSpan(rv *strings.Builder, class, amount, shortcut, txDate string) {
	rv.WriteString(`<span`)
	if class != "" {
		rv.WriteString(` class="`)
		rv.WriteString(class)
		rv.WriteString(`"`)
	}
	if txDate != "" {
		rv.WriteString(` tm="`)
		rv.WriteString(txDate)
		rv.WriteString(`"`)
	}
	rv.WriteString(">")
	i := strings.IndexByte(amount, '.')
	if i < 0 {
		appendSeparatedNumberSpans(rv, amount, "nc")
	} else {
		appendSeparatedNumberSpans(rv, amount[:i], "nc")
		rv.WriteString(`.`)
		rv.WriteString(`<span class="amt-dec">`)
		appendLeftSeparatedNumberSpans(rv, amount[i+1:], "ns")
		rv.WriteString("</span>")
	}
	if shortcut != "" {
		rv.WriteString(" ")
		rv.WriteString(shortcut)
	}
	rv.WriteString("</span>")
}

func appendAmountSpanBitcoinType(rv *strings.Builder, class, amount, shortcut, txDate string) {
	if amount == "0" {
		appendAmountSpan(rv, class, amount, shortcut, txDate)
		return
	}
	rv.WriteString(`<span`)
	if class != "" {
		rv.WriteString(` class="`)
		rv.WriteString(class)
		rv.WriteString(`"`)
	}
	if txDate != "" {
		rv.WriteString(` tm="`)
		rv.WriteString(txDate)
		rv.WriteString(`"`)
	}
	rv.WriteString(">")
	i := strings.IndexByte(amount, '.')
	var decimals string
	if i < 0 {
		appendSeparatedNumberSpans(rv, amount, "nc")
		decimals = "00000000"
	} else {
		appendSeparatedNumberSpans(rv, amount[:i], "nc")
		decimals = amount[i+1:] + "00000000"
	}
	rv.WriteString(`.`)
	rv.WriteString(`<span class="amt-dec">`)
	rv.WriteString(decimals[:2])
	rv.WriteString(`<span class="ns">`)
	rv.WriteString(decimals[2:5])
	rv.WriteString("</span>")
	rv.WriteString(`<span class="ns">`)
	rv.WriteString(decimals[5:8])
	rv.WriteString("</span>")
	rv.WriteString("</span>")
	if shortcut != "" {
		rv.WriteString(" ")
		rv.WriteString(shortcut)
	}
	rv.WriteString("</span>")
}

func appendAmountWrapperSpan(rv *strings.Builder, primary, symbol, classes string) {
	rv.WriteString(`<span class="amt`)
	if classes != "" {
		rv.WriteString(` `)
		rv.WriteString(classes)
	}
	rv.WriteString(`" cc="`)
	rv.WriteString(primary)
	rv.WriteString(" ")
	rv.WriteString(symbol)
	rv.WriteString(`">`)
}

func formatInt(i int) template.HTML {
	return formatInt64(int64(i))
}

func formatUint32(i uint32) template.HTML {
	return formatInt64(int64(i))
}

func appendSeparatedNumberSpans(rv *strings.Builder, s, separatorClass string) {
	if len(s) > 0 && s[0] == '-' {
		s = s[1:]
		rv.WriteByte('-')
	}
	t := (len(s) - 1) / 3
	if t <= 0 {
		rv.WriteString(s)
	} else {
		t *= 3
		rv.WriteString(s[:len(s)-t])
		for i := len(s) - t; i < len(s); i += 3 {
			rv.WriteString(`<span class="`)
			rv.WriteString(separatorClass)
			rv.WriteString(`">`)
			rv.WriteString(s[i : i+3])
			rv.WriteString("</span>")
		}
	}
}

func appendLeftSeparatedNumberSpans(rv *strings.Builder, s, separatorClass string) {
	l := len(s)
	if l <= 3 {
		rv.WriteString(s)
	} else {
		rv.WriteString(s[:3])
		for i := 3; i < len(s); i += 3 {
			rv.WriteString(`<span class="`)
			rv.WriteString(separatorClass)
			rv.WriteString(`">`)
			e := i + 3
			if e > l {
				e = l
			}
			rv.WriteString(s[i:e])
			rv.WriteString("</span>")
		}
	}
}

func formatInt64(i int64) template.HTML {
	s := strconv.FormatInt(i, 10)
	var rv strings.Builder
	appendSeparatedNumberSpans(&rv, s, "ns")
	return template.HTML(rv.String())
}

func formatBigInt(i *big.Int) template.HTML {
	if i == nil {
		return ""
	}
	s := i.String()
	var rv strings.Builder
	appendSeparatedNumberSpans(&rv, s, "ns")
	return template.HTML(rv.String())
}
