//go:build js && wasm

package clice

import (
	"context"
	"fmt"
	"strconv"
	"syscall/js"

	"github.com/crhntr/window"
	"github.com/crhntr/window/browser"
)

func Attach(ss *SpreadSheet, query string) {
	queryEl := window.Document.QuerySelector(query).(browser.Element)
	ss.uiElQuery = query

	window.AddEventListener("click", browser.NewEventListenerFunc(func(event browser.Event) {
		el, ok := event.Target().(browser.Element)
		if !ok || el.Closest(query) == nil {
			return
		}
		switch el.Attribute("class") {
		case "cell":
			inputs := queryEl.QuerySelectorAll(".cell-input")
			for i := 0; i < inputs.Length(); i++ {
				input := inputs.Item(i).(browser.Element)
				cellEl := input.Closest(".cell")
				row, _ := strconv.Atoi(cellEl.Attribute("data-row"))
				col, _ := strconv.Atoi(cellEl.Attribute("data-col"))
				cell, found := ss.FindCell(context.Background(), col, row)
				if !found {
					cellEl.ReplaceChildren()
					return
				}
				cellEl.SetTextContent(fmt.Sprintf("%v", cell.Value().Interface()))
			}
			input := browser.Input(window.Document.CreateElement("input").(browser.Element))
			row, _ := strconv.Atoi(el.Attribute("data-row"))
			col, _ := strconv.Atoi(el.Attribute("data-col"))
			if cell, found := ss.FindCell(context.Background(), col, row); found {
				input.SetValue(cell.Expression)
			}
			input.SetAttribute("class", "cell-input")
			el.ReplaceChildren(input)
		}
	}))

	window.AddEventListener("keyup", browser.NewEventListenerFunc(func(event browser.Event) {
		e := js.Value(event)

		switch e.Get("keyCode").Int() {
		case keyCodeEnter:
			target, ok := event.Target().(browser.Input)
			if !ok || target.Closest(query) == nil {
				return
			}
			cellEl := target.Closest(".cell")
			col, _ := strconv.Atoi(cellEl.Attribute("data-col"))
			row, _ := strconv.Atoi(cellEl.Attribute("data-row"))
			err := ss.UpdateCell(context.TODO(), col, row, target.Value())
			if err != nil {
				js.Global().Call("alert", err.Error())
				return
			}
			cellEl.ReplaceChildren()
			cell, _ := ss.FindCell(context.TODO(), col, row)
			if cell.view == nil {
				cell.view = queryEl.QuerySelector(fmt.Sprintf(`.cell[data-col="%d"][data-row="%d"]`, cell.Column, cell.Row))
			}
			cell.view.SetTextContent(fmt.Sprintf("%v", cell.Value().Interface()))
		}
	}))

	table := window.Document.CreateElement("table")
	tbody := window.Document.CreateElement("tbody")
	table.Append(tbody)
	style := window.Document.CreateElement("style")
	style.SetTextContent(`
td {
	min-width: 1rem;
	padding: .5rem;
}

.cell-input {

}

.cell {
	background: lightgrey;
}

.cell input {
	
}
.column-header {
	
}
.row-header {
	text-align: left;
}
`)
	table.Append(style)

	columnHeaderRow := window.Document.CreateElement("tr")
	columnHeaderRow.Append(window.Document.CreateElement("td"))
	for col := 0; col < ss.Columns; col++ {
		td := window.Document.CreateElement("td")
		td.SetAttribute("class", "column-header")
		td.SetAttribute("data-col", strconv.Itoa(col))
		td.SetTextContent(strconv.Itoa(col))
		columnHeaderRow.Append(td)
	}
	tbody.Append(columnHeaderRow)
	for row := 0; row < ss.Rows; row++ {
		tr := window.Document.CreateElement("tr")
		rowHeader := window.Document.CreateElement("td")
		rowHeader.SetAttribute("class", "row-header")
		rowHeader.SetAttribute("data-row", strconv.Itoa(row))
		rowHeader.SetTextContent(strconv.Itoa(row))
		tr.Append(rowHeader)
		for col := 0; col < ss.Columns; col++ {
			td := window.Document.CreateElement("td")
			td.SetAttribute("data-row", strconv.Itoa(row))
			td.SetAttribute("data-col", strconv.Itoa(col))
			td.SetAttribute("class", "cell")
			td.SetAttribute("title", fmt.Sprintf("row: %d, column: %d", row, col))
			tr.Append(td)
		}
		tbody.Append(tr)
	}

	queryEl.Append(table)

	for _, cell := range ss.Cells {
		cell.view = queryEl.QuerySelector(fmt.Sprintf(`.cell[data-col="%d"][data-row="%d"]`, cell.Column, cell.Row))
		cell.view.SetTextContent(fmt.Sprintf("%v", cell.Value().Interface()))
	}
}

const (
	keyCodeBackspace = 8
	keyCodeTab       = 9
	keyCodeClear     = 12
	keyCodeShift     = 16
	keyCodeControl   = 17
	keyCodeEnter     = 13
	keyCodeEscape    = 27
	keyCodeSpace     = 32
	keyCodePageUp    = 33
	keyCodePageDown  = 34
	keyCodeEnd       = 35
	keyCodeHome      = 36
	keyCodeLeft      = 37
	keyCodeUp        = 38
	keyCodeRight     = 39
	keyCodeDown      = 40
	keyCodeDelete    = 46

	// function keys are 112-130
)
