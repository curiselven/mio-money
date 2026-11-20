package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/curiselven/mio-money/internal/importer/efund"
	"github.com/curiselven/mio-money/internal/store"
)

func runImport(args []string, stdout io.Writer) error {
	if len(args) == 0 || args[0] != "efund" {
		return errors.New("usage: mio import efund --preview <file.xlsx> [--map <file>] [--json]")
	}
	fs := flag.NewFlagSet("import efund", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	dbPath := fs.String("db", store.DefaultPath(), "SQLite database path")
	mapPath := fs.String("map", "", "mapping JSON (default: efund-map.json next to the database)")
	previewOnly := fs.Bool("preview", false, "parse and preview only")
	asJSON := fs.Bool("json", false, "print machine-readable JSON")
	positional := []string{}
	rest := args[1:]
	for len(rest) > 0 {
		if err := fs.Parse(rest); err != nil {
			return err
		}
		rest = fs.Args()
		if len(rest) == 0 {
			break
		}
		positional = append(positional, rest[0])
		rest = rest[1:]
	}
	if !*previewOnly {
		return errors.New("only --preview is supported; writing is not implemented yet")
	}
	if len(positional) != 1 {
		return errors.New("usage: mio import efund --preview <file.xlsx> [--map <file>] [--json]")
	}
	file := positional[0]

	mapping := &efund.Mapping{}
	mappingFile := *mapPath
	optionalMapping := mappingFile == ""
	if optionalMapping {
		mappingFile = efund.DefaultMappingPath(*dbPath)
	}
	loaded, err := efund.LoadMapping(mappingFile)
	switch {
	case err == nil:
		mapping = loaded
	case optionalMapping && errors.Is(err, os.ErrNotExist):
	default:
		return err
	}

	parsed, err := efund.Parse(file)
	if err != nil {
		return err
	}
	st, err := store.OpenReadOnly(*dbPath)
	if err != nil {
		return fmt.Errorf("open database read-only: %w", err)
	}
	var repo efund.ReadRepo
	if st != nil {
		defer st.Close()
		repo = st
	}
	preview, err := efund.Build(context.Background(), repo, parsed, mapping)
	if err != nil {
		return err
	}
	if *asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(preview)
	}
	efund.Render(stdout, preview)
	return nil
}
