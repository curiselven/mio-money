package efund

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	yzip "github.com/yeka/zip"
	"golang.org/x/term"
)

type PasswordProvider func() (string, error)

const (
	maxZipEntries = 1000
	maxXLSXBytes  = 20 << 20
	// MaxInputBytes caps the xlsx or outer zip before it is read into memory.
	MaxInputBytes = 32 << 20
)

func CheckInputSize(size int64) error {
	if size > MaxInputBytes {
		return fmt.Errorf("文件过大：%d MB，上限 %d MB", (size+(1<<20)-1)>>20, MaxInputBytes>>20)
	}
	return nil
}

// Parse reads an xlsx, or an outer zip containing exactly one xlsx.
func Parse(file string) (*Result, error) {
	return ParseWithPassword(file, nil)
}

func ParseWithPassword(file string, password PasswordProvider) (*Result, error) {
	info, err := os.Stat(file)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}
	if err := CheckInputSize(info.Size()); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}
	return ParseBytes(file, data, password)
}

func ParseBytes(file string, data []byte, password PasswordProvider) (*Result, error) {
	if err := CheckInputSize(int64(len(data))); err != nil {
		return nil, err
	}
	if isOuterArchive(file, data) {
		xlsx, err := extractXLSX(data, password)
		if err != nil {
			return nil, err
		}
		data = xlsx
	}
	return parseXLSX(file, data)
}

// PasswordFromEnvOrTerminal reads MIO_EFUND_PASSWORD before prompting.
func PasswordFromEnvOrTerminal() (string, error) {
	if password := os.Getenv("MIO_EFUND_PASSWORD"); password != "" {
		return password, nil
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return "", errors.New("加密 zip 需要密码：请设置环境变量 MIO_EFUND_PASSWORD，或在终端中运行")
	}
	fmt.Fprint(os.Stderr, "基金E账户导出 zip 密码: ")
	password, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", fmt.Errorf("读取 zip 密码失败: %w", err)
	}
	return string(password), nil
}

func isOuterArchive(file string, data []byte) bool {
	ext := strings.ToLower(filepath.Ext(file))
	if ext == ".xlsx" || ext == ".xlsm" {
		return false
	}
	if ext == ".zip" {
		return true
	}
	if !hasZipMagic(data) {
		return false
	}
	return !isXLSXArchive(data)
}

func hasZipMagic(data []byte) bool {
	if len(data) < 4 || data[0] != 'P' || data[1] != 'K' {
		return false
	}
	switch {
	case data[2] == 3 && data[3] == 4:
		return true
	case data[2] == 5 && data[3] == 6:
		return true
	case data[2] == 7 && data[3] == 8:
		return true
	}
	return false
}

func isXLSXArchive(data []byte) bool {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return false
	}
	for _, file := range zr.File {
		if file.Name == "xl/workbook.xml" {
			return true
		}
	}
	return false
}

func extractXLSX(data []byte, password PasswordProvider) ([]byte, error) {
	zr, err := yzip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("打开 zip 失败: %w", err)
	}
	if len(zr.File) > maxZipEntries {
		return nil, fmt.Errorf("zip 条目过多：%d（上限 %d）", len(zr.File), maxZipEntries)
	}
	var matches []*yzip.File
	for _, file := range zr.File {
		lowerName := strings.ToLower(file.Name)
		if strings.HasSuffix(file.Name, "/") ||
			strings.HasPrefix(lowerName, "__macosx/") ||
			strings.HasPrefix(filepath.Base(file.Name), "._") ||
			!strings.EqualFold(filepath.Ext(file.Name), ".xlsx") {
			continue
		}
		matches = append(matches, file)
	}
	if len(matches) == 0 {
		return nil, errors.New("zip 内未找到 .xlsx")
	}
	if len(matches) > 1 {
		return nil, fmt.Errorf("zip 内需要恰好一个 .xlsx，实际找到 %d 个", len(matches))
	}
	file := matches[0]
	if file.IsEncrypted() {
		if password == nil {
			return nil, errors.New("zip 已加密，但未提供密码：请设置环境变量 MIO_EFUND_PASSWORD")
		}
		secret, err := password()
		if err != nil {
			return nil, err
		}
		if secret == "" {
			return nil, errors.New("zip 密码为空")
		}
		file.SetPassword(secret)
	}
	rc, err := file.Open()
	if err != nil {
		return nil, zipReadError(err)
	}
	defer rc.Close()
	out, err := io.ReadAll(io.LimitReader(rc, maxXLSXBytes+1))
	if err != nil {
		return nil, zipReadError(err)
	}
	if len(out) > maxXLSXBytes {
		return nil, fmt.Errorf("zip 内 xlsx 解压后超过 %d MB", maxXLSXBytes>>20)
	}
	return out, nil
}

func zipReadError(err error) error {
	if errors.Is(err, yzip.ErrPassword) || errors.Is(err, yzip.ErrDecryption) ||
		errors.Is(err, yzip.ErrAuthentication) {
		return errors.New("加密 zip 密码错误（或文件已损坏）")
	}
	return fmt.Errorf("读取 zip 内的 xlsx 失败: %w", err)
}
