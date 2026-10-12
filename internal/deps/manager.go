// Package deps keeps yt-dlp, ffmpeg and deno installed and up to date in bin/.
package deps

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/cgamache/youtube-dl-gui/internal/github"
	"github.com/cgamache/youtube-dl-gui/internal/paths"
	"github.com/cgamache/youtube-dl-gui/internal/procutil"
)

// ErrMissingTool is returned when a tool is absent and cannot be installed.
var ErrMissingTool = errors.New("deps: required tool missing")

const maxExtractSize = 1 << 30 // 1 GiB per extracted file (zip-bomb guard)

// Installed records the installed version of a tool.
type Installed struct {
	Version   string    `json:"version"`
	CheckedAt time.Time `json:"checkedAt"`
}

// Progress is reported while Ensure runs.
type Progress struct {
	Tool    string
	Step    string // check | download | verify | extract | install | done
	Message string
	Done    int64
	Total   int64
}

// Status is the final state after Ensure.
type Status struct {
	Versions map[string]string
	Warnings []string
}

// Releases is the subset of the GitHub client used by Manager.
type Releases interface {
	LatestRelease(ctx context.Context, owner, repo string) (github.Release, error)
	Download(ctx context.Context, url, dst string, progress func(done, total int64)) error
}

// Manager installs and updates external tools.
type Manager struct {
	Paths paths.Paths
	GH    Releases
	Tools []Tool
	// VersionOf runs "<binary> --version"; defaults to running the binary.
	VersionOf func(ctx context.Context, binary string) (string, error)
	Now       func() time.Time
}

// NewManager builds a Manager for the default tools.
func NewManager(p paths.Paths, gh Releases) *Manager {
	return &Manager{Paths: p, GH: gh, Tools: DefaultTools()}
}

func (m *Manager) now() time.Time {
	if m.Now != nil {
		return m.Now()
	}
	return time.Now()
}

func (m *Manager) versionOf(ctx context.Context, binary string) (string, error) {
	if m.VersionOf != nil {
		return m.VersionOf(ctx, binary)
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	out, err := procutil.Command(ctx, binary, "--version").Output()
	if err != nil {
		return "", fmt.Errorf("run %s --version: %w", binary, err)
	}
	return strings.TrimSpace(string(out)), nil
}

func (m *Manager) loadVersions() map[string]Installed {
	versions := map[string]Installed{}
	data, err := os.ReadFile(m.Paths.VersionsFile)
	if err != nil {
		return versions
	}
	if err := json.Unmarshal(data, &versions); err != nil {
		slog.Warn("ignoring corrupt versions file", "err", err)
		return map[string]Installed{}
	}
	return versions
}

func (m *Manager) saveVersions(v map[string]Installed) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("encode versions: %w", err)
	}
	tmp := m.Paths.VersionsFile + ".part"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, m.Paths.VersionsFile); err != nil {
		return fmt.Errorf("rename %s: %w", tmp, err)
	}
	return nil
}

func (m *Manager) present(t Tool) bool {
	for _, f := range t.Files {
		if _, err := os.Stat(filepath.Join(m.Paths.Bin, f)); err != nil {
			return false
		}
	}
	return true
}

