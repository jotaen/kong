package kong

import (
	"bytes"
	"fmt"
	"go/doc"
	"io"
	"strings"
)

const (
	defaultIndent        = 2
	defaultColumnPadding = 4
)

// HelpPrinterOptions for HelpPrinters.
type HelpPrinterOptions struct {
	// Write a one-line summary of the context.
	Summary bool

	// Write help in a more compact form, but still fully-specified.
	Compact bool
}

// HelpPrinter is used to print context-sensitive help.
type HelpPrinter func(options HelpPrinterOptions, ctx *Context) error

// DefaultHelpPrinter is the default HelpPrinter.
func DefaultHelpPrinter(options HelpPrinterOptions, ctx *Context) error {
	if ctx.Empty() {
		options.Summary = false
	}
	w := newHelpWriter(ctx, options)
	selected := ctx.Selected()
	if selected == nil {
		printApp(w, ctx.App.Model)
	} else {
		printCommand(w, ctx.App.Model, selected)
	}
	return w.Write(ctx.App.Stdout)
}

func printApp(w *helpWriter, app *Application) {
	w.Printf("Usage:  %s", app.Summary())
	printNodeDetail(w, &app.Node)
	cmds := app.Leaves()
	if len(cmds) > 0 {
		w.Print("")
		if w.Summary {
			w.Printf(`Run "%s --help" for more information.`, app.Name)
		} else {
			w.Printf(`Run "%s <command> --help" for more information on a command.`, app.Name)
		}
	}
}

func printCommand(w *helpWriter, app *Application, cmd *Command) {
	w.Printf("Usage:  %s %s", app.Name, cmd.Summary())
	printNodeDetail(w, cmd)
	if w.Summary {
		w.Print("")
		w.Printf(`Run "%s %s --help" for more information.`, app.Name, cmd.Path())
	}
}

func printNodeDetail(w *helpWriter, node *Node) {
	if node.Help != "" {
		w.Print("")
		w.Wrap(node.Help)
	}
	if w.Summary {
		return
	}
	if len(node.Positional) > 0 {
		w.Print("")
		w.Print("Arguments:")
		writePositionals(w.Indent(), node.Positional)
	}
	if flags := node.AllFlags(); len(flags) > 0 {
		w.Print("")
		w.Print("Flags:")
		writeFlags(w.Indent(), flags)
	}
	cmds := node.Leaves()
	if len(cmds) > 0 {
		w.Print("")
		w.Print("Commands:")
		iw := w.Indent()
		if w.Compact {
			rows := [][2]string{}
			for _, cmd := range cmds {
				rows = append(rows, [2]string{cmd.Path(), cmd.Help})
			}
			writeTwoColumns(iw, defaultColumnPadding, rows)
		} else {
			for i, cmd := range cmds {
				printCommandSummary(iw, cmd)
				if i != len(cmds)-1 {
					iw.Print("")
				}
			}
		}
	}
}

func printCommandSummary(w *helpWriter, cmd *Command) {
	w.Print(cmd.Summary())
	if cmd.Help != "" {
		w.Indent().Wrap(cmd.Help)
	}
}

type helpWriter struct {
	indent string
	width  int
	lines  *[]string
	HelpPrinterOptions
}

func newHelpWriter(ctx *Context, options HelpPrinterOptions) *helpWriter {
	lines := []string{}
	w := &helpWriter{
		indent:             "",
		width:              guessWidth(ctx.App.Stdout),
		lines:              &lines,
		HelpPrinterOptions: options,
	}
	return w
}

func (h *helpWriter) Printf(format string, args ...interface{}) {
	h.Print(fmt.Sprintf(format, args...))
}

func (h *helpWriter) Print(text string) {
	*h.lines = append(*h.lines, strings.TrimRight(h.indent+text, " "))
}

func (h *helpWriter) Indent() *helpWriter {
	return &helpWriter{indent: h.indent + "  ", lines: h.lines, width: h.width - 2, HelpPrinterOptions: h.HelpPrinterOptions}
}

func (h *helpWriter) String() string {
	return strings.Join(*h.lines, "\n")
}

func (h *helpWriter) Write(w io.Writer) error {
	for _, line := range *h.lines {
		_, err := io.WriteString(w, line+"\n")
		if err != nil {
			return err
		}
	}
	return nil
}

func (h *helpWriter) Wrap(text string) {
	w := bytes.NewBuffer(nil)
	doc.ToText(w, strings.TrimSpace(text), "", "    ", h.width)
	for _, line := range strings.Split(strings.TrimSpace(w.String()), "\n") {
		h.Print(line)
	}
}

func writePositionals(w *helpWriter, args []*Positional) {
	rows := [][2]string{}
	for _, arg := range args {
		rows = append(rows, [2]string{arg.Summary(), arg.Help})
	}
	writeTwoColumns(w, defaultColumnPadding, rows)
}

func writeFlags(w *helpWriter, groups [][]*Flag) {
	rows := [][2]string{}
	haveShort := false
	for _, group := range groups {
		for _, flag := range group {
			if flag.Short != 0 {
				haveShort = true
				break
			}
		}
	}
	for i, group := range groups {
		if i > 0 {
			rows = append(rows, [2]string{"", ""})
		}
		for _, flag := range group {
			if !flag.Hidden {
				rows = append(rows, [2]string{formatFlag(haveShort, flag), flag.Help})
			}
		}
	}
	writeTwoColumns(w, defaultColumnPadding, rows)
}

func writeTwoColumns(w *helpWriter, padding int, rows [][2]string) {
	// Find size of first column.
	leftSize := 0
	for _, row := range rows {
		if c := len(row[0]); c > leftSize && c < 30 {
			leftSize = c
		}
	}

	offsetStr := strings.Repeat(" ", leftSize+padding)

	for _, row := range rows {
		buf := bytes.NewBuffer(nil)
		doc.ToText(buf, row[1], "", strings.Repeat(" ", defaultIndent), w.width-leftSize-padding)
		lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")

		line := fmt.Sprintf("%-*s", leftSize, row[0])
		if len(row[0]) < 30 {
			line += fmt.Sprintf("%*s%s", padding, "", lines[0])
			lines = lines[1:]
		}
		w.Print(line)
		for _, line := range lines {
			w.Printf("%s%s", offsetStr, line)
		}
	}
}

// haveShort will be true if there are short flags present at all in the help. Useful for column alignment.
func formatFlag(haveShort bool, flag *Flag) string {
	flagString := ""
	name := flag.Name
	isBool := flag.IsBool()
	if flag.Short != 0 {
		flagString += fmt.Sprintf("-%c, --%s", flag.Short, name)
	} else {
		if haveShort {
			flagString += fmt.Sprintf("    --%s", name)
		} else {
			flagString += fmt.Sprintf("--%s", name)
		}
	}
	if !isBool {
		flagString += fmt.Sprintf("=%s", flag.FormatPlaceHolder())
	}
	return flagString
}
