package acpcmd

import (
	"context"
	"io"
	"strings"

	"github.com/spf13/cobra"
)

type GeminiOptions struct {
	Prompt     string
	Model      string
	GeminiBin  string
	GeminiArgs []string
}

func GeminiCommand() *cobra.Command {
	opts := GeminiOptions{GeminiBin: "gemini"}
	return newModelACPRunCommand(modelACPCommandConfig{
		Use:        "gemini",
		Short:      "Run Gemini CLI in ACP mode through Go ADK",
		InfoShort:  "Inspect Gemini ACP agent capabilities and auth methods",
		Prompt:     &opts.Prompt,
		Model:      &opts.Model,
		Binary:     &opts.GeminiBin,
		Args:       &opts.GeminiArgs,
		ModelHelp:  "Gemini model name",
		BinaryFlag: "gemini-bin",
		BinaryHelp: "Gemini executable path",
		ArgsFlag:   "gemini-arg",
		ArgsHelp:   "extra Gemini CLI argument (repeatable)",
		Run: func(ctx context.Context, workingDir string, stdin io.Reader, stdout, stderr io.Writer) error {
			return RunGeminiACP(ctx, workingDir, opts, stdin, stdout, stderr)
		},
	})
}

func GeminiInfoCommand() *cobra.Command {
	opts := GeminiOptions{GeminiBin: "gemini"}
	return newModelACPInfoCommand(modelACPCommandConfig{
		Use:        "gemini",
		Short:      "Run Gemini CLI in ACP mode through Go ADK",
		InfoShort:  "Inspect Gemini ACP agent capabilities and auth methods",
		Prompt:     &opts.Prompt,
		Model:      &opts.Model,
		Binary:     &opts.GeminiBin,
		Args:       &opts.GeminiArgs,
		ModelHelp:  "Gemini model name",
		BinaryFlag: "gemini-bin",
		BinaryHelp: "Gemini executable path",
		ArgsFlag:   "gemini-arg",
		ArgsHelp:   "extra Gemini CLI argument (repeatable)",
		RunInfo: func(ctx context.Context, workingDir string, jsonOutput bool, stdout io.Writer, stderr io.Writer) error {
			return RunGeminiACPInfo(ctx, workingDir, opts, jsonOutput, stdout, stderr)
		},
	})
}

func GeminiWebCommand() *cobra.Command {
	opts := GeminiOptions{GeminiBin: "gemini"}
	return newACPWebCommand(
		"gemini [-- <web launcher args...>]",
		"Run Gemini ACP with the ADK web launcher",
		func(cmd *cobra.Command) {
			cmd.Flags().StringVar(&opts.Model, "model", "", "Gemini model name")
			cmd.Flags().StringVar(&opts.GeminiBin, "gemini-bin", opts.GeminiBin, "Gemini executable path")
			cmd.Flags().StringArrayVar(&opts.GeminiArgs, "gemini-arg", nil, "extra Gemini CLI argument (repeatable)")
		},
		func(ctx context.Context, workingDir string, launcherArgs []string, stderr io.Writer) error {
			return RunGeminiACPWeb(ctx, workingDir, opts, launcherArgs, stderr)
		},
	)
}

func RunGeminiACP(ctx context.Context, workingDir string, opts GeminiOptions, stdin io.Reader, stdout, stderr io.Writer) error {
	return runStandardACP(ctx, workingDir, opts.Prompt, BuildGeminiACPCommand(opts), opts.Model, runtimeSpec{
		component:   "playground.gemini_acp",
		name:        "GeminiACP",
		description: "Gemini CLI playground agent via ACP",
		startMsg:    "starting Gemini ACP playground",
	}, stdin, stdout, stderr)
}

func BuildGeminiACPCommand(opts GeminiOptions) []string {
	cmd := []string{opts.GeminiBin, "--acp"}
	if strings.TrimSpace(opts.Model) != "" {
		cmd = append(cmd, "--model", opts.Model)
	}
	cmd = append(cmd, opts.GeminiArgs...)
	return cmd
}

func RunGeminiACPInfo(
	ctx context.Context,
	workingDir string,
	opts GeminiOptions,
	jsonOutput bool,
	stdout io.Writer,
	stderr io.Writer,
) error {
	return runACPInfo(
		ctx,
		workingDir,
		BuildGeminiACPCommand(opts),
		opts.Model,
		"playground.gemini_acp_info",
		"inspecting Gemini ACP agent",
		jsonOutput,
		stdout,
		stderr,
	)
}

func RunGeminiACPWeb(
	ctx context.Context,
	workingDir string,
	opts GeminiOptions,
	launcherArgs []string,
	stderr io.Writer,
) error {
	return runACPWeb(ctx, workingDir, BuildGeminiACPCommand(opts), opts.Model, runtimeSpec{
		component:   "playground.gemini_acp_web",
		name:        "GeminiACPWeb",
		description: "Gemini CLI playground agent via ACP (web launcher)",
		startMsg:    "starting Gemini ACP web launcher",
	}, launcherArgs, stderr)
}
