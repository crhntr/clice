package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/constant"
	"html/template"
	"io"
	"log"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/crhntr/clice/expression"
)

//go:embed index.html.template
var indexHTMLTemplate string

func main() {
	table := Table{ColumnCount: 10, RowCount: 10}
	flag.IntVar(&table.ColumnCount, "columns", table.ColumnCount, "the number of table columns")
	flag.IntVar(&table.RowCount, "rows", table.RowCount, "the number of table rows")
	flag.Parse()
	s := server{
		table:     table,
		templates: template.Must(template.New("index.html.template").Parse(indexHTMLTemplate)),
	}
	log.Println("starting server")
	log.Fatal(http.ListenAndServe(":8080", s.routes()))
}

type server struct {
	table Table
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

	column, row, err := cellCoordinates(req.PathValue("id"))
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
		if cell.SavedExpression == nil || cell.Expression == nil {
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
	var table Table
	if err = json.Unmarshal(tableJSON, &table); err != nil {
		log.Fatal(err)
	}
	if err := table.calculateValues(); err != nil {
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
	for key, value := range req.Form {
		if !strings.HasPrefix(key, "cell-") {
			continue
		}
		column, row, err := cellCoordinates(key)
		if err != nil {
			http.Error(res, err.Error(), http.StatusBadRequest)
			return
		}
		cell := server.cellPointer(column, row)
		cell.Error = ""
		cell.input = value[0]
		var node expression.Node
		if cell.input != "" {
			node, err = expression.New(cell.input)
			if err != nil {
				cell.Error = err.Error()
				continue
			}
			s, err := expression.Sprint(node)
			if err != nil {
				cell.Error = err.Error()
				continue
			}
			cell.input = s
		}
		cell.Expression = node
	}
	err := server.table.calculateValues()
	if err != nil {
		server.render(res, req, "table", http.StatusOK, &server.table)
		return
	}
	server.render(res, req, "table", http.StatusOK, &server.table)
}

func (server *server) cellPointer(column, row int) *Cell {
	var cell *Cell
	index := slices.IndexFunc(server.table.Cells, func(cell Cell) bool {
		return cell.Row == row && cell.Column == column
	})
	if index >= 0 {
		cell = &server.table.Cells[index]
	} else {
		server.table.Cells = append(server.table.Cells, Cell{
			Row:    row,
			Column: column,
		})
		cell = &server.table.Cells[len(server.table.Cells)-1]
	}
	return cell
}

type Column struct {
	Number int
}

func (column Column) Label() string {
	return columnLabel(column.Number)
}

func columnLabel(n int) string {
	result := ""
	for n >= 0 {
		remainder := n % 26
		result = fmt.Sprintf("%c", remainder+65) + result
		n = n/26 - 1
	}
	return result
}

type Row struct {
	Number int
}

func (row Row) Label() string {
	return strconv.Itoa(row.Number)
}

type Cell struct {
	Row    int
	Column int

	Expression,
	SavedExpression ast.Expr
	Value,
	SavedValue constant.Value

	input,
	Error string
}

func (cell *Cell) ExpressionText() string {
	if cell.Expression != nil && cell.Error == "" {
		s, err := expression.Sprint(cell.Expression)
		if err != nil {
			return cell.input
		}
		return s
	}
	return cell.input
}

type EncodedCell struct {
	ID         string `json:"id"`
	Expression string `json:"ex"`
}

func (cell *Cell) MarshalJSON() ([]byte, error) {
	s, err := expression.Sprint(cell.Expression)
	if err != nil {
		return nil, err
	}
	return json.Marshal(EncodedCell{
		ID:         strings.TrimPrefix(cell.ID(), "cell-"),
		Expression: s,
	})
}

type EncodedTable struct {
	ColumnCount int           `json:"columns"`
	RowCount    int           `json:"rows"`
	Cells       []EncodedCell `json:"cells"`
}

func (table *Table) UnmarshalJSON(in []byte) error {
	var encoded EncodedTable

	if err := json.Unmarshal(in, &encoded); err != nil {
		return err
	}
	table.RowCount = encoded.RowCount
	table.ColumnCount = encoded.ColumnCount
	for _, cell := range encoded.Cells {
		column, row, err := cellCoordinates(cell.ID)
		if err != nil {
			return err
		}
		exp, err := expression.New(cell.Expression)
		if err != nil {
			return err
		}
		table.Cells = append(table.Cells, Cell{
			Column:          column,
			Row:             row,
			SavedExpression: exp,
			Expression:      exp,
		})
	}

	return table.calculateValues()
}

func (cell *Cell) String() string {
	if cell.SavedExpression == nil {
		return ""
	}
	return cell.Value.String()
}

func (cell *Cell) IDPathParam() string {
	return fmt.Sprintf("%s%d", columnLabel(cell.Column), cell.Row)
}
func (cell *Cell) ID() string {
	return "cell-" + cell.IDPathParam()
}

type Table struct {
	ColumnCount int    `json:"columns"`
	RowCount    int    `json:"rows"`
	Cells       []Cell `json:"cells"`
}

func NewTable(columns, rows int) Table {
	table := Table{
		RowCount:    rows,
		ColumnCount: columns,
	}
	return table
}

func (table *Table) Cell(column, row int) *Cell {
	for i, cell := range table.Cells {
		if cell.Row == row && cell.Column == column {
			return &table.Cells[i]
		}
	}
	return &Cell{
		Row:    row,
		Column: column,
	}
}

func (table *Table) Rows() []Row {
	result := make([]Row, table.RowCount)
	for i := range result {
		result[i].Number = i
	}
	return result
}

func (table *Table) Columns() []Column {
	result := make([]Column, table.ColumnCount)
	for i := range result {
		result[i].Number = i
	}
	return result
}

func (table *Table) calculateValues() error {
	for _, cell := range table.Cells {
		if cell.Error != "" {
			return fmt.Errorf("cell parsing error %s", cell.IDPathParam())
		}
	}
	for i := range table.Cells {
		visited := make(visitSet)
		cell := &table.Cells[i]
		err := cell.evaluate(table, visited)
		if err != nil {
			cell.Error = err.Error()
			table.revertCellChanges()
			return err
		}
		cell.Error = ""
	}
	table.saveCellChanges()
	return nil
}

func (table *Table) saveCellChanges() {
	for i := range table.Cells {
		table.Cells[i].SavedValue = table.Cells[i].Value
		table.Cells[i].SavedExpression = table.Cells[i].Expression
	}
}

func (table *Table) revertCellChanges() {
	for i := range table.Cells {
		table.Cells[i].Value = table.Cells[i].SavedValue
		table.Cells[i].Expression = table.Cells[i].SavedExpression
	}
}

type visit struct {
	colum, row int
}

type visitSet map[visit]struct{}

func (set visitSet) check(v visit) bool {
	_, visited := set[v]
	if visited {
		return true
	}
	set[v] = struct{}{}
	return false
}

func (cell *Cell) evaluate(table *Table, visited visitSet) error {
	v := visit{
		colum: cell.Column,
		row:   cell.Row,
	}
	_, alreadyVisited := visited[v]
	if alreadyVisited {
		return fmt.Errorf("recursive reference to %s%d", columnLabel(cell.Column), cell.Row)
	}
	visited[v] = struct{}{}
	if cell.Expression == nil {
		cell.Value = constant.MakeInt64(0)
		return nil
	}
	result, err := expression.Evaluate(newScope(table, cell), cell.Expression)
	if err != nil {
		return err
	}
	cell.Value = result
	return nil
}

type Scope struct {
	Table   *Table
	cell    *Cell
	visited visitSet
}

func newScope(table *Table, cell *Cell) *Scope {
	return &Scope{
		Table:   table,
		cell:    cell,
		visited: make(visitSet),
	}
}

const (
	RowIdent       = "ROW"
	ColumnIdent    = "COLUMN"
	MaxRowIdent    = "MAX_ROW"
	MaxColumnIdent = "MAX_COLUMN"
	MinRowIdent    = "MIN_ROW"
	MinColumnIdent = "MIN_COLUMN"
)

func (s *Scope) Resolve(ident string) (constant.Value, error) {
	switch ident {
	case "true":
		return constant.MakeBool(true), nil
	case "false":
		return constant.MakeBool(false), nil
	case RowIdent:
		return constant.MakeInt64(int64(s.cell.Row)), nil
	case ColumnIdent:
		return constant.MakeInt64(int64(s.cell.Column)), nil
	case MaxRowIdent:
		return constant.MakeInt64(int64(s.Table.RowCount - 1)), nil
	case MaxColumnIdent:
		return constant.MakeInt64(int64(s.Table.ColumnCount - 1)), nil
	case MinRowIdent, MinColumnIdent:
		return constant.MakeInt64(0), nil
	default:
		ident = strings.ToUpper(ident)
		if !identifierPattern.MatchString(ident) {
			return nil, fmt.Errorf("unknown variable %s", ident)
		}
		column, row, err := cellCoordinates(ident)
		if err != nil {
			return nil, err
		}
		if s.visited.check(visit{row: row, colum: column}) {
			return nil, fmt.Errorf("recursive reference to %s%d", columnLabel(column), row)
		}
		cell := s.Table.Cell(column, row)
		if cell.Expression == nil {
			return constant.MakeInt64(0), nil
		}
		return expression.Evaluate(&Scope{
			cell:    cell,
			Table:   s.Table,
			visited: s.visited,
		}, cell.Expression)
	}
}

var identifierPattern = regexp.MustCompile("(?P<column>[A-Z]+)(?P<row>[0-9]+)")

func cellCoordinates(in string) (int, int, error) {
	in = strings.TrimPrefix(in, "cell-")
	if !identifierPattern.MatchString(in) {
		return 0, 0, fmt.Errorf("unexpected identifier pattern expected something like A4")
	}
	parts := identifierPattern.FindStringSubmatch(in)
	columnName := parts[identifierPattern.SubexpIndex("column")]
	row, err := strconv.Atoi(parts[identifierPattern.SubexpIndex("row")])
	if err != nil {
		return 0, 0, fmt.Errorf("failed to Parse row number: %w", err)
	}
	//if row > maxRow {
	//	return 0, 0, fmt.Errorf("row number %d out of range it must be greater than 0 and less than or equal to %d", row, maxRow)
	//}
	column := columnNumber(columnName)
	//if column > maxColumn {
	//	return 0, 0, fmt.Errorf("column %s out of range it must be greater than or equal to %s and less than or equal to %s", columnName, columnLabel(0), columnLabel(maxColumn))
	//}
	return column, row, nil
}

func columnNumber(label string) int {
	result := 0
	for _, char := range label {
		result = result*26 + int(char) - 64
	}
	return result - 1
}
