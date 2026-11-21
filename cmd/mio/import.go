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

const importEfundUsage = `用法：
  mio import efund --preview <file.xlsx|file.zip> [--map <path>] [--json] [--db <path>]
  mio import efund --emit-map [--map <path>] [--force] [--db <path>] <file.xlsx|file.zip>

--preview   只解析并预览，不写数据库
--emit-map  生成/合并映射骨架；省略 --map 时写到数据库同目录 efund-map.json
--map       --preview 读取映射；--emit-map 写入目标
--force     仅 --emit-map：整体覆盖已有映射文件
--json      仅 --preview：输出机器可读 JSON
--db        SQLite 路径，仅用于默认映射路径和只读快照对比

zip 密码优先读 MIO_EFUND_PASSWORD；否则在终端中无回显提示输入。`

func runImport(args []string, stdout io.Writer) error {
	if len(args) == 0 || args[0] != "efund" {
		return errors.New(importEfundUsage)
	}
	fs := flag.NewFlagSet("import efund", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	dbPath := fs.String("db", store.DefaultPath(), "SQLite database path")
	mapPath := fs.String("map", "", "mapping JSON (default: efund-map.json next to the database)")
	previewOnly := fs.Bool("preview", false, "parse and preview only")
	emitMap := fs.Bool("emit-map", false, "write a mapping skeleton")
	force := fs.Bool("force", false, "overwrite an existing mapping file")
	asJSON := fs.Bool("json", false, "print machine-readable JSON")
	fs.Usage = func() { fmt.Fprintln(stdout, importEfundUsage) }
	positional := []string{}
	rest := args[1:]
	for len(rest) > 0 {
		if err := fs.Parse(rest); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return nil
			}
			return err
		}
		rest = fs.Args()
		if len(rest) == 0 {
			break
		}
		positional = append(positional, rest[0])
		rest = rest[1:]
	}
	if *previewOnly == *emitMap {
		return errors.New("请择一使用 --preview 或 --emit-map\n\n" + importEfundUsage)
	}
	if len(positional) != 1 {
		return errors.New(importEfundUsage)
	}
	if *force && !*emitMap {
		return errors.New("--force 只能与 --emit-map 一起使用")
	}
	if *asJSON && !*previewOnly {
		return errors.New("--json 只能与 --preview 一起使用")
	}
	file := positional[0]

	mappingFile := *mapPath
	if mappingFile == "" {
		mappingFile = efund.DefaultMappingPath(*dbPath)
	}

	parsed, err := efund.ParseWithPassword(file, efund.PasswordFromEnvOrTerminal)
	if err != nil {
		return err
	}

	if *emitMap {
		generated := efund.Skeleton(parsed)
		if *force {
			if err := efund.WriteMapping(mappingFile, generated); err != nil {
				return err
			}
			fmt.Fprintf(stdout, "已覆盖 %s：%d 条\n", mappingFile, len(generated.Mappings))
			return nil
		}
		existing := &efund.Mapping{}
		loaded, err := efund.LoadMapping(mappingFile)
		switch {
		case err == nil:
			existing = loaded
		case errors.Is(err, os.ErrNotExist):
		default:
			return err
		}
		merged, added := efund.MergeMapping(existing, generated)
		if err := efund.WriteMapping(mappingFile, &merged); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "已写入 %s：新增 %d 条，保留 %d 条已有条目\n",
			mappingFile, added, len(merged.Mappings)-added)
		return nil
	}

	mapping := &efund.Mapping{}
	loaded, err := efund.LoadMapping(mappingFile)
	switch {
	case err == nil:
		mapping = loaded
	case *mapPath == "" && errors.Is(err, os.ErrNotExist):
	default:
		return err
	}
	st, err := store.OpenReadOnly(*dbPath)
	if err != nil {
		return fmt.Errorf("open database read-only: %w", err)
	}
	var repo efund.ReadRepo
	if st != nil {
		defer st.Close()
		if err := st.CheckReadOnlySchema(context.Background()); err != nil {
			return err
		}
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
