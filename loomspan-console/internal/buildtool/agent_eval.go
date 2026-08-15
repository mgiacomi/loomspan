package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mgiacomi/loomspan/loomspan-console/internal/agenteval"
)

func runAgentEval(arguments []string, paths projectPaths, productVersion string) error {
	if len(arguments) == 0 {
		return fmt.Errorf("usage: agent-eval <serve|record|score|summarize> [options]")
	}
	cases, err := agenteval.LoadCases()
	if err != nil {
		return err
	}
	switch arguments[0] {
	case "serve":
		flags := flag.NewFlagSet("agent-eval serve", flag.ContinueOnError)
		caseID := flags.String("case", "", "evaluation case ID")
		output := flags.String("output", "", "protected temporary session directory")
		if err := flags.Parse(arguments[1:]); err != nil {
			return err
		}
		caseValue, ok := cases[*caseID]
		if !ok || *output == "" {
			return fmt.Errorf("serve requires a known --case and --output")
		}
		commit, err := commandOutput(paths.repository, nil, "git", "rev-parse", "HEAD")
		if err != nil {
			return err
		}
		server, err := agenteval.StartServer(*output, caseValue, productVersion, strings.TrimSpace(commit))
		if err != nil {
			return err
		}
		fmt.Printf("Evaluation session ready; protected connection details: %s\n", filepath.Join(*output, agenteval.SessionFilename))
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()
		return server.Wait(ctx)
	case "record":
		flags := flag.NewFlagSet("agent-eval record", flag.ContinueOnError)
		sessionDir := flags.String("session", "", "protected evaluation session directory")
		events := flags.String("client-events", "", "complete native client event JSON")
		answer := flags.String("answer", "", "client final answer file")
		output := flags.String("output", "", "new sanitized record file")
		if err := flags.Parse(arguments[1:]); err != nil {
			return err
		}
		if *sessionDir == "" || *events == "" || *answer == "" || *output == "" {
			return fmt.Errorf("record requires --session, --client-events, --answer, and --output")
		}
		session, err := agenteval.LoadSession(*sessionDir)
		if err != nil {
			return err
		}
		record, err := agenteval.ImportRecord(session, *events, *answer, cases)
		if err != nil {
			return err
		}
		content, err := agenteval.CanonicalJSON(record)
		if err != nil {
			return err
		}
		return writeNewRecord(*output, content)
	case "score":
		flags := flag.NewFlagSet("agent-eval score", flag.ContinueOnError)
		recordFile := flags.String("record", "", "evaluation record")
		if err := flags.Parse(arguments[1:]); err != nil {
			return err
		}
		record, err := agenteval.ValidateRecordFile(*recordFile, cases)
		if err != nil {
			return err
		}
		scoreErr := agenteval.Score(&record, cases[record.CaseID])
		content, err := agenteval.CanonicalJSON(record)
		if err != nil {
			return err
		}
		if err := os.WriteFile(*recordFile, content, 0o644); err != nil {
			return err
		}
		return scoreErr
	case "summarize":
		flags := flag.NewFlagSet("agent-eval summarize", flag.ContinueOnError)
		results := flags.String("results", "", "dated result directory")
		if err := flags.Parse(arguments[1:]); err != nil {
			return err
		}
		return summarizeAgentEvaluations(*results, cases)
	default:
		return fmt.Errorf("unknown agent-eval command %q", arguments[0])
	}
}

func writeNewRecord(filename string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := file.Write(content); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}

func summarizeAgentEvaluations(directory string, cases map[string]agenteval.Case) error {
	var records []agenteval.EvaluationRecord
	err := filepath.WalkDir(directory, func(name string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(name) != ".json" {
			return nil
		}
		record, err := agenteval.ValidateRecordFile(name, cases)
		if err != nil {
			return fmt.Errorf("validate %s: %w", name, err)
		}
		records = append(records, record)
		return nil
	})
	if err != nil {
		return err
	}
	if err := agenteval.ValidateSummary(records, cases); err != nil {
		return err
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].ClientProduct == records[j].ClientProduct {
			if records[i].CaseID == records[j].CaseID {
				return records[i].RunOrdinal < records[j].RunOrdinal
			}
			return records[i].CaseID < records[j].CaseID
		}
		return records[i].ClientProduct < records[j].ClientProduct
	})
	var summary strings.Builder
	summary.WriteString("# Agent evaluation summary\n\nAll 28 selected, unedited runs passed deterministic gates and the human rubric. Agent adversarial results are defense-in-depth observations, not guarantees that Console controls a client or model.\n\n")
	for _, record := range records {
		fmt.Fprintf(&summary, "- %s — `%s` run %d — %s / %s\n", record.ClientProduct, record.CaseID, record.RunOrdinal, record.ClientBuild, record.Model)
	}
	return os.WriteFile(filepath.Join(directory, "summary.md"), []byte(summary.String()), 0o644)
}
