// Package web exports a self-contained, offline graph viewer. Layout and HTML
// generation use Go only; original game images remain optional local inputs.
package web

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/glthr/DeMystify/assets"
	"github.com/glthr/DeMystify/common"
)

//go:embed viewer.html viewer.css viewer.js
var viewerFiles embed.FS

const ownershipMarker = "<!-- DeMystify offline graph export v1 -->"

// Write publishes an offline index.html and optional copied artwork. It never
// changes graph metadata or original input files. Existing unrelated output is
// refused. On refresh, immutable image versions are installed first and the
// index is replaced by one atomic rename, preserving the previous usable index
// on any earlier failure. Older image versions are intentionally retained.
func Write(dir string, metadata *common.Metadata, options Options) (Report, error) {
	data, err := collectData(metadata, options.Corpus)
	if err != nil {
		return Report{}, err
	}
	target, err := outputDirectory(dir, options)
	if err != nil {
		return Report{}, err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return Report{}, fmt.Errorf("creating HTML output parent: %w", err)
	}
	stage, err := os.MkdirTemp(filepath.Dir(target), ".demystify-web-*")
	if err != nil {
		return Report{}, fmt.Errorf("creating HTML staging directory: %w", err)
	}
	defer os.RemoveAll(stage)
	report := Report{Dir: target, IndexPath: filepath.Join(target, "index.html"), Nodes: len(data.Nodes), Edges: len(data.Edges)}
	version := ""
	// Prepare also counts eligible missing views when no artwork is supplied,
	// without opening or creating any source/destination files in that case.
	imageDir := filepath.Join(stage, ".images")
	images, assetReport, err := assets.Prepare(options.AssetsDir, imageDir, metadata.Nodes)
	if err != nil {
		return Report{}, fmt.Errorf("preparing HTML artwork: %w", err)
	}
	report.Warnings = append(report.Warnings, assetReport.Diagnostics...)
	report.MissingImages = assetReport.Missing
	if len(images) > 0 {
		digest, err := directoryDigest(imageDir)
		if err != nil {
			return Report{}, fmt.Errorf("validating staged HTML artwork: %w", err)
		}
		version = "images-" + digest
		if err := os.Rename(imageDir, filepath.Join(stage, version)); err != nil {
			return Report{}, fmt.Errorf("versioning HTML artwork: %w", err)
		}
		for i := range data.Nodes {
			node := &data.Nodes[i]
			image, exists := images[node.ID]
			if !exists {
				continue
			}
			full, err := imageURL(filepath.Join(stage, version), version, image.Path)
			if err != nil {
				return Report{}, err
			}
			thumbnail := full
			if image.Thumbnail != "" {
				thumbnail, err = imageURL(filepath.Join(stage, version), version, image.Thumbnail)
				if err != nil {
					return Report{}, err
				}
			}
			node.Image, node.Thumbnail = full, thumbnail
			if image.Name != "" {
				node.AssetName = image.Name
			}
			node.ImageWidth, node.ImageHeight = image.Width, image.Height
			node.Height = 132
			report.Images++
		}
	} else {
		if err := os.RemoveAll(imageDir); err != nil {
			return Report{}, err
		}
	}
	data.Meta.Images, data.Meta.MissingImages = report.Images, report.MissingImages
	layout(&data)
	html, err := renderHTML(data)
	if err != nil {
		return Report{}, err
	}
	if err := writeSynced(filepath.Join(stage, "index.html"), html); err != nil {
		return Report{}, fmt.Errorf("writing staged HTML: %w", err)
	}
	if err := publish(stage, target, version); err != nil {
		return Report{}, err
	}
	return report, nil
}

func renderHTML(data graphData) ([]byte, error) {
	template, err := viewerFiles.ReadFile("viewer.html")
	if err != nil {
		return nil, err
	}
	css, err := viewerFiles.ReadFile("viewer.css")
	if err != nil {
		return nil, err
	}
	js, err := viewerFiles.ReadFile("viewer.js")
	if err != nil {
		return nil, err
	}
	for _, token := range []string{"{{STYLE}}", "{{GRAPH_JSON}}", "{{SCRIPT}}"} {
		if bytes.Count(template, []byte(token)) != 1 {
			return nil, fmt.Errorf("HTML viewer must contain exactly one %s placeholder", token)
		}
	}
	// encoding/json escapes <, >, &, U+2028 and U+2029, including </script>.
	// User-controlled names/scripts therefore cannot terminate this script.
	encoded, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("encoding HTML graph: %w", err)
	}
	output := strings.Replace(string(template), "{{STYLE}}", string(css), 1)
	output = strings.Replace(output, "{{SCRIPT}}", string(js), 1)
	output = strings.Replace(output, "{{GRAPH_JSON}}", string(encoded), 1)
	return []byte(ownershipMarker + "\n" + output), nil
}

