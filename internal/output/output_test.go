package output

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteFields_ToFile(t *testing.T) {
	tmpDir := t.TempDir()
	outputFile := filepath.Join(tmpDir, "output.txt")

	fields := map[string]string{
		"key1": "value1",
		"key2": "value2",
		"key3": "value3",
	}

	err := WriteFields(outputFile, fields)
	if err != nil {
		t.Fatalf("WriteFields failed: %v", err)
	}

	content, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	expected := "key1=value1\nkey2=value2\nkey3=value3\n"
	if string(content) != expected {
		t.Errorf("File content mismatch:\ngot:\n%q\nwant:\n%q", string(content), expected)
	}
}

func TestWriteFields_AppendToFile(t *testing.T) {
	tmpDir := t.TempDir()
	outputFile := filepath.Join(tmpDir, "output.txt")

	// First write
	fields1 := map[string]string{
		"key1": "value1",
	}
	err := WriteFields(outputFile, fields1)
	if err != nil {
		t.Fatalf("First WriteFields failed: %v", err)
	}

	// Second write (append)
	fields2 := map[string]string{
		"key2": "value2",
	}
	err = WriteFields(outputFile, fields2)
	if err != nil {
		t.Fatalf("Second WriteFields failed: %v", err)
	}

	content, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	expected := "key1=value1\nkey2=value2\n"
	if string(content) != expected {
		t.Errorf("File content mismatch:\ngot:\n%q\nwant:\n%q", string(content), expected)
	}
}

func TestWriteFields_SortedOutput(t *testing.T) {
	tmpDir := t.TempDir()
	outputFile := filepath.Join(tmpDir, "output.txt")

	fields := map[string]string{
		"zebra": "z",
		"apple": "a",
		"mango": "m",
		"banana": "b",
	}

	err := WriteFields(outputFile, fields)
	if err != nil {
		t.Fatalf("WriteFields failed: %v", err)
	}

	content, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	expected := "apple=a\nbanana=b\nmango=m\nzebra=z\n"
	if string(content) != expected {
		t.Errorf("File content mismatch:\ngot:\n%q\nwant:\n%q", string(content), expected)
	}
}

func TestWriteFields_StdoutFallback(t *testing.T) {
	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	fields := map[string]string{
		"key1": "value1",
		"key2": "value2",
	}

	err := WriteFields("", fields)
	if err != nil {
		t.Fatalf("WriteFields failed: %v", err)
	}

	// Close the write end and read the output
	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	expected := "key1=value1\nkey2=value2\n"
	if output != expected {
		t.Errorf("Stdout output mismatch:\ngot:\n%q\nwant:\n%q", output, expected)
	}
}

func TestWriteFields_FileCreation(t *testing.T) {
	tmpDir := t.TempDir()
	outputFile := filepath.Join(tmpDir, "newfile.txt")

	// Ensure file doesn't exist
	if _, err := os.Stat(outputFile); err == nil {
		t.Fatalf("File should not exist before WriteFields")
	}

	fields := map[string]string{
		"key": "value",
	}

	err := WriteFields(outputFile, fields)
	if err != nil {
		t.Fatalf("WriteFields failed: %v", err)
	}

	// Check file was created
	if _, err := os.Stat(outputFile); err != nil {
		t.Fatalf("File was not created: %v", err)
	}

	content, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	expected := "key=value\n"
	if string(content) != expected {
		t.Errorf("File content mismatch:\ngot:\n%q\nwant:\n%q", string(content), expected)
	}
}

func TestWriteFields_UnwritablePath(t *testing.T) {
	// Try to write to a path that doesn't have permission
	outputFile := "/root/no-permission-test-file.txt"

	fields := map[string]string{
		"key": "value",
	}

	err := WriteFields(outputFile, fields)
	if err == nil {
		t.Fatalf("WriteFields should have failed for unwritable path")
	}

	// Check that error is wrapped
	if !strings.Contains(err.Error(), "open output file") {
		t.Errorf("Error should be wrapped with context, got: %v", err)
	}
}

func TestWriteFields_EmptyFields(t *testing.T) {
	tmpDir := t.TempDir()
	outputFile := filepath.Join(tmpDir, "output.txt")

	fields := map[string]string{}

	err := WriteFields(outputFile, fields)
	if err != nil {
		t.Fatalf("WriteFields failed: %v", err)
	}

	content, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	// Empty fields should result in empty file (no output)
	if len(content) != 0 {
		t.Errorf("File should be empty, got: %q", string(content))
	}
}

