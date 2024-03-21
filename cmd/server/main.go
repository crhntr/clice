package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"flag"
	"html/template"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/crhntr/clice"
)

//go:embed index.html.template
var indexHTMLTemplate string

func main() {
	table := clice.NewTable(10, 10)
	flag.IntVar(&table.ColumnCount, "columns", table.ColumnCount, "the number of table columns")
	flag.IntVar(&table.RowCount, "rows", table.RowCount, "the number of table rows")
	flag.Parse()
	s := server{
		table:     table,
		templates: template.Must(template.New("index.html.template").Option("missingkey=error").Parse(indexHTMLTemplate)),
	}
	log.Println("starting server")
	log.Fatal(http.ListenAndServe(":8080", s.routes()))
}

type server struct {
	table clice.Table
	mut   sync.RWMutex

	templates *template.Template
}

func (server *server) routes() *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /", server.index)
	mux.HandleFunc("GET /table.json", server.getTableJSON)
	mux.HandleFunc("POST /table.json", server.postTableJSON)
	mux.HandleFunc("GET /cell/{id}", server.getCellEdit)
	mux.HandleFunc("PATCH /table", server.patchTable)

	return mux
}

func (server *server) render(res http.ResponseWriter, _ *http.Request, name string, status int, data any) {
	var buf bytes.Buffer
	if err := server.templates.ExecuteTemplate(&buf, name, data); err != nil {
		http.Error(res, err.Error(), http.StatusInternalServerError)
		return
	}
	header := res.Header()
	header.Set("content-type", "text/html")
	res.WriteHeader(status)
	_, _ = res.Write(buf.Bytes())
}

func (server *server) index(res http.ResponseWriter, req *http.Request) {
	server.mut.RLock()
	defer server.mut.RUnlock()
	server.render(res, req, "index.html.template", http.StatusOK, &server.table)
}

func (server *server) getCellEdit(res http.ResponseWriter, req *http.Request) {
	server.mut.RLock()
	defer server.mut.RUnlock()

	column, row, err := clice.CellID(req.PathValue("id"))
	if err != nil {
		http.Error(res, err.Error(), http.StatusBadRequest)
		return
	}

	cell := server.table.Cell(column, row)
	server.render(res, req, "edit-cell", http.StatusOK, cell)
}

func (server *server) getTableJSON(res http.ResponseWriter, _ *http.Request) {
	server.mut.RLock()
	defer server.mut.RUnlock()

	filtered := server.table.Cells[:0]
	for _, cell := range server.table.Cells {
		if !cell.HasExpression() {
			continue
		}
		filtered = append(filtered, cell)
	}
	server.table.Cells = filtered

	buf, err := json.MarshalIndent(server.table, "", "\t")
	if err != nil {
		http.Error(res, err.Error(), http.StatusBadRequest)
		return
	}
	h := res.Header()
	h.Set("content-type", "application/json")
	h.Set("content-length", strconv.Itoa(len(buf)))
	res.WriteHeader(http.StatusOK)
	_, _ = res.Write(buf)
}

func (server *server) postTableJSON(res http.ResponseWriter, req *http.Request) {
	if err := req.ParseMultipartForm((1 << 10) * 10); err != nil {
		http.Error(res, err.Error(), http.StatusBadRequest)
		return
	}
	tableJSONHeaders, ok := req.MultipartForm.File["table.json"]
	if !ok || len(tableJSONHeaders) == 0 {
		http.Error(res, "expected table.json file", http.StatusBadRequest)
		return
	}
	f, err := tableJSONHeaders[0].Open()
	if err != nil {
		http.Error(res, err.Error(), http.StatusBadRequest)
		return
	}
	defer closeAndIgnoreError(f)
	tableJSON, err := io.ReadAll(f)
	if err != nil {
		http.Error(res, err.Error(), http.StatusBadRequest)
		return
	}
	var table clice.Table
	if err = json.Unmarshal(tableJSON, &table); err != nil {
		log.Fatal(err)
	}
	if err := table.Evaluate(); err != nil {
		http.Error(res, err.Error(), http.StatusBadRequest)
		return
	}
	server.mut.Lock()
	defer server.mut.Unlock()
	server.table = table
	server.render(res, req, "table", http.StatusOK, &server.table)
}

func closeAndIgnoreError(c io.Closer) {
	_ = c.Close()
}

func (server *server) patchTable(res http.ResponseWriter, req *http.Request) {
	if err := req.ParseForm(); err != nil {
		http.Error(res, err.Error(), http.StatusBadRequest)
		return
	}
	server.mut.Lock()
	defer server.mut.Unlock()
	var assignments []clice.Assignment
	const prefix = "cell-"
	for key, value := range req.Form {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		assignments = append(assignments, clice.Assignment{
			Identifier: key[len(prefix):],
			Expression: value[0],
		})
	}
	err := server.table.Apply(assignments...)
	if err != nil {
		log.Println(err)
		http.Error(res, err.Error(), http.StatusBadRequest)
		return
	}
	server.render(res, req, "table", http.StatusOK, &server.table)
}
