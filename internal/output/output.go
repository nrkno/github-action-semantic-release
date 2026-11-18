package output

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"sort"
	"strings"
)

// WriteFields writes key=value pairs to outputFile (append mode) or stdout if outputFile is empty.
// Keys are sorted for deterministic output.
// Multiline values are written in the heredoc format required by GitHub Actions GITHUB_OUTPUT.
// Returns error if file write fails.
func WriteFields(outputFile string, fields map[string]string) (err error) {
	// Sort keys for deterministic output
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var sb strings.Builder
	for _, k := range keys {
		if k == "" || strings.ContainsAny(k, "\r\n=<") {
			return fmt.Errorf("invalid output key %q", k)
		}
		v := fields[k]
		if strings.ContainsAny(v, "\r\n") {
			// Multiline: use heredoc format required by GitHub Actions. The
			// delimiter is random per value so a value can never close the
			// heredoc early and inject further output keys.
			delimiter, derr := newDelimiter(v)
			if derr != nil {
				return derr
			}
			fmt.Fprintf(&sb, "%s<<%s\n%s\n%s\n", k, delimiter, v, delimiter)
		} else {
			fmt.Fprintf(&sb, "%s=%s\n", k, v)
		}
	}

	// Write to file or stdout
	if outputFile != "" {
		// Append to file
		var f *os.File
		f, err = os.OpenFile(outputFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Errorf("open output file: %w", err)
		}
		defer func() {
			if cerr := f.Close(); cerr != nil && err == nil {
				err = fmt.Errorf("close output file: %w", cerr)
			}
		}()
		_, err = f.WriteString(sb.String())
		if err != nil {
			return fmt.Errorf("write output file: %w", err)
		}
	} else {
		// Write to stdout
		fmt.Print(sb.String())
	}

	return nil
}

// newDelimiter returns a random heredoc delimiter that does not occur in value.
func newDelimiter(value string) (string, error) {
	for range 8 {
		b := make([]byte, 16)
		if _, err := rand.Read(b); err != nil {
			return "", fmt.Errorf("generate output delimiter: %w", err)
		}
		d := "semrel_" + hex.EncodeToString(b)
		if !strings.Contains(value, d) {
			return d, nil
		}
	}
	return "", fmt.Errorf("could not generate a unique output delimiter")
}
