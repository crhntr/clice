package main

import (
	"bytes"
	"html/template"
	"log"
	"net/http"
	"sync/atomic"
)

func main() {
	var templates = template.Must(template.New("").Parse(
		/* language=gohtml */ `
{{define "index" -}}
<!DOCTYPE html>
<html lang='en'>
<head>
<title>Hello</title>
<script type="module" src="https://cdn.jsdelivr.net/gh/starfederation/datastar@1.0.0-RC.7/bundles/datastar.js"></script>
</head>
<body>
<button data-on:click="@get('/endpoint')">
    Open the pod bay doors, HAL.
</button>
<div id="hal">
<input name='something' data-abc>
</div>
</body>
</html>
{{end}}

{{define "response" -}}
<div id="hal">
<input name='something' data-banana='{{.}}'>
I’m sorry, Dave. I’m afraid I can’t do that.</div>
{{- end}}
`))

	var num int64
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(res http.ResponseWriter, req *http.Request) {
		var buf bytes.Buffer
		_ = templates.ExecuteTemplate(&buf, "index", struct{}{})
		res.WriteHeader(http.StatusOK)
		_, _ = res.Write(buf.Bytes())
	})
	mux.HandleFunc("GET /endpoint", func(res http.ResponseWriter, req *http.Request) {
		atomic.AddInt64(&num, 1)
		var buf bytes.Buffer
		_ = templates.ExecuteTemplate(&buf, "response", num)
		res.WriteHeader(http.StatusOK)
		_, _ = res.Write(buf.Bytes())
	})
	if err := http.ListenAndServe(":8081", mux); err != nil {
		log.Fatal(err)
	}
}
