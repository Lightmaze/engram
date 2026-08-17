package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Lightmaze/engram/internal/engram"
	"github.com/Lightmaze/engram/internal/server"
)

const maxInput = 16 * 1024 * 1024

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

type options struct {
	data      string
	driver    string
	endpoint  string
	model     string
	maxOutput int
	allowRule bool
}

func defaults() options {
	return options{
		data:      environmentOr("ENGRAM_DATA", ".engram"),
		driver:    environmentOr("ENGRAM_DRIVER", "openai-responses"),
		endpoint:  os.Getenv("ENGRAM_ENDPOINT"),
		model:     os.Getenv("ENGRAM_MODEL"),
		maxOutput: 4096,
	}
}

func (options *options) bind(flags *flag.FlagSet) {
	flags.StringVar(&options.data, "data", options.data, "Engram Journal directory")
	flags.StringVar(&options.driver, "driver", options.driver, "openai-responses, deepseek-chat, or rule")
	flags.StringVar(&options.endpoint, "endpoint", options.endpoint, "optional provider endpoint")
	flags.StringVar(&options.model, "model", options.model, "Engram model id")
	flags.IntVar(&options.maxOutput, "max-output-tokens", options.maxOutput, "response ceiling")
	flags.BoolVar(&options.allowRule, "allow-rule-driver", false, "allow deterministic test driver")
}

func run(args []string, input io.Reader, output, diagnostics io.Writer) int {
	if len(args) == 0 {
		usage(diagnostics)
		return 2
	}

	var err error
	switch args[0] {
	case "version", "--version", "-v":
		fmt.Fprintln(output, engram.Version)
		return 0
	case "help", "--help", "-h":
		usage(output)
		return 0
	case "create":
		err = runCreate(args[1:], input, output, diagnostics)
	case "list":
		err = runList(args[1:], output, diagnostics)
	case "invoke":
		err = runInvoke(args[1:], input, output, diagnostics)
	case "mcp":
		err = runMCP(args[1:], input, output, diagnostics)
	case "hook":
		err = runHook(args[1:], input, output, diagnostics)
	default:
		fmt.Fprintf(diagnostics, "unknown command %q\n", args[0])
		usage(diagnostics)
		return 2
	}
	if err != nil {
		fmt.Fprintln(diagnostics, err)
		return 1
	}
	return 0
}

func parse(name string, args []string, diagnostics io.Writer, extra func(*flag.FlagSet)) (options, error) {
	options := defaults()
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(diagnostics)
	options.bind(flags)
	if extra != nil {
		extra(flags)
	}
	if err := flags.Parse(args); err != nil {
		return options, err
	}
	if flags.NArg() != 0 {
		return options, fmt.Errorf("unexpected arguments: %s", strings.Join(flags.Args(), " "))
	}
	return options, nil
}

func openJournal(options options) (*engram.Journal, error) {
	return engram.OpenJournal(options.data)
}

func openRuntime(options options) (*engram.Runtime, *engram.Journal, error) {
	journal, err := openJournal(options)
	if err != nil {
		return nil, nil, err
	}
	apiKey := ""
	switch options.driver {
	case "openai-responses":
		apiKey = os.Getenv("OPENAI_API_KEY")
	case "deepseek-chat":
		apiKey = os.Getenv("DEEPSEEK_API_KEY")
	}
	provider, err := engram.NewProvider(engram.ProviderConfig{
		Driver:          options.driver,
		Endpoint:        options.endpoint,
		Model:           options.model,
		APIKey:          apiKey,
		MaxOutputTokens: options.maxOutput,
		AllowRule:       options.allowRule,
	})
	if err != nil {
		return nil, nil, err
	}
	runtime, err := engram.NewRuntime(journal, provider)
	return runtime, journal, err
}

