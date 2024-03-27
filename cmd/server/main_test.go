package main

import (
	"bytes"
	"html/template"
	"io"
	"math"
	"math/big"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/crhntr/dom/domtest"
	"golang.org/x/net/html/atom"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/crhntr/clice"
)

func TestServer(t *testing.T) {
	var templates = template.Must(template.New("index.html.template").Parse(indexHTMLTemplate))

	setup := func(columns, rows int) *server {
		return &server{
			table:     clice.NewTable(columns, rows),
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
		t.Run("row out of bounds", func(t *testing.T) {
			s := setup(1, 1)
			mux := s.routes()

			rec := setCellExpressionRequest(t, mux, "A0", "A1")
			res := rec.Result()

			assert.Equal(t, http.StatusBadRequest, res.StatusCode)
			assert.Contains(t, rec.Body.String(), "row index 1 out of bounds [0, 1)")
		})
		t.Run("column out of bounds", func(t *testing.T) {
			s := setup(1, 1)
			mux := s.routes()

			rec := setCellExpressionRequest(t, mux, "A0", "B0")
			res := rec.Result()

			assert.Equal(t, http.StatusBadRequest, res.StatusCode)
			assert.Contains(t, rec.Body.String(), "column index 1 out of bounds [0, 1)")
		})
		t.Run("cell with expression", func(t *testing.T) {
			s := setup(1, 1)

			mux := s.routes()
			require.Equal(t, http.StatusOK, setCellExpressionRequest(t, mux, "cell-A0", "100").Result().StatusCode)

			req := httptest.NewRequest(http.MethodGet, "/cell/A0", nil)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
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
			mux := s.routes()

			req := httptest.NewRequest(http.MethodGet, "/cell/A0", nil)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
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
			mux := s.routes()

			rec := setCellExpressionRequest(t, mux, "cell-A0", "100")
			res := rec.Result()
			document := domtest.Response(t, res)
			assert.Equal(t, http.StatusOK, res.StatusCode)

			cellElement := document.QuerySelector("#cell-A0")
			require.NotNil(t, cellElement)
			require.Equal(t, "100", cellElement.TextContent())
		})

		t.Run("float", func(t *testing.T) {
			s := setup(1, 1)
			mux := s.routes()

			rec := setCellExpressionRequest(t, mux, "cell-A0", "0.5")
			res := rec.Result()
			document := domtest.Response(t, res)
			assert.Equal(t, http.StatusOK, res.StatusCode)

			if cellElement := document.QuerySelector("#cell-A0"); assert.NotNil(t, cellElement) {
				assert.Equal(t, "0.5", cellElement.TextContent())
			}
		})

		t.Run("string", func(t *testing.T) {
			s := setup(1, 1)
			mux := s.routes()

			rec := setCellExpressionRequest(t, mux, "cell-A0", `"Hello, world!"`)
			res := rec.Result()
			document := domtest.Response(t, res)

			assert.Equal(t, http.StatusOK, res.StatusCode)

			if cellElement := document.QuerySelector("#cell-A0"); assert.NotNil(t, cellElement) {
				assert.Equal(t, `"Hello, world!"`, cellElement.TextContent())
			}
		})

		t.Run("bool", func(t *testing.T) {
			t.Run("true", func(t *testing.T) {
				s := setup(1, 1)
				mux := s.routes()

				{ // add a cell with a bool
					rec := setCellExpressionRequest(t, mux, "cell-A0", "true")
					res := rec.Result()
					assert.Equal(t, http.StatusOK, res.StatusCode)
				}

				{ // add a cell that references the cell with a bool
					rec := setCellExpressionRequest(t, mux, "cell-A1", "!A0")
					res := rec.Result()
					assert.Equal(t, http.StatusOK, res.StatusCode)
				}

				req := httptest.NewRequest(http.MethodGet, "/", nil)
				rec := httptest.NewRecorder()
				mux.ServeHTTP(rec, req)
				res := rec.Result()
				assert.Equal(t, http.StatusOK, res.StatusCode)
				document := domtest.Response(t, res)

				if cellElement := document.QuerySelector("#cell-A0"); assert.NotNil(t, cellElement) {
					require.Equal(t, "true", cellElement.TextContent())
				}
			})
			t.Run("false", func(t *testing.T) {
				s := setup(1, 1)
				mux := s.routes()

				rec := setCellExpressionRequest(t, mux, "cell-A0", "false")
				res := rec.Result()
				document := domtest.Response(t, res)
				assert.Equal(t, http.StatusOK, res.StatusCode)

				if cellElement := document.QuerySelector("#cell-A0"); assert.NotNil(t, cellElement) {
					require.Equal(t, "false", cellElement.TextContent())
				}
			})
		})
	})

	t.Run("cell identifiers", func(t *testing.T) {
		t.Run("simple cell reference", func(t *testing.T) {
			s := setup(1, 2)
			mux := s.routes()

			{ // setup some cell to reference
				rec := setCellExpressionRequest(t, mux, "cell-A0", "100")
				res := rec.Result()
				assert.Equal(t, http.StatusOK, res.StatusCode)
			}
			{ // reference the cell
				rec := setCellExpressionRequest(t, mux, "cell-A1", "A0")
				res := rec.Result()
				assert.Equal(t, http.StatusOK, res.StatusCode)
				document := domtest.Response(t, res)

				if cellElement := document.QuerySelector("#cell-A0"); assert.NotNil(t, cellElement) {
					assert.NotNil(t, cellElement)
					assert.Equal(t, `100`, cellElement.TextContent())
				}
				if cellElement := document.QuerySelector("#cell-A1"); assert.NotNil(t, cellElement) {
					require.NotNil(t, cellElement)
					require.Equal(t, `100`, cellElement.TextContent())
				}
			}
		})

		t.Run("self reference", func(t *testing.T) {
			s := setup(1, 1)
			mux := s.routes()

			{ // setup some cell to reference
				rec := setCellExpressionRequest(t, mux, "cell-A0", "A0")
				res := rec.Result()
				assert.Equal(t, http.StatusBadRequest, res.StatusCode)
				assert.Contains(t, rec.Body.String(), "recursive reference to A0")
			}
		})

		t.Run("loop reference", func(t *testing.T) {
			s := setup(1, 2)
			mux := s.routes()

			{ // setup some cell to reference
				rec := setCellExpressionRequest(t, mux, "cell-A0", "A1")
				res := rec.Result()
				assert.Equal(t, http.StatusOK, res.StatusCode)
			}

			{ // setup some cell to reference
				rec := setCellExpressionRequest(t, mux, "cell-A1", "A0")
				res := rec.Result()
				assert.Equal(t, http.StatusBadRequest, res.StatusCode)
				assert.Contains(t, rec.Body.String(), "recursive reference to A1")
			}
		})

		t.Run("parsing a huge cell value fails", func(t *testing.T) {
			s := setup(1, 2)
			mux := s.routes()
			{ // setup some cell to reference
				n := new(big.Int)
				n = n.SetInt64(math.MaxInt64)
				n = n.Add(n, big.NewInt(1))
				id := "cell-A" + n.String()
				t.Log(id)
				rec := setCellExpressionRequest(t, mux, id, "100")
				res := rec.Result()
				assert.Equal(t, http.StatusBadRequest, res.StatusCode)
				buf, _ := io.ReadAll(res.Body)
				assert.Contains(t, string(buf), "failed to Parse row number:")
			}
		})
		t.Run("parsing a negative cell value", func(t *testing.T) {
			s := setup(1, 2)
			mux := s.routes()
			{ // setup some cell to reference
				n := new(big.Int)
				n = n.SetInt64(math.MinInt64)
				n.Add(n, big.NewInt(1))
				rec := setCellExpressionRequest(t, mux, "cell-A-1", "100")
				res := rec.Result()
				assert.Equal(t, http.StatusBadRequest, res.StatusCode)
			}
		})
		t.Run("updating referencing cells", func(t *testing.T) {
			s := setup(1, 3)
			mux := s.routes()
			{ // setup some cell to reference
				rec := setCellExpressionRequest(t, mux, "cell-A0", "100")
				res := rec.Result()
				assert.Equal(t, http.StatusOK, res.StatusCode)
			}
			{ // setup some referencing cell to reference
				rec := setCellExpressionRequest(t, mux, "cell-A1", "A0")
				res := rec.Result()
				assert.Equal(t, http.StatusOK, res.StatusCode)
			}
			{ // setup some referencing cell to reference
				rec := setCellExpressionRequest(t, mux, "cell-A2", "A1")
				res := rec.Result()
				assert.Equal(t, http.StatusOK, res.StatusCode)
			}
			{ // update the initial cell
				rec := setCellExpressionRequest(t, mux, "cell-A0", "20")
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

	t.Run("upload", func(t *testing.T) {
		t.Run("example file", func(t *testing.T) {
			const tableJSON =
			/* language=json */ `{
  "rows": 2,
  "columns": 2,
  "cells": [
    {"id": "A0", "ex": "100"},
    {"id": "A1", "ex": "80"},
    {"id": "B0", "ex": "\"Average\""},
	{"id": "B1", "ex": "(A0 + A1) / (iota + 1)"}
  ]
}`
			t.Run("upload json", func(t *testing.T) {
				s := setup(2, 2)
				mux := s.routes()

				{ // upload table.json
					rec := uploadJSONTableRequest(t, mux, tableJSON)
					res := rec.Result()
					assert.Equal(t, http.StatusOK, res.StatusCode)

					if out, _ := io.ReadAll(res.Body); t.Failed() {
						t.Log(string(out))
					}
				}

				{ // asser the values are calculated
					req := httptest.NewRequest(http.MethodGet, "/", nil)
					rec := httptest.NewRecorder()
					mux.ServeHTTP(rec, req)
					res := rec.Result()
					assert.Equal(t, http.StatusOK, res.StatusCode)
					document := domtest.Response(t, res)
					if el := document.QuerySelector("#cell-A0"); assert.NotNil(t, el) {
						assert.Contains(t, el.TextContent(), "100")
					}
					if el := document.QuerySelector("#cell-A1"); assert.NotNil(t, el) {
						assert.Contains(t, el.TextContent(), "80")
					}
					if el := document.QuerySelector("#cell-B0"); assert.NotNil(t, el) {
						assert.Contains(t, el.TextContent(), `"Average"`)
					}
					if el := document.QuerySelector("#cell-B1"); assert.NotNil(t, el) {
						assert.Contains(t, el.TextContent(), "90")
					}
				}
			})
			t.Run("example file", func(t *testing.T) {
				s := setup(2, 2)
				mux := s.routes()

				for _, update := range []struct {
					ID         string
					Expression string
				}{
					{ID: "cell-A0", Expression: "100"},
					{ID: "cell-A1", Expression: "80"},
					{ID: "cell-B1", Expression: "(A0 + A1) / (iota + 1)"},
					{ID: "cell-B0", Expression: `"Average"`},
				} {
					rec := setCellExpressionRequest(t, mux, update.ID, update.Expression)
					res := rec.Result()
					assert.Equal(t, http.StatusOK, res.StatusCode)
				}

				req := httptest.NewRequest(http.MethodGet, "/table.json", nil)
				rec := httptest.NewRecorder()
				mux.ServeHTTP(rec, req)
				res := rec.Result()
				assert.Equal(t, http.StatusOK, res.StatusCode)
				defer closeAndIgnoreError(res.Body)
				body, _ := io.ReadAll(res.Body)
				assert.JSONEq(t, tableJSON, string(body))
			})
		})
	})
}

func setCellExpressionRequest(t *testing.T, mux http.Handler, cell string, value string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPatch, "/table", strings.NewReader(url.Values{
		"cell-" + cell: []string{value},
	}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func uploadJSONTableRequest(t *testing.T, mux http.Handler, tableJSON string) *httptest.ResponseRecorder {
	t.Helper()
	body := bytes.NewBuffer(nil)
	writer := multipart.NewWriter(body)
	w, err := writer.CreateFormFile("table.json", "table.json")
	require.NoError(t, err)
	_, _ = w.Write([]byte(tableJSON))
	require.NoError(t, writer.Close())
	req := httptest.NewRequest(http.MethodPost, "/table.json", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}