// Ensure makes sure every tool is installed and up to date. Failures on a
// tool that is already installed become warnings; failures on a missing tool
// abort with ErrMissingTool.
func (m *Manager) Ensure(ctx context.Context, report func(Progress)) (Status, error) {
	if report == nil {
		report = func(Progress) {}
	}
	if err := os.MkdirAll(m.Paths.Bin, 0o755); err != nil {
		return Status{}, fmt.Errorf("create %s: %w", m.Paths.Bin, err)
	}
	versions := m.loadVersions()
	st := Status{Versions: map[string]string{}}

	for _, t := range m.Tools {
		if err := ctx.Err(); err != nil {
			return st, err
		}
		inst, hasInst := versions[t.Name]
		present := hasInst && inst.Version != "" && m.present(t)

		if present && t.CheckEvery > 0 && m.now().Sub(inst.CheckedAt) < t.CheckEvery {
			st.Versions[t.Name] = inst.Version
			report(Progress{Tool: t.Name, Step: "done", Message: t.Name + " à jour"})
			continue
		}

		report(Progress{Tool: t.Name, Step: "check", Message: "Recherche de mises à jour de " + t.Name})
		newVersion, err := m.update(ctx, t, inst, present, report)
		if err == nil {
			if newVersion != "" {
				inst.Version = newVersion
			}
			inst.CheckedAt = m.now()
			versions[t.Name] = inst
			st.Versions[t.Name] = inst.Version
			if serr := m.saveVersions(versions); serr != nil {
				st.Warnings = append(st.Warnings, serr.Error())
			}
			report(Progress{Tool: t.Name, Step: "done", Message: t.Name + " prêt"})
			continue
		}
		if ctx.Err() != nil {
			return st, ctx.Err()
		}
		if !present {
			return st, fmt.Errorf("%w: %s: %w", ErrMissingTool, t.Name, err)
		}
		slog.Warn("tool update failed, keeping local version", "tool", t.Name, "err", err)
		st.Versions[t.Name] = inst.Version
		st.Warnings = append(st.Warnings, fmt.Sprintf("%s : mise à jour impossible (%v), version locale conservée", t.Name, err))
	}
	return st, nil
}

// update checks for a newer release and installs it. It returns the newly
// installed version, or "" when nothing changed.
func (m *Manager) update(ctx context.Context, t Tool, inst Installed, present bool, report func(Progress)) (string, error) {
	rel, err := m.GH.LatestRelease(ctx, t.Owner, t.Repo)
	if err != nil {
		return "", fmt.Errorf("latest release: %w", err)
	}
	latest := t.Version(rel)
	if present && !t.Newer(inst.Version, latest) {
		return "", nil
	}
	if err := m.install(ctx, t, rel, latest, report); err != nil {
		return "", err
	}
	return latest, nil
}

func (m *Manager) install(ctx context.Context, t Tool, rel github.Release, latest string, report func(Progress)) error {
	asset, ok := rel.Asset(t.AssetName)
	if !ok {
		return fmt.Errorf("asset %s not found in release %s", t.AssetName, rel.TagName)
	}
	tmp, err := os.MkdirTemp(m.Paths.Bin, ".dl-"+t.Name+"-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmp)

	archive := filepath.Join(tmp, "asset")
	report(Progress{Tool: t.Name, Step: "download", Message: "Téléchargement de " + t.Name, Total: asset.Size})
	err = m.GH.Download(ctx, asset.BrowserDownloadURL, archive, func(done, total int64) {
		report(Progress{Tool: t.Name, Step: "download", Message: "Téléchargement de " + t.Name, Done: done, Total: total})
	})
	if err != nil {
		return fmt.Errorf("download %s: %w", t.AssetName, err)
	}

	if err := m.verifyChecksum(ctx, t, rel, asset.Name, archive, report); err != nil {
		return err
	}

	staged := make([]string, len(t.Files))
	for i, f := range t.Files {
		staged[i] = filepath.Join(tmp, "new-"+f)
	}
	if strings.HasSuffix(strings.ToLower(t.AssetName), ".zip") {
		report(Progress{Tool: t.Name, Step: "extract", Message: "Extraction de " + t.Name})
		if err := extractZip(archive, t.Files, staged); err != nil {
			return err
		}
	} else {
		if len(staged) != 1 {
			return fmt.Errorf("tool %s: non-archive asset needs exactly one file", t.Name)
		}
		if err := os.Rename(archive, staged[0]); err != nil {
			return fmt.Errorf("stage %s: %w", t.Name, err)
		}
	}

	report(Progress{Tool: t.Name, Step: "install", Message: "Installation de " + t.Name})
	return m.swap(ctx, t, staged, rel.TagName, report)
}

func (m *Manager) verifyChecksum(ctx context.Context, t Tool, rel github.Release, assetName, file string, report func(Progress)) error {
	if t.ChecksumAsset == "" {
		return nil
	}
	sumAsset, ok := rel.Asset(t.ChecksumAsset)
	if !ok {
		slog.Warn("checksum asset missing, skipping verification", "tool", t.Name, "asset", t.ChecksumAsset)
		return nil
	}
	report(Progress{Tool: t.Name, Step: "verify", Message: "Vérification de l'intégrité de " + t.Name})
	sumFile := file + ".sums"
	if err := m.GH.Download(ctx, sumAsset.BrowserDownloadURL, sumFile, nil); err != nil {
		return fmt.Errorf("download %s: %w", t.ChecksumAsset, err)
	}
	f, err := os.Open(sumFile)
	if err != nil {
		return fmt.Errorf("open checksums: %w", err)
	}
	defer f.Close()
	sums, err := ParseChecksums(f)
	if err != nil {
		return err
	}
	want, ok := lookupChecksum(sums, assetName)
	if !ok {
		return fmt.Errorf("no checksum for %s in %s", assetName, t.ChecksumAsset)
	}
	got, err := sha256File(file)
	if err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("checksum mismatch for %s: expected %s, got %s", assetName, want, got)
	}
	return nil
}

