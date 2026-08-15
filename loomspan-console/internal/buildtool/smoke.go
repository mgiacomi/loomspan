package main

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/agentskills"
)

func smokeReleaseArchive(archive, version string) error {
	target, err := supportedReleaseTarget(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return err
	}
	expectedName := "loomspan-console-" + version + "-" + target.label + target.extension
	if filepath.Base(archive) != expectedName {
		return fmt.Errorf("archive name %q does not match current target %q", filepath.Base(archive), expectedName)
	}
	if err := verifyArchiveSidecar(archive); err != nil {
		return err
	}
	root, err := os.MkdirTemp("", "loomspan-console-smoke-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)
	top := strings.TrimSuffix(expectedName, target.extension)
	executableName := "loomspan-console"
	if target.goos == "windows" {
		executableName += ".exe"
	}
	expected := map[string]os.FileMode{"LICENSE": 0o644, "README.md": 0o644, executableName: 0o755}
	for _, relative := range agentskills.RuntimeDebuggingFiles {
		expected[filepath.ToSlash(filepath.Join("skills", agentskills.RuntimeDebuggingSkillName, relative))] = 0o644
	}
	if err := extractStrictArchive(archive, target.extension, root, top, expected); err != nil {
		return err
	}
	extractedSkill := filepath.Join(root, top, "skills", agentskills.RuntimeDebuggingSkillName)
	if err := agentskills.ValidateRuntimeDebugging(extractedSkill); err != nil {
		return fmt.Errorf("validate extracted runtime debugging skill: %w", err)
	}
	paths, err := resolveProjectPaths()
	if err != nil {
		return err
	}
	canonicalSkill := filepath.Join(paths.agentSkills, agentskills.RuntimeDebuggingSkillName)
	for _, relative := range agentskills.RuntimeDebuggingFiles {
		canonical, err := os.ReadFile(filepath.Join(canonicalSkill, filepath.FromSlash(relative)))
		if err != nil {
			return fmt.Errorf("read canonical skill %s: %w", relative, err)
		}
		extracted, err := os.ReadFile(filepath.Join(extractedSkill, filepath.FromSlash(relative)))
		if err != nil || !bytes.Equal(canonical, extracted) {
			return fmt.Errorf("packaged skill %s differs from canonical source", relative)
		}
	}
	executable := filepath.Join(root, top, executableName)
	versionOutput, err := commandOutput(filepath.Dir(executable), nil, executable, "--version")
	if err != nil {
		return err
	}
	if !strings.Contains(versionOutput, version) {
		return fmt.Errorf("packaged --version output %q does not contain %q", strings.TrimSpace(versionOutput), version)
	}
	return smokeStartup(executable, root)
}

func verifyArchiveSidecar(archive string) error {
	contents, err := os.ReadFile(archive)
	if err != nil {
		return err
	}
	sidecar, err := os.ReadFile(archive + ".sha256")
	if err != nil {
		return fmt.Errorf("read checksum sidecar: %w", err)
	}
	digest := sha256.Sum256(contents)
	expected := hex.EncodeToString(digest[:]) + "  " + filepath.Base(archive) + "\n"
	if string(sidecar) != expected {
		return fmt.Errorf("checksum sidecar does not match archive")
	}
	return nil
}

func extractStrictArchive(filename, extension, destination, top string, expected map[string]os.FileMode) error {
	seen := make(map[string]bool)
	write := func(name string, mode os.FileMode, contents io.Reader) error {
		prefix := top + "/"
		if !strings.HasPrefix(name, prefix) || strings.Contains(name, "\\") || strings.Contains(name, "../") {
			return fmt.Errorf("unsafe archive path %q", name)
		}
		if !mode.IsRegular() || mode&os.ModeSymlink != 0 {
			return fmt.Errorf("archive entry %q is not a regular file", name)
		}
		relative := strings.TrimPrefix(name, prefix)
		wantedMode, ok := expected[relative]
		if !ok || seen[relative] || mode.Perm() != wantedMode {
			return fmt.Errorf("unexpected archive entry %q with mode %04o", name, mode.Perm())
		}
		seen[relative] = true
		target := filepath.Join(destination, top, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		file, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, wantedMode)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(file, contents)
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	}
	if extension == ".zip" {
		reader, err := zip.OpenReader(filename)
		if err != nil {
			return err
		}
		defer reader.Close()
		for _, entry := range reader.File {
			opened, err := entry.Open()
			if err != nil {
				return err
			}
			err = write(entry.Name, entry.Mode(), opened)
			opened.Close()
			if err != nil {
				return err
			}
		}
	} else {
		file, err := os.Open(filename)
		if err != nil {
			return err
		}
		defer file.Close()
		gz, err := gzip.NewReader(file)
		if err != nil {
			return err
		}
		defer gz.Close()
		reader := tar.NewReader(gz)
		for {
			header, err := reader.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return err
			}
			if header.Typeflag != tar.TypeReg {
				return fmt.Errorf("archive entry %q is not a regular file", header.Name)
			}
			if err := write(header.Name, os.FileMode(header.Mode), io.LimitReader(reader, header.Size)); err != nil {
				return err
			}
		}
	}
	if len(seen) != len(expected) {
		return fmt.Errorf("archive contains %d required files, want %d", len(seen), len(expected))
	}
	return nil
}

func smokeStartup(executable, root string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	profile := filepath.Join(root, "profile", "config.yaml")
	workspace := filepath.Join(root, "work")
	command := exec.CommandContext(ctx, executable, "--config", profile, "--work-dir", workspace, "--listen", "127.0.0.1:0", "--no-open-browser")
	stdout, err := command.StdoutPipe()
	if err != nil {
		return err
	}
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		return err
	}
	defer func() {
		_ = command.Process.Kill()
		_ = command.Wait()
	}()
	lines := make(chan string, 1)
	errors := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			if strings.HasPrefix(scanner.Text(), "Pairing URL: ") {
				lines <- strings.TrimPrefix(scanner.Text(), "Pairing URL: ")
				return
			}
		}
		errors <- scanner.Err()
	}()
	select {
	case pairingURL := <-lines:
		fragment := strings.IndexByte(pairingURL, '#')
		if fragment >= 0 {
			pairingURL = pairingURL[:fragment]
		}
		client := &http.Client{Timeout: 5 * time.Second}
		response, err := client.Get(pairingURL)
		if err != nil {
			return fmt.Errorf("request packaged bootstrap: %w", err)
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			return fmt.Errorf("packaged bootstrap status = %d", response.StatusCode)
		}
		return nil
	case err := <-errors:
		return fmt.Errorf("packaged process exited before pairing: %v: %s", err, stderr.String())
	case <-ctx.Done():
		return fmt.Errorf("packaged process startup timed out: %s", stderr.String())
	}
}
