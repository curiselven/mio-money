// Command stdio-demo drives the Mio MCP server over stdin/stdout and prints
// every JSON-RPC message in both directions. All data is fake.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type envelope struct {
	ID     json.RawMessage `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Message string `json:"message"`
	} `json:"error"`
}

type toolResult struct {
	StructuredContent json.RawMessage `json:"structuredContent"`
	IsError           bool            `json:"isError"`
	Content           []struct {
		Text string `json:"text"`
	} `json:"content"`
}

type session struct {
	tmp  string
	cmd  *exec.Cmd
	in   io.WriteCloser
	out  *bufio.Reader
	next int
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "demo failed:", err)
		os.Exit(1)
	}
}

func run() error {
	root, err := repoRoot()
	if err != nil {
		return err
	}
	s, err := start(root)
	if err != nil {
		return err
	}
	defer s.close()

	if err := s.call("initialize", map[string]any{
		"protocolVersion": "2025-06-18",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "mio-stdio-demo", "version": "0"},
	}, nil); err != nil {
		return err
	}
	s.notify("notifications/initialized", nil)

	var tools struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	if err := s.call("tools/list", map[string]any{}, &tools); err != nil {
		return err
	}
	fmt.Printf("服务端注册了 %d 个工具\n\n", len(tools.Tools))

	var account struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := s.tool("upsert_account", map[string]any{
		"name": "示例券商", "platform": "broker", "type": "stock", "owner": "我",
	}, &account); err != nil {
		return err
	}

	var submitted struct {
		SnapshotID string `json:"snapshot_id"`
		Status     string `json:"status"`
	}
	if err := s.tool("submit_snapshot", map[string]any{
		"account_id": account.ID,
		"as_of":      "2026-11-01",
		"source":     "mcp",
		"cash_cents": 123_456,
		"items": []map[string]any{
			{"name": "示例ETF", "code": "510300", "quantity": "100", "market_value_cents": 250_000},
			{"name": "示例基金", "code": "000001", "quantity": "1234.56", "market_value_cents": 98_765},
		},
	}, &submitted); err != nil {
		return err
	}

	if err := s.tool("confirm_snapshot", map[string]any{"snapshot_id": submitted.SnapshotID}, nil); err != nil {
		return err
	}
	if err := s.tool("get_net_worth", map[string]any{"as_of": "2026-11-01"}, nil); err != nil {
		return err
	}
	return nil
}

func start(root string) (*session, error) {
	tmp, err := os.MkdirTemp("", "mio-stdio-demo-")
	if err != nil {
		return nil, err
	}
	bin := filepath.Join(tmp, "mio")
	build := exec.Command("go", "build", "-o", bin, "./cmd/mio")
	build.Dir = root
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		os.RemoveAll(tmp)
		return nil, fmt.Errorf("go build: %w", err)
	}
	db := filepath.Join(tmp, "mio.db")
	fmt.Printf("临时库：%s\n\n", db)

	cmd := exec.Command(bin, "--db", db)
	cmd.Stderr = os.Stderr
	in, err := cmd.StdinPipe()
	if err != nil {
		os.RemoveAll(tmp)
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		os.RemoveAll(tmp)
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		os.RemoveAll(tmp)
		return nil, err
	}
	return &session{tmp: tmp, cmd: cmd, in: in, out: bufio.NewReader(out)}, nil
}

func (s *session) close() {
	s.in.Close()
	s.cmd.Wait()
	os.RemoveAll(s.tmp)
}

func (s *session) call(method string, params any, out any) error {
	return s.callAs(method, method, params, out)
}

func (s *session) callAs(method, label string, params any, out any) error {
	s.next++
	id := s.next
	msg := map[string]any{"jsonrpc": "2.0", "id": id, "method": method}
	if params != nil {
		msg["params"] = params
	}
	if err := s.send(label, msg); err != nil {
		return err
	}
	for {
		line, err := s.receive()
		if err != nil {
			return err
		}
		var env envelope
		if err := json.Unmarshal([]byte(line), &env); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
		if len(env.ID) == 0 {
			fmt.Println("（通知，无 id）")
			fmt.Println(pretty(line))
			continue
		}
		if string(env.ID) != strconv.Itoa(id) {
			continue
		}
		if env.Error != nil {
			return fmt.Errorf("%s: %s", method, env.Error.Message)
		}
		if out != nil {
			if err := json.Unmarshal(env.Result, out); err != nil {
				return fmt.Errorf("decode %s result: %w", method, err)
			}
		}
		return nil
	}
}

func (s *session) notify(method string, params any) {
	msg := map[string]any{"jsonrpc": "2.0", "method": method}
	if params != nil {
		msg["params"] = params
	}
	if err := s.send(method, msg); err != nil {
		fmt.Fprintln(os.Stderr, "send notification:", err)
	}
}

func (s *session) tool(name string, args map[string]any, out any) error {
	var res toolResult
	if err := s.callAs("tools/call", "tools/call "+name, map[string]any{"name": name, "arguments": args}, &res); err != nil {
		return err
	}
	if res.IsError {
		text := "tool error"
		if len(res.Content) > 0 {
			text = res.Content[0].Text
		}
		return fmt.Errorf("%s: %s", name, text)
	}
	if out != nil {
		if err := json.Unmarshal(res.StructuredContent, out); err != nil {
			return fmt.Errorf("decode %s output: %w", name, err)
		}
	}
	return nil
}

func (s *session) send(method string, msg any) error {
	raw, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	fmt.Printf("→ 发送 %s\n%s\n", method, pretty(string(raw)))
	if _, err := s.in.Write(append(raw, '\n')); err != nil {
		return err
	}
	return nil
}

func (s *session) receive() (string, error) {
	type result struct {
		line string
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		line, err := s.out.ReadString('\n')
		ch <- result{strings.TrimSpace(line), err}
	}()
	select {
	case r := <-ch:
		if r.line != "" {
			fmt.Printf("← 收到\n%s\n", pretty(r.line))
		}
		return r.line, r.err
	case <-time.After(15 * time.Second):
		return "", errors.New("等待 MCP 响应超时")
	}
}

func pretty(raw string) string {
	var buf bytes.Buffer
	if err := json.Indent(&buf, []byte(raw), "", "  "); err != nil {
		return raw
	}
	return buf.String()
}

func repoRoot() (string, error) {
	if _, file, _, ok := runtime.Caller(0); ok {
		root := filepath.Dir(filepath.Dir(filepath.Dir(file)))
		if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
			return root, nil
		}
	}
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err != nil {
		return "", fmt.Errorf("run this from the repository root")
	}
	return dir, nil
}