func outputDirectory(dir string, options Options) (string, error) {
	if strings.TrimSpace(dir) == "" {
		return "", fmt.Errorf("HTML output directory is empty")
	}
	if info, err := os.Lstat(dir); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("HTML output cannot be a symlink")
	} else if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	target, err := canonicalPath(dir)
	if err != nil {
		return "", fmt.Errorf("resolving HTML output: %w", err)
	}
	if filepath.Dir(target) == target {
		return "", fmt.Errorf("HTML output cannot be a filesystem root")
	}
	for _, source := range []struct{ name, path string }{{"artwork", options.AssetsDir}, {"original stack input", options.InputDir}} {
		if source.path == "" {
			continue
		}
		protected, err := canonicalPath(source.path)
		if err != nil {
			return "", fmt.Errorf("resolving %s directory: %w", source.name, err)
		}
		if containsPath(target, protected) || containsPath(protected, target) {
			return "", fmt.Errorf("HTML output directory overlaps %s directory", source.name)
		}
	}
	if err := validateOutput(target); err != nil {
		return "", err
	}
	return target, nil
}

func canonicalPath(name string) (string, error) {
	abs, err := filepath.Abs(name)
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)
	current := abs
	var suffix []string
	for {
		_, err := os.Lstat(current)
		if err == nil {
			resolved, err := filepath.EvalSymlinks(current)
			if err != nil {
				return "", err
			}
			for i := len(suffix) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, suffix[i])
			}
			return resolved, nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", err
		}
		suffix = append(suffix, filepath.Base(current))
		current = parent
	}
}

func containsPath(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	return err == nil && (rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))))
}

func validateOutput(target string) error {
	info, err := os.Lstat(target)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("HTML output must be a directory, not a file or symlink")
	}
	entries, err := os.ReadDir(target)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return nil
	}
	index := filepath.Join(target, "index.html")
	info, err = os.Lstat(index)
	if err != nil || !info.Mode().IsRegular() {
		return fmt.Errorf("refusing nonempty HTML output directory without an exporter-owned index.html")
	}
	file, err := os.Open(index)
	if err != nil {
		return err
	}
	defer file.Close()
	leading, err := io.ReadAll(io.LimitReader(file, 512))
	if err != nil {
		return err
	}
	if !bytes.HasPrefix(leading, []byte(ownershipMarker+"\n")) {
		return fmt.Errorf("refusing to replace an index.html not generated by DeMystify")
	}
	return nil
}

func imageURL(root, version, name string) (string, error) {
	if name == "" || strings.Contains(name, "\\") || path.IsAbs(name) || filepath.IsAbs(name) || path.Clean(name) != name || name == "." || strings.HasPrefix(name, "../") {
		return "", fmt.Errorf("unsafe exported image path %q", name)
	}
	for _, segment := range strings.Split(name, "/") {
		if segment == ".." || segment == "" {
			return "", fmt.Errorf("unsafe exported image path %q", name)
		}
	}
	full := filepath.Join(root, filepath.FromSlash(name))
	info, err := os.Lstat(full)
	if err != nil || !info.Mode().IsRegular() {
		return "", fmt.Errorf("exported image %q is missing or not a regular file", name)
	}
	escaped := []string{url.PathEscape(version)}
	for _, segment := range strings.Split(name, "/") {
		escaped = append(escaped, url.PathEscape(segment))
	}
	return strings.Join(escaped, "/"), nil
}

func directoryDigest(root string) (string, error) {
	hash := sha256.New()
	err := filepath.WalkDir(root, func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("staged artwork contains symlink %q", name)
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("staged artwork contains nonregular file %q", name)
		}
		rel, err := filepath.Rel(root, name)
		if err != nil {
			return err
		}
		file, err := os.Open(name)
		if err != nil {
			return err
		}
		fileHash := sha256.New()
		_, copyErr := io.Copy(fileHash, file)
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		fmt.Fprintf(hash, "%s\x00%x\n", filepath.ToSlash(rel), fileHash.Sum(nil))
		return nil
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func writeSynced(name string, data []byte) error {
	file, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return err
	}
	if _, err = file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err = file.Sync(); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}

func publish(stage, target, version string) error {
	if err := validateOutput(target); err != nil {
		return err
	}
	if _, err := os.Lstat(target); os.IsNotExist(err) {
		if err := os.Rename(stage, target); err != nil {
			return fmt.Errorf("publishing HTML directory: %w", err)
		}
		return nil
	} else if err != nil {
		return err
	}
	entries, err := os.ReadDir(target)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		// POSIX rename can also atomically replace an empty destination dir.
		// On platforms that refuse this form, the unchanged empty directory
		// remains safe and the error is returned before installing any files.
		if err := os.Rename(stage, target); err != nil {
			return fmt.Errorf("publishing HTML into empty directory: %w", err)
		}
		return nil
	}
	if version != "" {
		destination := filepath.Join(target, version)
		if _, err := os.Lstat(destination); os.IsNotExist(err) {
			if err := os.Rename(filepath.Join(stage, version), destination); err != nil {
				return fmt.Errorf("installing HTML artwork: %w", err)
			}
		} else if err != nil {
			return err
		} else {
			actual, err := directoryDigest(destination)
			if err != nil || "images-"+actual != version {
				return fmt.Errorf("existing HTML artwork version failed verification")
			}
		}
	}
	if err := os.Rename(filepath.Join(stage, "index.html"), filepath.Join(target, "index.html")); err != nil {
		return fmt.Errorf("publishing HTML index: %w", err)
	}
	return nil
}