func TestWriteFields_SpecialCharacters(t *testing.T) {
	tmpDir := t.TempDir()
	outputFile := filepath.Join(tmpDir, "output.txt")

	fields := map[string]string{
		"key_with_underscore": "value with spaces",
		"key-with-dash": "value=with=equals",
		"UPPERCASE_KEY": "MixedCaseValue",
	}

	err := WriteFields(outputFile, fields)
	if err != nil {
		t.Fatalf("WriteFields failed: %v", err)
	}

	content, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	expected := "UPPERCASE_KEY=MixedCaseValue\nkey-with-dash=value=with=equals\nkey_with_underscore=value with spaces\n"
	if string(content) != expected {
		t.Errorf("File content mismatch:\ngot:\n%q\nwant:\n%q", string(content), expected)
	}
}

func TestWriteFields_MultipleFields(t *testing.T) {
	tmpDir := t.TempDir()
	outputFile := filepath.Join(tmpDir, "output.txt")

	fields := map[string]string{
		"a": "1",
		"b": "2",
		"c": "3",
		"d": "4",
		"e": "5",
	}

	err := WriteFields(outputFile, fields)
	if err != nil {
		t.Fatalf("WriteFields failed: %v", err)
	}

	content, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	expected := "a=1\nb=2\nc=3\nd=4\ne=5\n"
	if string(content) != expected {
		t.Errorf("File content mismatch:\ngot:\n%q\nwant:\n%q", string(content), expected)
	}
}

func TestWriteFields_FilePermissions(t *testing.T) {
	tmpDir := t.TempDir()
	outputFile := filepath.Join(tmpDir, "output.txt")

	fields := map[string]string{
		"key": "value",
	}

	err := WriteFields(outputFile, fields)
	if err != nil {
		t.Fatalf("WriteFields failed: %v", err)
	}

	// Check file permissions
	fileInfo, err := os.Stat(outputFile)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}

	// File should be readable and writable by owner
	mode := fileInfo.Mode()
	if mode&0644 != 0644 {
		t.Errorf("File permissions mismatch: got %o, want 0644", mode&0777)
	}
}

func TestWriteFields_LargeValues(t *testing.T) {
	tmpDir := t.TempDir()
	outputFile := filepath.Join(tmpDir, "output.txt")

	largeValue := strings.Repeat("x", 10000)
	fields := map[string]string{
		"large_key": largeValue,
		"small_key": "small",
	}

	err := WriteFields(outputFile, fields)
	if err != nil {
		t.Fatalf("WriteFields failed: %v", err)
	}

	content, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	expected := fmt.Sprintf("large_key=%s\nsmall_key=small\n", largeValue)
	if string(content) != expected {
		t.Errorf("File content mismatch (large value)")
	}
}

func TestWriteFields_DeterministicOutput(t *testing.T) {
	tmpDir := t.TempDir()
	outputFile1 := filepath.Join(tmpDir, "output1.txt")
	outputFile2 := filepath.Join(tmpDir, "output2.txt")

	fields := map[string]string{
		"z": "1",
		"a": "2",
		"m": "3",
	}

	// Write the same fields twice
	err := WriteFields(outputFile1, fields)
	if err != nil {
		t.Fatalf("First WriteFields failed: %v", err)
	}

	err = WriteFields(outputFile2, fields)
	if err != nil {
		t.Fatalf("Second WriteFields failed: %v", err)
	}

	content1, err := os.ReadFile(outputFile1)
	if err != nil {
		t.Fatalf("ReadFile 1 failed: %v", err)
	}

	content2, err := os.ReadFile(outputFile2)
	if err != nil {
		t.Fatalf("ReadFile 2 failed: %v", err)
	}

	if string(content1) != string(content2) {
		t.Errorf("Output is not deterministic:\nfile1:\n%q\nfile2:\n%q", string(content1), string(content2))
	}
}

func TestWriteFields_NewlineHandling(t *testing.T) {
	tmpDir := t.TempDir()
	outputFile := filepath.Join(tmpDir, "output.txt")

	fields := map[string]string{
		"key1": "value1",
		"key2": "value2",
	}

	err := WriteFields(outputFile, fields)
	if err != nil {
		t.Fatalf("WriteFields failed: %v", err)
	}

	content, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	lines := strings.Split(string(content), "\n")
	// Should have 3 elements: "key1=value1", "key2=value2", "" (trailing newline creates empty element)
	if len(lines) != 3 {
		t.Errorf("Expected 3 lines (including trailing empty), got %d: %v", len(lines), lines)
	}

	if lines[0] != "key1=value1" {
		t.Errorf("Line 0 mismatch: got %q, want %q", lines[0], "key1=value1")
	}

	if lines[1] != "key2=value2" {
		t.Errorf("Line 1 mismatch: got %q, want %q", lines[1], "key2=value2")
	}

	if lines[2] != "" {
		t.Errorf("Line 2 should be empty (trailing newline), got %q", lines[2])
	}
}

