//go:build js && wasm

package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/crhntr/window"

	"github.com/crhntr/clice"
)

func main() {
	spreadsheet := clice.New()

	ctx := context.Background()

	err := spreadsheet.UpdateCell(ctx, 0, 0, `"Hello, world!"`)
	if err != nil {
		fmt.Println("ERROR", err)
		return
	}

	err = spreadsheet.UpdateCell(ctx, 0, 1, "420")
	if err != nil {
		fmt.Println("ERROR", err)
		return
	}

	err = spreadsheet.UpdateCell(ctx, 0, 2, "int8(10)")
	if err != nil {
		fmt.Println("ERROR", err)
		return
	}

	err = spreadsheet.AddFunction("HasPrefix", strings.HasPrefix)
	err = spreadsheet.UpdateCell(ctx, 0, 3, `HasPrefix(value(0, 1), "Hello")`)
	if err != nil {
		fmt.Println("ERROR", err)
		return
	}

	div := window.Document.CreateElement("div")
	div.SetAttribute("id", "spreadsheet")
	window.Document.Body().Append(div)

	clice.Attach(spreadsheet, "#spreadsheet")

	select {}
}
