package web

import "embed"

//go:embed static/index.html static/merchant.html static/login.html static/confirmed.html static/css static/js static/img
var StaticFS embed.FS
