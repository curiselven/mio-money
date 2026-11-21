package efund

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	yzip "github.com/yeka/zip"
)

func TestParseEncryptedZip(t *testing.T) {
	xlsx, err := os.ReadFile(sampleBook(t))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	archive := writeEncryptedZip(t, xlsx, "fictional-secret")

	parsed, err := ParseWithPassword(archive, func() (string, error) {
		return "fictional-secret", nil
	})
	if err != nil {
		t.Fatalf("parse encrypted zip: %v", err)
	}
	if len(parsed.Holdings) != 3 {
		t.Fatalf("holdings = %+v", parsed.Holdings)
	}

	_, err = ParseWithPassword(archive, func() (string, error) {
		return "wrong-secret", nil
	})
	if err == nil || !strings.Contains(err.Error(), "密码错误") {
		t.Fatalf("wrong password error = %v", err)
	}
	if strings.Contains(err.Error(), "wrong-secret") {
		t.Fatalf("password leaked in error: %v", err)
	}
}

func TestParseEncryptedZipPasswordFromEnv(t *testing.T) {
	xlsx, err := os.ReadFile(sampleBook(t))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	archive := writeEncryptedZip(t, xlsx, "env-secret")
	t.Setenv("MIO_EFUND_PASSWORD", "env-secret")

	parsed, err := ParseWithPassword(archive, PasswordFromEnvOrTerminal)
	if err != nil {
		t.Fatalf("parse with env password: %v", err)
	}
	if len(parsed.Holdings) != 3 {
		t.Fatalf("holdings = %+v", parsed.Holdings)
	}
}

func TestParsePlainZipDoesNotRequestPassword(t *testing.T) {
	xlsx, err := os.ReadFile(sampleBook(t))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	archive := writePlainZip(t, xlsx, "holdings.xlsx")
	parsed, err := ParseWithPassword(archive, func() (string, error) {
		t.Fatal("password provider called for a plain zip")
		return "", nil
	})
	if err != nil {
		t.Fatalf("parse plain zip: %v", err)
	}
	if len(parsed.Holdings) != 3 {
		t.Fatalf("holdings = %+v", parsed.Holdings)
	}
}

func TestParseBytes(t *testing.T) {
	data, err := os.ReadFile(sampleBook(t))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	parsed, err := ParseBytes("memory.xlsx", data, nil)
	if err != nil {
		t.Fatalf("parse bytes: %v", err)
	}
	if len(parsed.Holdings) != 3 || parsed.File != "memory.xlsx" {
		t.Fatalf("parsed = %+v", parsed)
	}
}

func TestParseZipWithMultipleXLSX(t *testing.T) {
	xlsx, err := os.ReadFile(sampleBook(t))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	file := filepath.Join(t.TempDir(), "two.xlsx.zip")
	out, err := os.Create(file)
	if err != nil {
		t.Fatalf("create zip: %v", err)
	}
	zw := zip.NewWriter(out)
	for _, name := range []string{"a.xlsx", "b.xlsx"} {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("create entry: %v", err)
		}
		if _, err := w.Write(xlsx); err != nil {
			t.Fatalf("write entry: %v", err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	if err := out.Close(); err != nil {
		t.Fatalf("close file: %v", err)
	}
	if _, err := Parse(file); err == nil || !strings.Contains(err.Error(), "恰好一个") {
		t.Fatalf("multiple xlsx error = %v", err)
	}
}

func writeEncryptedZip(t *testing.T, data []byte, password string) string {
	t.Helper()
	file := filepath.Join(t.TempDir(), "encrypted.zip")
	out, err := os.Create(file)
	if err != nil {
		t.Fatalf("create zip: %v", err)
	}
	zw := yzip.NewWriter(out)
	w, err := zw.Encrypt("holdings.xlsx", password, yzip.AES256Encryption)
	if err != nil {
		t.Fatalf("encrypt entry: %v", err)
	}
	if _, err := w.Write(data); err != nil {
		t.Fatalf("write encrypted entry: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close encrypted zip: %v", err)
	}
	if err := out.Close(); err != nil {
		t.Fatalf("close encrypted file: %v", err)
	}
	return file
}

func writePlainZip(t *testing.T, data []byte, name string) string {
	t.Helper()
	file := filepath.Join(t.TempDir(), "plain.zip")
	out, err := os.Create(file)
	if err != nil {
		t.Fatalf("create zip: %v", err)
	}
	zw := zip.NewWriter(out)
	w, err := zw.Create(name)
	if err != nil {
		t.Fatalf("create entry: %v", err)
	}
	if _, err := w.Write(data); err != nil {
		t.Fatalf("write entry: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	if err := out.Close(); err != nil {
		t.Fatalf("close file: %v", err)
	}
	return file
}
