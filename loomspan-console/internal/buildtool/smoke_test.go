package main

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/agentskills"
)

func TestArchiveSidecarMismatchFailsClosed(t *testing.T) {
	root := t.TempDir()
	archive := filepath.Join(root, "archive.zip")
	if err := os.WriteFile(archive, []byte("archive"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(archive+".sha256", []byte(strings.Repeat("0", 64)+"  archive.zip\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := verifyArchiveSidecar(archive); err == nil {
		t.Fatal("mismatched checksum was accepted")
	}
}

func TestStrictSmokeRequiresExactRuntimeDebuggingSkill(t *testing.T) {
	paths, err := resolveProjectPaths()
	if err != nil {
		t.Fatal(err)
	}
	canonical := filepath.Join(paths.agentSkills, agentskills.RuntimeDebuggingSkillName)
	expected := map[string]os.FileMode{"LICENSE": 0o644, "README.md": 0o644, "loomspan-console.exe": 0o755}
	entries := map[string]smokeEntry{
		"LICENSE": {0o644, []byte("license")}, "README.md": {0o644, []byte("readme")},
		"loomspan-console.exe": {0o755, []byte("binary")},
	}
	for _, relative := range agentskills.RuntimeDebuggingFiles {
		name := filepath.ToSlash(filepath.Join("skills", agentskills.RuntimeDebuggingSkillName, relative))
		expected[name] = 0o644
		entries[name] = smokeEntry{0o644, mustRead(t, filepath.Join(canonical, filepath.FromSlash(relative)))}
	}
	root := t.TempDir()
	archive := filepath.Join(root, "good.zip")
	writeSmokeZIP(t, archive, "package", entries, "")
	out := filepath.Join(root, "out")
	if err := extractStrictArchive(archive, ".zip", out, "package", expected); err != nil {
		t.Fatal(err)
	}
	extracted := filepath.Join(out, "package", "skills", agentskills.RuntimeDebuggingSkillName)
	if err := agentskills.ValidateRuntimeDebugging(extracted); err != nil {
		t.Fatal(err)
	}
	for _, relative := range agentskills.RuntimeDebuggingFiles {
		if !bytes.Equal(mustRead(t, filepath.Join(canonical, filepath.FromSlash(relative))), mustRead(t, filepath.Join(extracted, filepath.FromSlash(relative)))) {
			t.Fatalf("extracted %s differs from canonical source", relative)
		}
	}

	for _, mutation := range []struct {
		name      string
		omit      string
		extraName string
	}{
		{"missing", "skills/loomspan/SKILL.md", ""},
		{"extra", "", "skills/loomspan/extra.md"},
		{"duplicate", "", "skills/loomspan/SKILL.md"},
	} {
		t.Run(mutation.name, func(t *testing.T) {
			archive := filepath.Join(t.TempDir(), "bad.zip")
			writeSmokeZIP(t, archive, "package", entries, mutation.omit)
			if mutation.extraName != "" {
				appendSmokeZIPEntry(t, archive, "package/"+mutation.extraName, 0o644)
			}
			if err := extractStrictArchive(archive, ".zip", t.TempDir(), "package", expected); err == nil {
				t.Fatalf("%s archive mutation was accepted", mutation.name)
			}
		})
	}
}

type smokeEntry struct {
	mode os.FileMode
	data []byte
}

func writeSmokeZIP(t *testing.T, filename, top string, entries map[string]smokeEntry, omit string) {
	t.Helper()
	file, err := os.Create(filename)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	for name, value := range entries {
		if name == omit {
			continue
		}
		header := &zip.FileHeader{Name: top + "/" + name}
		header.SetMode(value.mode)
		entry, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = entry.Write(value.data)
	}
	_ = writer.Close()
	_ = file.Close()
}

func appendSmokeZIPEntry(t *testing.T, filename, name string, mode os.FileMode) {
	t.Helper()
	// Rebuild because archive/zip cannot append portably.
	reader, err := zip.OpenReader(filename)
	if err != nil {
		t.Fatal(err)
	}
	var entries []struct {
		name string
		mode os.FileMode
		data []byte
	}
	for _, existing := range reader.File {
		opened, _ := existing.Open()
		data, _ := io.ReadAll(opened)
		opened.Close()
		entries = append(entries, struct {
			name string
			mode os.FileMode
			data []byte
		}{existing.Name, existing.Mode(), data})
	}
	reader.Close()
	file, _ := os.Create(filename)
	writer := zip.NewWriter(file)
	for _, existing := range entries {
		header := &zip.FileHeader{Name: existing.name}
		header.SetMode(existing.mode)
		entry, _ := writer.CreateHeader(header)
		_, _ = entry.Write(existing.data)
	}
	header := &zip.FileHeader{Name: name}
	header.SetMode(mode)
	entry, _ := writer.CreateHeader(header)
	_, _ = entry.Write([]byte("extra"))
	_ = writer.Close()
	_ = file.Close()
}

func mustRead(t *testing.T, filename string) []byte {
	t.Helper()
	contents, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	return contents
}

func TestStrictExtractionRejectsTraversalAndUnexpectedFiles(t *testing.T) {
	for _, entryName := range []string{"../escape", "package/extra", "other/loomspan-console.exe"} {
		t.Run(strings.ReplaceAll(entryName, "/", "-"), func(t *testing.T) {
			root := t.TempDir()
			archive := filepath.Join(root, "bad.zip")
			file, err := os.Create(archive)
			if err != nil {
				t.Fatal(err)
			}
			writer := zip.NewWriter(file)
			header := &zip.FileHeader{Name: entryName}
			header.SetMode(0o755)
			entry, _ := writer.CreateHeader(header)
			_, _ = entry.Write([]byte("x"))
			_ = writer.Close()
			_ = file.Close()
			err = extractStrictArchive(archive, ".zip", filepath.Join(root, "out"), "package", map[string]os.FileMode{"loomspan-console.exe": 0o755})
			if err == nil {
				t.Fatalf("unsafe entry %q was accepted", entryName)
			}
		})
	}

	for _, invalidMode := range []os.FileMode{0o644, os.ModeSymlink | 0o777, os.ModeDir | 0o755} {
		t.Run(invalidMode.String(), func(t *testing.T) {
			root := t.TempDir()
			archive := filepath.Join(root, "bad-mode.zip")
			writeSmokeZIP(t, archive, "package", map[string]smokeEntry{
				"loomspan-console.exe": {invalidMode, []byte("x")},
			}, "")
			err := extractStrictArchive(archive, ".zip", filepath.Join(root, "out"), "package", map[string]os.FileMode{"loomspan-console.exe": 0o755})
			if err == nil {
				t.Fatalf("unsafe mode %v was accepted", invalidMode)
			}
		})
	}
}

func TestPackageModeRejectsVersionMismatchBeforeBuilding(t *testing.T) {
	err := run([]string{"package", "--expected-version", "not-the-product-version"})
	if err == nil || !strings.Contains(err.Error(), "does not match root POM version") {
		t.Fatalf("version mismatch error = %v", err)
	}
}
