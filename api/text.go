package api

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/gobuffalo/packr"
)

// Text contains static overridable texts used in explorer
var Text struct {
	BlockbookAbout, TOSLink string
}

func init() {
	box := packr.NewBox("../build/text")
	if about, err := box.MustString("about"); err == nil {
		Text.BlockbookAbout = strings.TrimSpace(about)
	} else {
		panic(err)
	}
	if tosLink, err := box.MustString("tos_link"); err == nil {
		if _, err := url.ParseRequestURI(tosLink); err == nil {
			Text.TOSLink = strings.TrimSpace(tosLink)
		} else {
			panic(fmt.Sprint("tos_link is not valid URL:", err.Error()))
		}
	} else {
		panic(err)
	}
}
