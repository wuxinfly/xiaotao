package office

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

const maxReviewFiles = 128
const maxReviewFileBytes = 64 << 10

type FileChange struct {
	Path  string `json:"path"`
	Label string `json:"label"`
	Diff  string `json:"diff,omitempty"`
}

type fileSample struct {
	data            []byte
	mode            os.FileMode
	exists, unknown bool
}

type workspaceSample struct {
	dir, head, prefix string
	dirty             map[string]fileSample
}

type cappedBuffer struct {
	data  bytes.Buffer
	limit int
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	if len(p) > b.limit-b.data.Len() {
		return 0, errors.New("Git 输出超过验收上限")
	}
	return b.data.Write(p)
}

func reviewGit(ctx context.Context, dir string, limit int, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"--no-optional-locks", "-c", "core.fsmonitor=false"}, args...)...)
	cmd.Dir = dir
	hideCommandWindow(cmd)
	out := &cappedBuffer{limit: limit}
	cmd.Stdout = out
	err := cmd.Run()
	return out.data.Bytes(), err
}

func sampleFile(dir, name string) fileSample {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return fileSample{unknown: true}
	}
	defer root.Close()
	name = filepath.FromSlash(name)
	info, err := root.Lstat(name)
	if errors.Is(err, os.ErrNotExist) {
		return fileSample{}
	}
	if err != nil {
		return fileSample{unknown: true}
	}
	f := fileSample{exists: true, mode: info.Mode()}
	if !info.Mode().IsRegular() || info.Size() > maxReviewFileBytes {
		f.unknown = true
		return f
	}
	file, err := root.Open(name)
	if err != nil {
		f.unknown = true
		return f
	}
	defer file.Close()
	f.data, err = io.ReadAll(io.LimitReader(file, maxReviewFileBytes+1))
	f.unknown = err != nil || len(f.data) > maxReviewFileBytes
	return f
}

func captureWorkspace(ctx context.Context, dir string) (*workspaceSample, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	prefix, err := reviewGit(ctx, dir, 4096, "rev-parse", "--show-prefix")
	if err != nil {
		return nil, errors.New("无法读取 Git 工作区")
	}
	head, err := reviewGit(ctx, dir, 128, "rev-parse", "--verify", "HEAD")
	if err != nil {
		// New repositories have no HEAD yet. Git can name the empty tree
		// without writing objects or changing the user's index.
		head, err = reviewGit(ctx, dir, 128, "hash-object", "-t", "tree", "--stdin")
		if err != nil {
			return nil, errors.New("没有可比对的 Git 基线")
		}
	}
	snap := &workspaceSample{dir: dir, head: strings.TrimSpace(string(head)), prefix: strings.TrimSuffix(string(prefix), "\n"), dirty: map[string]fileSample{}}
	for _, args := range [][]string{{"ls-files", "-z", "-m", "-d", "-o", "--exclude-standard", "--", "."}, {"diff", "--cached", "--name-only", "--relative", "--no-renames", "-z", "--", "."}} {
		data, err := reviewGit(ctx, dir, 128<<10, args...)
		if err != nil {
			return nil, errors.New("无法完整读取工作区路径")
		}
		for _, path := range strings.Split(string(data), "\x00") {
			if path == "" {
				continue
			}
			if _, ok := snap.dirty[path]; ok {
				continue
			}
			if len(snap.dirty) >= maxReviewFiles {
				return nil, errors.New("工作区变更超过 128 个文件")
			}
			snap.dirty[path] = sampleFile(dir, path)
		}
	}
	return snap, nil
}

func (s *workspaceSample) original(ctx context.Context, path string) fileSample {
	if f, ok := s.dirty[path]; ok {
		return f
	}
	// Query the tree first: a missing path and an unreadable blob are different.
	entry, err := reviewGit(ctx, s.dir, 8192, "--literal-pathspecs", "ls-tree", "-z", s.head, "--", path)
	if err != nil {
		return fileSample{unknown: true}
	}
	if len(entry) == 0 {
		return fileSample{}
	}
	data, err := reviewGit(ctx, s.dir, maxReviewFileBytes, "cat-file", "blob", s.head+":"+s.prefix+path)
	return fileSample{data: data, exists: true, unknown: err != nil}
}

func compareWorkspace(ctx context.Context, before, after *workspaceSample) ([]FileChange, error) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	paths := map[string]bool{}
	for path := range before.dirty {
		paths[path] = true
	}
	for path := range after.dirty {
		paths[path] = true
	}
	if before.head != after.head {
		names, err := reviewGit(ctx, before.dir, 128<<10, "diff", "--name-only", "--relative", "--no-renames", "-z", before.head, after.head, "--", ".")
		if err != nil {
			return nil, errors.New("无法比对本轮提交")
		}
		for _, p := range strings.Split(string(names), "\x00") {
			if p != "" {
				paths[p] = true
			}
		}
	}
	if len(paths) > maxReviewFiles {
		return nil, errors.New("工作区变更超过 128 个文件")
	}
	var names []string
	for p := range paths {
		names = append(names, p)
	}
	slices.Sort(names)
	var changes []FileChange
	for _, p := range names {
		if ctx.Err() != nil {
			return changes, errors.New("文件比对超时，清单可能不完整")
		}
		a := before.original(ctx, p)
		b, ok := after.dirty[p]
		if !ok {
			b = sampleFile(after.dir, p)
		}
		_, wasDirty := before.dirty[p]
		item := FileChange{Path: p}
		switch {
		case a.unknown || b.unknown:
			item.Label = "内容待核对（大文件、特殊文件或读取受限）"
		case a.exists == b.exists && bytes.Equal(a.data, b.data) && (!wasDirty || a.mode == b.mode):
			if !wasDirty {
				item.Label = "Git 状态变化，文件内容相同"
			} else {
				item.Label = "开始前已有变更，内容未变"
			}
		default:
			switch {
			case !a.exists && b.exists:
				item.Label = "本轮新增"
			case a.exists && !b.exists:
				item.Label = "本轮删除"
			default:
				item.Label = "本轮修改"
			}
			item.Diff = reviewDiff(ctx, a.data, b.data)
		}
		if wasDirty && !strings.HasPrefix(item.Label, "开始前") {
			item.Label = "开始前已有变更 · " + item.Label
		}
		changes = append(changes, item)
	}
	return changes, nil
}

func reviewDiff(ctx context.Context, a, b []byte) string {
	if !utf8.Valid(a) || !utf8.Valid(b) || bytes.ContainsRune(a, 0) || bytes.ContainsRune(b, 0) {
		return "二进制文件"
	}
	dir, err := os.MkdirTemp("", "xiaotao-review-*")
	if err != nil {
		return "差异预览不可用"
	}
	defer os.RemoveAll(dir)
	for name, data := range map[string][]byte{"before": a, "after": b} {
		if os.WriteFile(filepath.Join(dir, name), data, 0600) != nil {
			return "差异预览不可用"
		}
	}
	out, err := reviewGit(ctx, dir, 256<<10, "diff", "--no-index", "--no-ext-diff", "--no-textconv", "--no-color", "--ignore-cr-at-eol", "--", "before", "after")
	var exit *exec.ExitError
	if err == nil {
		return ""
	}
	if !errors.As(err, &exit) || exit.ExitCode() != 1 {
		return "差异预览不可用"
	}
	_, body, ok := strings.Cut(string(out), "\n@@")
	if !ok {
		return ""
	}
	snippet := []rune("@@" + body)
	if len(snippet) > 2048 {
		return fmt.Sprintf("%s\n…（预览已截断）", string(snippet[:2048]))
	}
	return string(snippet)
}