// delimiterIn extracts the heredoc delimiter used for key in got (the
// delimiter is random per write).
func delimiterIn(t *testing.T, got, key string) string {
	t.Helper()
	prefix := key + "<<"
	i := strings.Index(got, prefix)
	if i < 0 {
		t.Fatalf("no heredoc for %q in %q", key, got)
	}
	rest := got[i+len(prefix):]
	d, _, _ := strings.Cut(rest, "\n")
	if !strings.HasPrefix(d, "semrel_") || len(d) < 30 {
		t.Fatalf("unexpected delimiter %q", d)
	}
	return d
}

func TestWriteFields_MultilineValue_UsesHeredoc(t *testing.T) {
	tmpDir := t.TempDir()
	outputFile := filepath.Join(tmpDir, "output.txt")

	fields := map[string]string{
		"notes": "- feat: add foo\n- fix: bar baz",
	}

	if err := WriteFields(outputFile, fields); err != nil {
		t.Fatalf("WriteFields failed: %v", err)
	}

	content, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	heredocDelimiter := delimiterIn(t, string(content), "notes")
	expected := fmt.Sprintf("notes<<%s\n- feat: add foo\n- fix: bar baz\n%s\n", heredocDelimiter, heredocDelimiter)
	if string(content) != expected {
		t.Errorf("Multiline heredoc mismatch:\ngot:  %q\nwant: %q", string(content), expected)
	}
}

func TestWriteFields_SingleLineValue_UsesKeyEquals(t *testing.T) {
	tmpDir := t.TempDir()
	outputFile := filepath.Join(tmpDir, "output.txt")

	fields := map[string]string{
		"version": "1.2.3",
	}

	if err := WriteFields(outputFile, fields); err != nil {
		t.Fatalf("WriteFields failed: %v", err)
	}

	content, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	expected := "version=1.2.3\n"
	if string(content) != expected {
		t.Errorf("Single-line format mismatch:\ngot:  %q\nwant: %q", string(content), expected)
	}
}

func TestWriteFields_MixedSingleAndMultiline(t *testing.T) {
	tmpDir := t.TempDir()
	outputFile := filepath.Join(tmpDir, "output.txt")

	fields := map[string]string{
		"notes":    "- feat: foo\n- fix: bar",
		"released": "true",
		"version":  "2.0.0",
	}

	if err := WriteFields(outputFile, fields); err != nil {
		t.Fatalf("WriteFields failed: %v", err)
	}

	content, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	// Keys are sorted: notes, released, version
	heredocDelimiter := delimiterIn(t, string(content), "notes")
	expected := fmt.Sprintf(
		"notes<<%s\n- feat: foo\n- fix: bar\n%s\nreleased=true\nversion=2.0.0\n",
		heredocDelimiter, heredocDelimiter,
	)
	if string(content) != expected {
		t.Errorf("Mixed format mismatch:\ngot:  %q\nwant: %q", string(content), expected)
	}
}

func TestWriteFields_DelimiterIsRandomAndInjectionSafe(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "out")
	// A value containing the old fixed delimiter must not terminate the heredoc.
	evil := "x\n_GitHubActionsFileCommandDelimeter_\ninjected=1"
	if err := WriteFields(f, map[string]string{"notes": evil}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(f)
	d := delimiterIn(t, string(b), "notes")
	if strings.Count(string(b), d) != 2 {
		t.Errorf("delimiter must appear exactly twice, got:\n%q", string(b))
	}
	f2 := filepath.Join(dir, "out2")
	if err := WriteFields(f2, map[string]string{"notes": "a\nb"}); err != nil {
		t.Fatal(err)
	}
	b2, _ := os.ReadFile(f2)
	if d == delimiterIn(t, string(b2), "notes") {
		t.Error("delimiter must differ between writes")
	}
}

func TestWriteFields_RejectsInvalidKey(t *testing.T) {
	for _, k := range []string{"", "a\nb", "a=b", "a<<b"} {
		if err := WriteFields(filepath.Join(t.TempDir(), "o"), map[string]string{k: "v"}); err == nil {
			t.Errorf("key %q should be rejected", k)
		}
	}
}
