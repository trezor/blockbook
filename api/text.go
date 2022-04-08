package api

import (
	"embed"
	"fmt"
	"net/url"
	"strings"
)

//go:embed embed/*
var embedded embed.FS

// Text contains static overridable texts used in explorer
var Text struct {
	BlockbookAbout, TOSLink string
}

func init() {
	if about, err := embedded.ReadFile("embed/about"); err == nil {
		Text.BlockbookAbout = strings.TrimSpace(string(about))
	} else {
		panic(err)
	}
	if tosLinkB, err := embedded.ReadFile("embed/tos_link"); err == nil {
		tosLink := strings.TrimSpace(string(tosLinkB))
		if _, err := url.ParseRequestURI(tosLink); err == nil {
			Text.TOSLink = tosLink
		} else {
			panic(fmt.Sprint("tos_link is not valid URL:", err.Error()))
		}
	} else {
		panic(err)
	}
}
