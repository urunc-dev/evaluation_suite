package artifact

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/urunc-dev/evaluation_suite/internal/environment"
	"github.com/urunc-dev/evaluation_suite/internal/orchestrator"
	"github.com/urunc-dev/evaluation_suite/internal/plan"
)

type Store struct {
	Root string
}

type RunPaths struct {
	RunDir string
}

func NewStore(root string) *Store {
	if root == "" {
		root = "results"
	}

	return &Store{
		Root: root,
	}
}

func (s *Store) InitRun(
	runID string,
	manifestPath string,
	p *plan.Plan,
	env *environment.Environment,
) (*RunPaths, error) {
	paths := &RunPaths{
		RunDir: filepath.Join(s.Root, runID),
	}

	if err := copyFile(manifestPath, filepath.Join(paths.RunDir, "manifest.yaml")); err != nil {
		return nil, fmt.Errorf("write frozen manifest: %w", err)
	}

	if err := writeJSON(filepath.Join(paths.RunDir, "plan.json"), p); err != nil {
		return nil, fmt.Errorf("write plan.json: %w", err)
	}

	if err := writeJSON(filepath.Join(paths.RunDir, "environment.json"), env); err != nil {
		return nil, fmt.Errorf("write environment.json: %w", err)
	}

	return paths, nil
}

func (s *Store) WriteRunResult(result *orchestrator.RunResult) error {
	runDir := filepath.Join(s.Root, result.RunID)

	if err := writeJSON(filepath.Join(runDir, "run.json"), result); err != nil {
		return fmt.Errorf("write run.json: %w", err)
	}

	return nil
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}

	data = append(data, '\n')

	return writeFileAtomic(path, data, 0o644)
}

func copyFile(src string, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	f, err := createAtomic(dst, 0o644)
	if err != nil {
		return err
	}

	if _, err := io.Copy(f.file, in); err != nil {
		_ = f.abort()
		return err
	}

	return f.commit()
}

func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	f, err := createAtomic(path, perm)
	if err != nil {
		return err
	}

	if _, err := f.file.Write(data); err != nil {
		_ = f.abort()
		return err
	}

	return f.commit()
}

type atomicFile struct {
	path string
	tmp  string
	file *os.File
	perm os.FileMode
}

func createAtomic(path string, perm os.FileMode) (*atomicFile, error) {
	dir := filepath.Dir(path)

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}

	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return nil, err
	}

	return &atomicFile{
		path: path,
		tmp:  tmp.Name(),
		file: tmp,
		perm: perm,
	}, nil
}

func (f *atomicFile) commit() error {
	if err := f.file.Close(); err != nil {
		_ = os.Remove(f.tmp)
		return err
	}

	if err := os.Chmod(f.tmp, f.perm); err != nil {
		_ = os.Remove(f.tmp)
		return err
	}

	return os.Rename(f.tmp, f.path)
}

func (f *atomicFile) abort() error {
	_ = f.file.Close()
	return os.Remove(f.tmp)
}