// swap moves staged files into bin/, keeping *.old backups until the new
// binary is validated; any failure restores the previous files.
func (m *Manager) swap(ctx context.Context, t Tool, staged []string, wantVersion string, report func(Progress)) error {
	dsts := make([]string, len(t.Files))
	backed := make([]bool, len(t.Files))
	for i, f := range t.Files {
		dsts[i] = filepath.Join(m.Paths.Bin, f)
	}
	rollback := func() {
		for i, dst := range dsts {
			os.Remove(dst)
			if backed[i] {
				if err := os.Rename(dst+".old", dst); err != nil {
					slog.Error("rollback failed", "file", dst, "err", err)
				}
			}
		}
	}

	for i, dst := range dsts {
		if _, err := os.Stat(dst); err == nil {
			os.Remove(dst + ".old")
			if err := os.Rename(dst, dst+".old"); err != nil {
				rollback()
				return fmt.Errorf("backup %s: %w", dst, err)
			}
			backed[i] = true
		}
		if err := os.Rename(staged[i], dst); err != nil {
			rollback()
			return fmt.Errorf("install %s: %w", dst, err)
		}
	}

	if t.VerifyBinary {
		got, err := m.versionOf(ctx, dsts[0])
		if err != nil || CompareYtdlp(got, wantVersion) != 0 {
			rollback()
			if err != nil {
				return fmt.Errorf("verify new %s: %w", t.Name, err)
			}
			return fmt.Errorf("verify new %s: expected version %s, got %s", t.Name, wantVersion, got)
		}
	}
	for i, dst := range dsts {
		if backed[i] {
			os.Remove(dst + ".old")
		}
	}
	return nil
}

// extractZip extracts the entries whose base name matches wanted[i] into
// dsts[i]. Entry paths are never used to build destinations (zip-slip safe).
func extractZip(archive string, wanted, dsts []string) error {
	zr, err := zip.OpenReader(archive)
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}
	defer zr.Close()

	found := make([]bool, len(wanted))
	for _, zf := range zr.File {
		if zf.FileInfo().IsDir() {
			continue
		}
		base := path.Base(strings.ReplaceAll(zf.Name, "\\", "/"))
		for i, w := range wanted {
			if base != w || found[i] {
				continue
			}
			if err := extractEntry(zf, dsts[i]); err != nil {
				return err
			}
			found[i] = true
		}
	}
	for i, ok := range found {
		if !ok {
			return fmt.Errorf("%s not found in archive", wanted[i])
		}
	}
	return nil
}

func extractEntry(zf *zip.File, dst string) error {
	rc, err := zf.Open()
	if err != nil {
		return fmt.Errorf("open zip entry %s: %w", zf.Name, err)
	}
	defer rc.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return fmt.Errorf("create %s: %w", dst, err)
	}
	n, err := io.Copy(out, io.LimitReader(rc, maxExtractSize+1))
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return fmt.Errorf("extract %s: %w", zf.Name, err)
	}
	if n > maxExtractSize {
		return fmt.Errorf("extract %s: entry too large", zf.Name)
	}
	return nil
}
