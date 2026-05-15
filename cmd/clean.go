package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/cx009/tgconn/internal/storage"
)

var cleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Manually clean stored data (tmp, logs, history)",
	Long: `Manually delete files from the per-project storage directory at
~/.tgconn/projects/<encoded-cwd>/.

At least one of --tmp, --logs, --history, or --all must be specified.
--history and --all prompt for confirmation unless --yes or --dry-run is set.`,
	RunE: runClean,
}

func init() {
	cleanCmd.Flags().Bool("tmp", false, "delete all files under tmp/")
	cleanCmd.Flags().Bool("logs", false, "delete all daily *.jsonl logs (history files excluded)")
	cleanCmd.Flags().Bool("history", false, "delete all history_<chatID>.jsonl files")
	cleanCmd.Flags().Bool("all", false, "delete tmp + logs + history")
	cleanCmd.Flags().Bool("dry-run", false, "list files that would be deleted, do nothing")
	cleanCmd.Flags().Bool("yes", false, "skip interactive confirmation")
	rootCmd.AddCommand(cleanCmd)
}

type cleanCandidate struct {
	path string
	size int64
}

func runClean(cmd *cobra.Command, _ []string) error {
	doTmp, _ := cmd.Flags().GetBool("tmp")
	doLogs, _ := cmd.Flags().GetBool("logs")
	doHist, _ := cmd.Flags().GetBool("history")
	doAll, _ := cmd.Flags().GetBool("all")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	skipConfirm, _ := cmd.Flags().GetBool("yes")

	if doAll {
		doTmp, doLogs, doHist = true, true, true
	}
	if !doTmp && !doLogs && !doHist {
		return errors.New("must specify at least one of --tmp, --logs, --history, or --all")
	}

	resolver, err := storage.NewResolver()
	if err != nil {
		return fmt.Errorf("resolve storage path: %w", err)
	}
	projectDir := resolver.ProjectDir()

	if _, err := os.Stat(projectDir); errors.Is(err, fs.ErrNotExist) {
		fmt.Fprintf(cmd.OutOrStdout(), "No storage directory yet at %s — nothing to clean.\n", projectDir)
		return nil
	}

	var tmpFiles, logFiles, histFiles []cleanCandidate
	if doTmp {
		tmpFiles = collectAllFiles(filepath.Join(projectDir, "tmp"))
	}
	if doLogs {
		logFiles = collectDailyLogs(projectDir)
	}
	if doHist {
		histFiles = collectHistoryFiles(projectDir)
	}

	total := len(tmpFiles) + len(logFiles) + len(histFiles)
	if total == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "Nothing to clean.")
		return nil
	}

	printGroup(cmd.OutOrStdout(), "tmp", tmpFiles)
	printGroup(cmd.OutOrStdout(), "logs", logFiles)
	printGroup(cmd.OutOrStdout(), "history", histFiles)

	totalBytes := sumBytes(tmpFiles) + sumBytes(logFiles) + sumBytes(histFiles)
	fmt.Fprintf(cmd.OutOrStdout(), "\nTotal: %d files, %s\n", total, humanBytes(totalBytes))

	if dryRun {
		fmt.Fprintln(cmd.OutOrStdout(), "(dry-run — no files deleted)")
		return nil
	}

	needsConfirm := (doHist || doAll) && !skipConfirm
	if needsConfirm {
		ok, cerr := confirm(cmd.InOrStdin(), cmd.OutOrStdout(),
			"This will permanently delete the files listed above. Type 'yes' to proceed: ")
		if cerr != nil {
			return fmt.Errorf("read confirmation: %w", cerr)
		}
		if !ok {
			fmt.Fprintln(cmd.OutOrStdout(), "Aborted.")
			return nil
		}
	}

	var removed int
	var freed int64
	for _, group := range [][]cleanCandidate{tmpFiles, logFiles, histFiles} {
		for _, c := range group {
			if err := os.Remove(c.path); err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "skip %s: %v\n", c.path, err)
				continue
			}
			removed++
			freed += c.size
		}
	}

	if doTmp {
		removeEmptyTmpSubdirs(filepath.Join(projectDir, "tmp"))
	}

	fmt.Fprintf(cmd.OutOrStdout(), "\n✅ Removed %d files, freed %s.\n", removed, humanBytes(freed))
	return nil
}

func collectAllFiles(dir string) []cleanCandidate {
	var out []cleanCandidate
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		fi, ferr := d.Info()
		if ferr != nil {
			return nil
		}
		out = append(out, cleanCandidate{path: path, size: fi.Size()})
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].path < out[j].path })
	return out
}

func collectDailyLogs(projectDir string) []cleanCandidate {
	var out []cleanCandidate
	for _, dir := range []string{projectDir, filepath.Join(projectDir, "cron")} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if !strings.HasSuffix(name, ".jsonl") || strings.HasPrefix(name, "history_") {
				continue
			}
			fi, ferr := e.Info()
			if ferr != nil {
				continue
			}
			out = append(out, cleanCandidate{path: filepath.Join(dir, name), size: fi.Size()})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].path < out[j].path })
	return out
}

func collectHistoryFiles(projectDir string) []cleanCandidate {
	entries, err := os.ReadDir(projectDir)
	if err != nil {
		return nil
	}
	var out []cleanCandidate
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, "history_") || !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		fi, ferr := e.Info()
		if ferr != nil {
			continue
		}
		out = append(out, cleanCandidate{path: filepath.Join(projectDir, name), size: fi.Size()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].path < out[j].path })
	return out
}

func removeEmptyTmpSubdirs(tmpDir string) {
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		sub := filepath.Join(tmpDir, e.Name())
		children, rerr := os.ReadDir(sub)
		if rerr != nil || len(children) > 0 {
			continue
		}
		_ = os.Remove(sub)
	}
}

func printGroup(w io.Writer, label string, items []cleanCandidate) {
	if len(items) == 0 {
		return
	}
	fmt.Fprintf(w, "\n%s (%d files):\n", label, len(items))
	for _, c := range items {
		fmt.Fprintf(w, "  %s  (%s)\n", c.path, humanBytes(c.size))
	}
}

func sumBytes(items []cleanCandidate) int64 {
	var n int64
	for _, c := range items {
		n += c.size
	}
	return n
}

func humanBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}

func confirm(in io.Reader, out io.Writer, prompt string) (bool, error) {
	fmt.Fprint(out, prompt)
	reader := bufio.NewReader(in)
	line, err := reader.ReadString('\n')
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(line) == "yes", nil
}
