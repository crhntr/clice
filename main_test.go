package main

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/crhntr/dom/domtest"
	"golang.org/x/net/html/atom"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServer(t *testing.T) {
	var templates = template.Must(template.New("index.html.template").Parse(indexHTMLTemplate))

	setup := func(columns, rows int) *server {
		return &server{
			table:     NewTable(columns, rows),
			templates: templates,
		}
	}

	t.Run("editing a cell", func(t *testing.T) {
		t.Run("unknown cell", func(t *testing.T) {
			s := setup(1, 1)

			req := httptest.NewRequest(http.MethodGet, "/cell/peach1", nil)
			rec := httptest.NewRecorder()
			s.routes().ServeHTTP(rec, req)
			res := rec.Result()

			assert.Equal(t, http.StatusBadRequest, res.StatusCode)
			// TODO: use assert.Equal(t, http.StatusNotFound, res.StatusCode)
		})
		t.Run("cell with expression", func(t *testing.T) {
			s := setup(1, 1)

			require.Equal(t, http.StatusOK, setCellExpressionRequest(t, s, "cell-A0", "100").Result().StatusCode)

			req := httptest.NewRequest(http.MethodGet, "/cell/A0", nil)
			rec := httptest.NewRecorder()
			s.routes().ServeHTTP(rec, req)
			res := rec.Result()
			assert.Equal(t, http.StatusOK, res.StatusCode)
			elements := domtest.DocumentFragment(t, res.Body, atom.Tr)
			require.Len(t, elements, 1)
			cell := elements[0]

			require.True(t, cell.Matches(`.cell`))

			if assert.NotNil(t, cell) {
				assert.Equal(t, "0", cell.GetAttribute("data-column-index"))
				assert.Equal(t, "0", cell.GetAttribute("data-row-index"))
				assert.Equal(t, "cell-A0", cell.GetAttribute("id"))
			}

			if input := cell.QuerySelector(`input[type="text"]`); assert.NotNil(t, input) {
				assert.Equal(t, "100", input.GetAttribute("value"))
				assert.True(t, input.HasAttribute("autofocus"))
				assert.NotZero(t, input.GetAttribute("aria-label"))
			}
		})
		t.Run("empty table no cells", func(t *testing.T) {
			s := setup(1, 1)

			req := httptest.NewRequest(http.MethodGet, "/cell/A0", nil)
			rec := httptest.NewRecorder()
			s.routes().ServeHTTP(rec, req)
			res := rec.Result()
			assert.Equal(t, http.StatusOK, res.StatusCode)
			elements := domtest.DocumentFragment(t, res.Body, atom.Tr)
			require.Len(t, elements, 1)
			cell := elements[0]

			require.True(t, cell.Matches(`.cell`))

			if assert.NotNil(t, cell) {
				assert.Equal(t, "0", cell.GetAttribute("data-column-index"))
				assert.Equal(t, "0", cell.GetAttribute("data-row-index"))
				assert.Equal(t, "cell-A0", cell.GetAttribute("id"))
			}

			if input := cell.QuerySelector(`input[type="text"]`); assert.NotNil(t, input) {
				assert.Equal(t, "", input.GetAttribute("value"))
				assert.True(t, input.HasAttribute("autofocus"))
				assert.NotZero(t, input.GetAttribute("aria-label"))
			}
		})
	})

	t.Run("setting a cell expression literal", func(t *testing.T) {
		t.Run("int", func(t *testing.T) {
			s := setup(1, 1)

			rec := setCellExpressionRequest(t, s, "cell-A0", "100")
			res := rec.Result()
			document := domtest.Response(t, res)
			assert.Equal(t, http.StatusOK, res.StatusCode)

			cellElement := document.QuerySelector("#cell-A0")
			require.NotNil(t, cellElement)
			require.Equal(t, cellElement.TextContent(), "100")
		})

		t.Run("float", func(t *testing.T) {
			s := setup(1, 1)

			rec := setCellExpressionRequest(t, s, "cell-A0", "0.5")
			res := rec.Result()
			document := domtest.Response(t, res)
			assert.Equal(t, http.StatusOK, res.StatusCode)

			if cellElement := document.QuerySelector("#cell-A0"); assert.NotNil(t, cellElement) {
				assert.Equal(t, cellElement.TextContent(), "0.5")
			}
		})

		t.Run("string", func(t *testing.T) {
			t.Skip("need to remove up-casing of string literals")
			s := setup(1, 1)

			rec := setCellExpressionRequest(t, s, "cell-A0", `"Hello, world!"`)
			res := rec.Result()
			document := domtest.Response(t, res)

			assert.Equal(t, http.StatusOK, res.StatusCode)

			if cellElement := document.QuerySelector("#cell-A0"); assert.NotNil(t, cellElement) {
				assert.Equal(t, cellElement.TextContent(), `"Hello, world!"`)
			}
		})

		t.Run("bool", func(t *testing.T) {
			t.Skip("need to add literal boolean support")
			s := setup(1, 1)

			rec := setCellExpressionRequest(t, s, "cell-A0", "true")
			res := rec.Result()
			document := domtest.Response(t, res)
			assert.Equal(t, http.StatusOK, res.StatusCode)

			if cellElement := document.QuerySelector("#cell-A0"); assert.NotNil(t, cellElement) {
				require.Equal(t, cellElement.TextContent(), "true")
			}
		})
	})

	t.Run("cell identifiers", func(t *testing.T) {
		t.Run("simple cell reference", func(t *testing.T) {
			s := setup(1, 2)
			{ // setup some cell to reference
				rec := setCellExpressionRequest(t, s, "cell-A0", "100")
				res := rec.Result()
				assert.Equal(t, http.StatusOK, res.StatusCode)
			}
			{ // reference the cell
				rec := setCellExpressionRequest(t, s, "cell-A1", "A0")
				res := rec.Result()
				assert.Equal(t, http.StatusOK, res.StatusCode)
				document := domtest.Response(t, res)

				if cellElement := document.QuerySelector("#cell-A0"); assert.NotNil(t, cellElement) {
					assert.NotNil(t, cellElement)
					assert.Equal(t, cellElement.TextContent(), `100`)
				}
				if cellElement := document.QuerySelector("#cell-A1"); assert.NotNil(t, cellElement) {
					require.NotNil(t, cellElement)
					require.Equal(t, cellElement.TextContent(), `100`)
				}
			}
		})
		t.Run("updating referencing cells", func(t *testing.T) {
			s := setup(1, 3)
			{ // setup some cell to reference
				rec := setCellExpressionRequest(t, s, "cell-A0", "100")
				res := rec.Result()
				assert.Equal(t, http.StatusOK, res.StatusCode)
			}
			{ // setup some referencing cell to reference
				rec := setCellExpressionRequest(t, s, "cell-A1", "A0")
				res := rec.Result()
				assert.Equal(t, http.StatusOK, res.StatusCode)
			}
			{ // setup some referencing cell to reference
				rec := setCellExpressionRequest(t, s, "cell-A2", "A1")
				res := rec.Result()
				assert.Equal(t, http.StatusOK, res.StatusCode)
			}
			{ // update the initial cell
				rec := setCellExpressionRequest(t, s, "cell-A0", "20")
				res := rec.Result()
				assert.Equal(t, http.StatusOK, res.StatusCode)
				document := domtest.Response(t, res)

				cells := document.QuerySelectorAll(`.cell`)
				assert.Equal(t, 3, cells.Length())

				for i := 0; i < cells.Length(); i++ {
					cell := cells.Item(i)
					if assert.NotNil(t, cell) {
						assert.Equal(t, "20", cell.TextContent())
					}
				}
			}
		})
	})
}

func setCellExpressionRequest(t *testing.T, s *server, cell string, value string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPatch, "/table", strings.NewReader(url.Values{
		cell: []string{value},
	}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	s.routes().ServeHTTP(rec, req)
	return rec
}