func runCreate(args []string, input io.Reader, output, diagnostics io.Writer) error {
	fileName := "-"
	options, err := parse("create", args, diagnostics, func(flags *flag.FlagSet) {
		flags.StringVar(&fileName, "file", "-", "JSON import file or -")
	})
	if err != nil {
		return err
	}
	reader := input
	if fileName != "-" {
		file, err := os.Open(fileName)
		if err != nil {
			return err
		}
		defer file.Close()
		reader = file
	}
	var request engram.CreateRequest
	if err := decode(reader, &request); err != nil {
		return err
	}
	journal, err := openJournal(options)
	if err != nil {
		return err
	}
	value, err := journal.Create(request)
	if err != nil {
		return err
	}
	return writeJSON(output, engram.Summary{
		ProtocolVersion: engram.ProtocolVersion,
		ID:              value.ID,
		Name:            value.Name,
		CreatedAt:       value.CreatedAt,
	})
}

func runList(args []string, output, diagnostics io.Writer) error {
	options, err := parse("list", args, diagnostics, nil)
	if err != nil {
		return err
	}
	journal, err := openJournal(options)
	if err != nil {
		return err
	}
	value, err := journal.List()
	if err != nil {
		return err
	}
	return writeJSON(output, value)
}

func runInvoke(args []string, input io.Reader, output, diagnostics io.Writer) error {
	options, err := parse("invoke", args, diagnostics, nil)
	if err != nil {
		return err
	}
	runtime, journal, err := openRuntime(options)
	if err != nil {
		return err
	}
	var call struct {
		Name   string          `json:"method"`
		Params json.RawMessage `json:"params"`
	}
	if err := decode(input, &call); err != nil {
		return err
	}
	value, err := (server.MCP{Runtime: runtime, Journal: journal}).CallTool(context.Background(), call.Name, call.Params)
	if err != nil {
		return err
	}
	return writeJSON(output, value)
}

func runMCP(args []string, input io.Reader, output, diagnostics io.Writer) error {
	options, err := parse("mcp", args, diagnostics, nil)
	if err != nil {
		return err
	}
	runtime, journal, err := openRuntime(options)
	if err != nil {
		return err
	}
	return (server.MCP{Runtime: runtime, Journal: journal}).Serve(context.Background(), input, output)
}

func runHook(args []string, input io.Reader, output, diagnostics io.Writer) error {
	host := ""
	engramID := ""
	idleSeconds := int64(1800)
	failClosed := false
	options, err := parse("hook", args, diagnostics, func(flags *flag.FlagSet) {
		flags.StringVar(&host, "host", "", "codex, claude-code, cursor, pi, or generic")
		flags.StringVar(&engramID, "engram", os.Getenv("ENGRAM_ID"), "guardian Engram id")
		flags.Int64Var(&idleSeconds, "idle-seconds", 1800, "guardian idle timeout")
		flags.BoolVar(&failClosed, "fail-closed", false, "fail host turn when guardian fails")
	})
	if err != nil {
		return err
	}
	raw, err := readAll(input)
	if err != nil {
		return err
	}
	runtime, _, runtimeErr := openRuntime(options)
	if runtimeErr == nil {
		value, hookErr := (server.Hook{Runtime: runtime, EngramID: engramID, IdleSeconds: idleSeconds}).Handle(context.Background(), host, raw)
		if hookErr == nil {
			_, err = output.Write(value)
			return err
		}
		runtimeErr = hookErr
	}
	if failClosed {
		return runtimeErr
	}
	fmt.Fprintf(diagnostics, "Engram guardian unavailable; host continues: %v\n", runtimeErr)
	if host == "cursor" {
		_, err = output.Write([]byte(`{"continue":true}`))
	} else {
		_, err = output.Write([]byte(`{}`))
	}
	return err
}

func decode(reader io.Reader, target any) error {
	raw, err := readAll(reader)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("input must contain one JSON value")
	}
	return nil
}

func readAll(reader io.Reader) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(reader, maxInput+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > maxInput {
		return nil, errors.New("input exceeds 16 MiB")
	}
	return raw, nil
}

func writeJSON(writer io.Writer, value any) error {
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func environmentOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func usage(writer io.Writer) {
	fmt.Fprintln(writer, "usage: engram <create|list|mcp|invoke|hook|version>")
	fmt.Fprintln(writer, "MCP summon is explicit and multi-round; Hook guardian is automatic and advisory.")
}
