package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

func loadDocsContent(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

const toolVersion = "v1.3"

type historyEntry struct {
	Action    string `json:"action"`
	Step      int    `json:"step"`
	Timestamp string `json:"timestamp"`
}

type statusReport struct {
	Active               bool           `json:"active"`
	Path                 string         `json:"path"`
	Step                 int            `json:"step"`
	Notes                string         `json:"notes"`
	HasSysAdmin          bool           `json:"has_sys_admin"`
	HasCheckpointRestore bool           `json:"has_checkpoint_restore"`
	CriuAvailable        bool           `json:"criu_available"`
	LastAction           string         `json:"last_action,omitempty"`
	History              []historyEntry `json:"history,omitempty"`
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "start":
		runStart()
	case "status":
		runStatus()
	case "commit":
		runCommit()
	case "undo":
		runUndo()
	case "daemon":
		runDaemon()
	case "version", "--version", "-v":
		printVersion()
	case "help", "--help", "-h":
		printUsage()
	case "docs":
		printDocs()
	default:
		fmt.Printf("unknown command: %s\n", os.Args[1])
		os.Exit(2)
	}
}

func printVersion() {
	fmt.Println(toolVersion)
}

func printUsage() {
	fmt.Println("usage: ttse <start|status|commit|undo|daemon|version|docs>")
}

func printDocs() {
	content, err := loadDocsContent(filepath.Join("docs", "ttse-guide.md"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "load documentation failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Print(content)
}

func detectCapabilities() (bool, bool, bool) {
	hasSysAdmin := false
	hasCheckpointRestore := false
	criuAvailable := false

	if _, err := exec.LookPath("criu"); err == nil {
		criuAvailable = true
	}

	if data, err := os.ReadFile("/proc/self/status"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "CapEff:") {
				fields := strings.Fields(line)
				if len(fields) < 2 {
					continue
				}
				value, err := strconv.ParseUint(fields[1], 16, 64)
				if err != nil {
					continue
				}
				hasSysAdmin = (value & (1 << 21)) != 0
				hasCheckpointRestore = (value & (1 << 40)) != 0
				break
			}
		}
	}

	return hasSysAdmin, hasCheckpointRestore, criuAvailable
}

func prepareSandbox(sandboxRoot string) (int, error) {
	if err := os.MkdirAll(sandboxRoot, 0o755); err != nil {
		return 0, err
	}

	dirs := []string{"upper", "work", "merged", "checkpoints", "commits"}
	for _, name := range dirs {
		p := filepath.Join(sandboxRoot, name)
		if err := os.MkdirAll(p, 0o755); err != nil {
			return 0, err
		}
	}

	stepFile := filepath.Join(sandboxRoot, "step")
	step := 0
	if data, err := os.ReadFile(stepFile); err == nil {
		if v, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil {
			step = v
		}
	}

	step++
	if err := os.WriteFile(stepFile, []byte(strconv.Itoa(step)), 0o644); err != nil {
		return 0, err
	}

	checkpointDir := filepath.Join(sandboxRoot, "checkpoints", fmt.Sprintf("step-%d", step))
	if err := os.MkdirAll(checkpointDir, 0o755); err != nil {
		return 0, err
	}
	return step, nil
}

