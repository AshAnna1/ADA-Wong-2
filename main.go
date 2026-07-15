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

const toolVersion = "v1.1"

type statusReport struct {
	Active               bool   `json:"active"`
	Path                 string `json:"path"`
	Step                 int    `json:"step"`
	Notes                string `json:"notes"`
	HasSysAdmin          bool   `json:"has_sys_admin"`
	HasCheckpointRestore bool   `json:"has_checkpoint_restore"`
	CriuAvailable        bool   `json:"criu_available"`
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
	default:
		fmt.Printf("unknown command: %s\n", os.Args[1])
		os.Exit(2)
	}
}

func printVersion() {
	fmt.Println(toolVersion)
}

func printUsage() {
	fmt.Println("usage: ttse <start|status|commit|undo|daemon|version>")
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

	dirs := []string{"upper", "work", "merged", "checkpoints"}
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
	if err := cleanupSandbox(sandboxRoot); err != nil {
		fmt.Fprintf(os.Stderr, "cleanup sandbox failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("transaction cleaned up")
}

func runUndo() {
	sandboxRoot := "/tmp/ada_wong_sandbox"
	if _, err := os.Stat(sandboxRoot); err != nil {
		fmt.Println("no active transaction")
		os.Exit(1)
	}

	fmt.Println("undo requested; restoring the sandbox state")
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
