package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestGetPassphraseExplicitSources(t *testing.T) {
	t.Run("file trims trailing newline", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "passphrase")
		if err := os.WriteFile(path, []byte("file-secret\r\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := getPassphrase(passphraseInput{file: path}, "unused", true)
		if err != nil {
			t.Fatal(err)
		}
		if got != "file-secret" {
			t.Fatalf("passphrase from file = %q", got)
		}
	})

	t.Run("stdin trims trailing newline", func(t *testing.T) {
		withTestStdin(t, "stdin-secret\n", func() {
			got, err := getPassphrase(passphraseInput{stdin: true}, "unused", true)
			if err != nil {
				t.Fatal(err)
			}
			if got != "stdin-secret" {
				t.Fatalf("passphrase from stdin = %q", got)
			}
		})
	})

	t.Run("argument remains compatible", func(t *testing.T) {
		got, err := getPassphrase(passphraseInput{argument: "argument-secret", argumentSet: true}, "unused", false)
		if err != nil {
			t.Fatal(err)
		}
		if got != "argument-secret" {
			t.Fatalf("passphrase from deprecated argument = %q", got)
		}
	})
}

func TestGetPassphraseRejectsUnsafeSourceCombinationsAndEmptyValues(t *testing.T) {
	secret := "must-not-appear"
	_, err := getPassphrase(passphraseInput{
		argument:    secret,
		argumentSet: true,
		file:        "/tmp/passphrase",
	}, "unused", false)
	if err == nil || !strings.Contains(err.Error(), "provide only one") {
		t.Fatalf("conflicting sources error = %v", err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("conflicting sources error leaked passphrase: %v", err)
	}

	if _, err := getPassphrase(passphraseInput{argumentSet: true}, "unused", false); err == nil || err.Error() != "passphrase cannot be empty" {
		t.Fatalf("empty argument error = %v", err)
	}

	path := filepath.Join(t.TempDir(), "empty-passphrase")
	if err := os.WriteFile(path, []byte("\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := getPassphrase(passphraseInput{file: path}, "unused", false); err == nil || err.Error() != "passphrase cannot be empty" {
		t.Fatalf("empty file error = %v", err)
	}
}

func TestGetPassphraseNonInteractiveRequiresExplicitSource(t *testing.T) {
	withTestStdin(t, "must-not-be-consumed\n", func() {
		if _, err := getPassphrase(passphraseInput{}, "unused", false); err == nil || err.Error() != "non-interactive passphrase input requires --passphrase-stdin or --passphrase-file" {
			t.Fatalf("non-interactive source error = %v", err)
		}
	})
}

func TestPassphraseCommandsExposeSecureSourcesAndDeprecateArgument(t *testing.T) {
	for _, cmd := range []*cobra.Command{backupCmd, restoreCmd, keysExportCmd, keysImportCmd} {
		if cmd.Flags().Lookup("passphrase-file") == nil || cmd.Flags().Lookup("passphrase-stdin") == nil {
			t.Errorf("%s is missing secure passphrase flags", cmd.CommandPath())
		}
		flag := cmd.Flags().Lookup("passphrase")
		if flag == nil || flag.Deprecated == "" {
			t.Errorf("%s --passphrase is not deprecated", cmd.CommandPath())
		}
	}
}

func withTestStdin(t *testing.T, input string, fn func()) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.WriteString(input); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	oldStdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = oldStdin
		_ = r.Close()
	})
	fn()
}