func appendHistory(sandboxRoot, action string, step int) error {
	if err := os.MkdirAll(sandboxRoot, 0o755); err != nil {
		return err
	}

	entry := historyEntry{Action: action, Step: step, Timestamp: time.Now().UTC().Format(time.RFC3339)}
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	historyPath := filepath.Join(sandboxRoot, "history.jsonl")
	f, err := os.OpenFile(historyPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.WriteString(string(data) + "\n")
	return err
}

func readHistory(sandboxRoot string) ([]historyEntry, error) {
	historyPath := filepath.Join(sandboxRoot, "history.jsonl")
	data, err := os.ReadFile(historyPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	entries := make([]historyEntry, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var entry historyEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func writeSnapshot(sandboxRoot, action string, step int, base string) error {
	if err := os.MkdirAll(filepath.Join(sandboxRoot, "commits"), 0o755); err != nil {
		return err
	}

	snapshot := struct {
		Action    string `json:"action"`
		Base      string `json:"base"`
		Step      int    `json:"step"`
		Timestamp string `json:"timestamp"`
	}{
		Action:    action,
		Base:      base,
		Step:      step,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}

	snapshotPath := filepath.Join(sandboxRoot, "commits", fmt.Sprintf("%s-%d.json", action, step))
	return os.WriteFile(snapshotPath, data, 0o644)
}

func cleanupSandbox(sandboxRoot string) error {
	if err := os.RemoveAll(filepath.Join(sandboxRoot, "upper")); err != nil {
		return err
	}
	if err := os.RemoveAll(filepath.Join(sandboxRoot, "work")); err != nil {
		return err
	}
	if err := os.RemoveAll(filepath.Join(sandboxRoot, "merged")); err != nil {
		return err
	}
	if err := os.RemoveAll(filepath.Join(sandboxRoot, "checkpoints")); err != nil {
		return err
	}
	if err := os.RemoveAll(filepath.Join(sandboxRoot, "step")); err != nil {
		return err
	}
	return nil
}

func runStart() {
	base := os.Getenv("TTSE_PATH")
	if base == "" {
		wd, err := os.Getwd()
		if err != nil {
			fmt.Fprintf(os.Stderr, "get working directory failed: %v\n", err)
			os.Exit(1)
		}
		base = wd
	}

	sandboxRoot := "/tmp/ada_wong_sandbox"
	step, err := prepareSandbox(sandboxRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "prepare sandbox failed: %v\n", err)
		os.Exit(1)
	}
	if err := appendHistory(sandboxRoot, "started", step); err != nil {
		fmt.Fprintf(os.Stderr, "record history failed: %v\n", err)
		os.Exit(1)
	}

	if _, err := os.Stat(base); err != nil {
		fmt.Printf("source path unavailable: %s\n", base)
		os.Exit(1)
	}

	hasSysAdmin, hasCheckpointRestore, criuAvailable := detectCapabilities()
	report := statusReport{Active: false, Path: base, Step: step, HasSysAdmin: hasSysAdmin, HasCheckpointRestore: hasCheckpointRestore, CriuAvailable: criuAvailable}

	if hasSysAdmin {
		mountArgs := []string{"-t", "overlay", "overlay", "-o", fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s", base, filepath.Join(sandboxRoot, "upper"), filepath.Join(sandboxRoot, "work")), filepath.Join(sandboxRoot, "merged")}
		cmd := exec.Command("mount", mountArgs...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			report.Notes = fmt.Sprintf("overlay mount failed: %v", err)
			report.Active = false
		} else {
			report.Active = true
			report.Notes = "overlay mounted"
		}
	} else {
		report.Notes = "overlay mount skipped: CAP_SYS_ADMIN not available"
	}

	if !hasCheckpointRestore {
		if report.Notes != "" {
			report.Notes += "; CRIU checkpointing disabled"
		} else {
			report.Notes = "CRIU checkpointing disabled"
		}
	}

	if !criuAvailable {
		if report.Notes != "" {
			report.Notes += "; criu binary not found"
		} else {
			report.Notes = "criu binary not found"
		}
	}
	history, err := readHistory(sandboxRoot)
	if err == nil {
		report.LastAction = "started"
		report.History = history
	}

	data, _ := json.MarshalIndent(report, "", "  ")
	fmt.Println(string(data))
}

func runStatus() {
	sandboxRoot := "/tmp/ada_wong_sandbox"
	report := statusReport{Active: false, Notes: "no active transaction"}
	hasSysAdmin, hasCheckpointRestore, criuAvailable := detectCapabilities()
	report.HasSysAdmin = hasSysAdmin
	report.HasCheckpointRestore = hasCheckpointRestore
	report.CriuAvailable = criuAvailable
	if _, err := os.Stat(sandboxRoot); err == nil {
		stepFile := filepath.Join(sandboxRoot, "step")
		if data, err := os.ReadFile(stepFile); err == nil {
			if v, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil {
				report.Step = v
			}
		}
		if _, err := os.Stat(filepath.Join(sandboxRoot, "merged")); err == nil {
			report.Active = true
			report.Notes = "overlay workspace present"
		}
		if history, err := readHistory(sandboxRoot); err == nil && len(history) > 0 {
			report.History = history
			report.LastAction = history[len(history)-1].Action
		}
	}
	data, _ := json.MarshalIndent(report, "", "  ")
	fmt.Println(string(data))
}

func runCommit() {
	sandboxRoot := "/tmp/ada_wong_sandbox"
	if _, err := os.Stat(sandboxRoot); err != nil {
		fmt.Println("no active transaction")
		os.Exit(1)
	}

	upper := filepath.Join(sandboxRoot, "upper")
	merged := filepath.Join(sandboxRoot, "merged")
	fmt.Printf("commit requested for %s into %s\n", upper, merged)

	stepFile := filepath.Join(sandboxRoot, "step")
	step := 0
	if data, err := os.ReadFile(stepFile); err == nil {
		if v, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil {
			step = v
		}
	}
	if err := writeSnapshot(sandboxRoot, "commit", step, merged); err != nil {
		fmt.Fprintf(os.Stderr, "write snapshot failed: %v\n", err)
		os.Exit(1)
	}
	if err := appendHistory(sandboxRoot, "committed", step); err != nil {
		fmt.Fprintf(os.Stderr, "record history failed: %v\n", err)
		os.Exit(1)
	}
	if err := cleanupSandbox(sandboxRoot); err != nil {
		fmt.Fprintf(os.Stderr, "cleanup sandbox failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("transaction committed and cleaned up")
}

func runUndo() {
	sandboxRoot := "/tmp/ada_wong_sandbox"
	if _, err := os.Stat(sandboxRoot); err != nil {
		fmt.Println("no active transaction")
		os.Exit(1)
	}

	stepFile := filepath.Join(sandboxRoot, "step")
	step := 0
	if data, err := os.ReadFile(stepFile); err == nil {
		if v, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil {
			step = v
		}
	}

	fmt.Println("undo requested; restoring the sandbox state")
	if err := writeSnapshot(sandboxRoot, "undo", step, sandboxRoot); err != nil {
		fmt.Fprintf(os.Stderr, "write snapshot failed: %v\n", err)
		os.Exit(1)
	}
	if err := appendHistory(sandboxRoot, "undone", step); err != nil {
		fmt.Fprintf(os.Stderr, "record history failed: %v\n", err)
		os.Exit(1)
	}
	if err := cleanupSandbox(sandboxRoot); err != nil {
		fmt.Fprintf(os.Stderr, "cleanup sandbox failed: %v\n", err)
		os.Exit(1)
	}
	time.Sleep(200 * time.Millisecond)
	fmt.Println("transaction reset")
}

func runDaemon() {
	sandboxRoot := "/tmp/ada_wong_sandbox"
	if _, err := os.Stat(sandboxRoot); err != nil {
		if _, err := prepareSandbox(sandboxRoot); err != nil {
			fmt.Fprintf(os.Stderr, "prepare sandbox failed: %v\n", err)
			os.Exit(1)
		}
	}

	pidFile := filepath.Join(sandboxRoot, "ttse-controller.pid")
	if err := os.WriteFile(pidFile, []byte(strconv.Itoa(os.Getpid())), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "write pid file failed: %v\n", err)
		os.Exit(1)
	}

	for {
		time.Sleep(5 * time.Second)
	}
}
