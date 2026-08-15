package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/agentskills"
	"github.com/mgiacomi/loomspan/loomspan-console/internal/release"
)

var archiveTimestamp = time.Date(1980, time.January, 1, 0, 0, 0, 0, time.UTC)

type releaseTarget struct {
	goos, goarch string
	label        string
	extension    string
}

type packageRequest struct {
	version, executable, license, readme, skill, outputDirectory string
	target                                                       releaseTarget
}

type packageResult struct {
	archive, sidecar string
	digest           string
}

func packageCurrentTarget(context pipelineContext) (packageResult, error) {
	target, err := supportedReleaseTarget(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return packageResult{}, err
	}
	return writeReleasePackage(packageRequest{
		version: context.productVersion, target: target,
		executable:      filepath.Join(context.paths.build, executableName()),
		license:         filepath.Join(context.paths.repository, "LICENSE"),
		readme:          filepath.Join(context.paths.release, "README.md"),
		skill:           filepath.Join(context.paths.agentSkills, agentskills.RuntimeDebuggingSkillName),
		outputDirectory: context.paths.dist,
	})
}

func supportedReleaseTarget(goos, goarch string) (releaseTarget, error) {
	switch goos + "/" + goarch {
	case "windows/amd64":
		return releaseTarget{goos: goos, goarch: goarch, label: "windows-x86_64", extension: ".zip"}, nil
	case "linux/amd64":
		return releaseTarget{goos: goos, goarch: goarch, label: "linux-x86_64", extension: ".tar.gz"}, nil
	case "darwin/arm64":
		return releaseTarget{goos: goos, goarch: goarch, label: "macos-arm64", extension: ".tar.gz"}, nil
	default:
		return releaseTarget{}, fmt.Errorf("unsupported release target %s/%s", goos, goarch)
	}
}

func writeReleasePackage(request packageRequest) (packageResult, error) {
	if err := release.ValidateProductVersion(request.version); err != nil {
		return packageResult{}, err
	}
	wanted, err := supportedReleaseTarget(request.target.goos, request.target.goarch)
	if err != nil || wanted != request.target {
		return packageResult{}, fmt.Errorf("invalid release target declaration")
	}
	top := "loomspan-console-" + request.version + "-" + request.target.label
	if path.Base(top) != top || strings.ContainsAny(top, `/\\`) {
		return packageResult{}, fmt.Errorf("unsafe archive directory %q", top)
	}
	if err := agentskills.ValidateRuntimeDebugging(request.skill); err != nil {
		return packageResult{}, fmt.Errorf("validate runtime debugging skill: %w", err)
	}
	executableName := "loomspan-console"
	if request.target.goos == "windows" {
		executableName += ".exe"
	}
	files := []packageFile{
		{name: "LICENSE", source: request.license, mode: 0o644},
		{name: "README.md", source: request.readme, mode: 0o644},
		{name: executableName, source: request.executable, mode: 0o755},
	}
	for _, relative := range agentskills.RuntimeDebuggingFiles {
		archiveName := path.Join("skills", agentskills.RuntimeDebuggingSkillName, relative)
		if path.Clean(archiveName) != archiveName || strings.HasPrefix(archiveName, "../") {
			return packageResult{}, fmt.Errorf("unsafe skill archive path %q", archiveName)
		}
		files = append(files, packageFile{name: archiveName, source: filepath.Join(request.skill, filepath.FromSlash(relative)), mode: 0o644})
	}
	for index := range files {
		if err := files[index].load(); err != nil {
			return packageResult{}, err
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].name < files[j].name })
	var archive bytes.Buffer
	if request.target.extension == ".zip" {
		err = writeDeterministicZIP(&archive, top, files)
	} else {
		err = writeDeterministicTarGzip(&archive, top, files)
	}
	if err != nil {
		return packageResult{}, err
	}
	if err := os.MkdirAll(request.outputDirectory, 0o755); err != nil {
		return packageResult{}, err
	}
	archiveName := top + request.target.extension
	archivePath := filepath.Join(request.outputDirectory, archiveName)
	if err := os.WriteFile(archivePath, archive.Bytes(), 0o644); err != nil {
		return packageResult{}, err
	}
	digestBytes := sha256.Sum256(archive.Bytes())
	digest := hex.EncodeToString(digestBytes[:])
	sidecarPath := archivePath + ".sha256"
	if err := os.WriteFile(sidecarPath, []byte(digest+"  "+archiveName+"\n"), 0o644); err != nil {
		return packageResult{}, err
	}
	return packageResult{archive: archivePath, sidecar: sidecarPath, digest: digest}, nil
}

type packageFile struct {
	name, source string
	mode         os.FileMode
	contents     []byte
}

func (file *packageFile) load() error {
	info, err := os.Lstat(file.source)
	if err != nil {
		return fmt.Errorf("inspect package input %s: %w", file.name, err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("package input %s must be a regular non-symlink file", file.name)
	}
	file.contents, err = os.ReadFile(file.source)
	return err
}

func writeDeterministicZIP(output io.Writer, top string, files []packageFile) error {
	writer := zip.NewWriter(output)
	for _, file := range files {
		header := &zip.FileHeader{Name: path.Join(top, file.name), Method: zip.Deflate}
		header.SetMode(file.mode)
		header.SetModTime(archiveTimestamp)
		header.Extra = nil
		entry, err := writer.CreateHeader(header)
		if err != nil {
			return err
		}
		if _, err := entry.Write(file.contents); err != nil {
			return err
		}
	}
	return writer.Close()
}

func writeDeterministicTarGzip(output io.Writer, top string, files []packageFile) error {
	gzipWriter, err := gzip.NewWriterLevel(output, gzip.BestCompression)
	if err != nil {
		return err
	}
	gzipWriter.Header.ModTime = time.Unix(0, 0).UTC()
	gzipWriter.Header.OS = 255
	tarWriter := tar.NewWriter(gzipWriter)
	for _, file := range files {
		header := &tar.Header{Name: path.Join(top, file.name), Mode: int64(file.mode.Perm()), Size: int64(len(file.contents)),
			ModTime: time.Unix(0, 0).UTC(), AccessTime: time.Time{}, ChangeTime: time.Time{}, Typeflag: tar.TypeReg, Format: tar.FormatPAX}
		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}
		if _, err := tarWriter.Write(file.contents); err != nil {
			return err
		}
	}
	if err := tarWriter.Close(); err != nil {
		return err
	}
	return gzipWriter.Close()
}
