package acpcmd

import (
	"context"
	"io"

	"github.com/spf13/cobra"
)

type OpenCodeOptions struct {
	Prompt       string
	Model        string
	OpenCodeBin  string
	OpenCodeArgs []string
}

func OpenCodeCommand() *cobra.Command {
	opts := OpenCodeOptions{OpenCodeBin: "opencode"}
	return newModelACPRunCommand(modelACPCommandConfig{
		Use:        "opencode",
		Short:      "Run OpenCode CLI in ACP mode through Go ADK",
		InfoShort:  "Inspect OpenCode ACP agent capabilities and auth methods",
		Prompt:     &opts.Prompt,
		Model:      &opts.Model,
		Binary:     &opts.OpenCodeBin,
		Args:       &opts.OpenCodeArgs,
		ModelHelp:  "OpenCode model name",
		BinaryFlag: "opencode-bin",
		BinaryHelp: "OpenCode executable path",
		ArgsFlag:   "opencode-arg",
		ArgsHelp:   "extra OpenCode ACP argument (repeatable)",
		Run: func(ctx context.Context, workingDir string, stdin io.Reader, stdout, stderr io.Writer) error {
			return RunOpenCodeACP(ctx, workingDir, opts, stdin, stdout, stderr)
		},
	})
}

func OpenCodeInfoCommand() *cobra.Command {
	opts := OpenCodeOptions{OpenCodeBin: "opencode"}
	return newModelACPInfoCommand(modelACPCommandConfig{
		Use:        "opencode",
		Short:      "Run OpenCode CLI in ACP mode through Go ADK",
		InfoShort:  "Inspect OpenCode ACP agent capabilities and auth methods",
		Prompt:     &opts.Prompt,
		Model:      &opts.Model,
		Binary:     &opts.OpenCodeBin,
		Args:       &opts.OpenCodeArgs,
		ModelHelp:  "OpenCode model name",
		BinaryFlag: "opencode-bin",
		BinaryHelp: "OpenCode executable path",
		ArgsFlag:   "opencode-arg",
		ArgsHelp:   "extra OpenCode ACP argument (repeatable)",
		RunInfo: func(ctx context.Context, workingDir string, jsonOutput bool, stdout io.Writer, stderr io.Writer) error {
			return RunOpenCodeACPInfo(ctx, workingDir, opts, jsonOutput, stdout, stderr)
		},
	})
}

func OpenCodeWebCommand() *cobra.Command {
	opts := OpenCodeOptions{OpenCodeBin: "opencode"}
	return newACPWebCommand(
		"opencode [-- <web launcher args...>]",
		"Run OpenCode ACP with the ADK web launcher",
		func(cmd *cobra.Command) {
			cmd.Flags().StringVar(&opts.Model, "model", "", "OpenCode model name")
			cmd.Flags().StringVar(&opts.OpenCodeBin, "opencode-bin", opts.OpenCodeBin, "OpenCode executable path")
			cmd.Flags().StringArrayVar(&opts.OpenCodeArgs, "opencode-arg", nil, "extra OpenCode ACP argument (repeatable)")
		},
		func(ctx context.Context, workingDir string, launcherArgs []string, stderr io.Writer) error {
			return RunOpenCodeACPWeb(ctx, workingDir, opts, launcherArgs, stderr)
		},
	)
}

func RunOpenCodeACP(ctx context.Context, workingDir string, opts OpenCodeOptions, stdin io.Reader, stdout, stderr io.Writer) error {
	return runStandardACP(ctx, workingDir, opts.Prompt, BuildOpenCodeACPCommand(opts), opts.Model, runtimeSpec{
		component:   "playground.opencode_acp",
		name:        "OpenCodeACP",
		description: "OpenCode CLI playground agent via ACP",
		startMsg:    "starting OpenCode ACP playground",
	}, stdin, stdout, stderr)
}

func BuildOpenCodeACPCommand(opts OpenCodeOptions) []string {
	cmd := make([]string, 0, 2+len(opts.OpenCodeArgs))
	cmd = append(cmd, opts.OpenCodeBin, "acp")
	cmd = append(cmd, opts.OpenCodeArgs...)
	return cmd
}

func RunOpenCodeACPInfo(
	ctx context.Context,
	workingDir string,
	opts OpenCodeOptions,
	jsonOutput bool,
	stdout io.Writer,
	stderr io.Writer,
) error {
	return runACPInfo(
		ctx,
		workingDir,
		BuildOpenCodeACPCommand(opts),
		opts.Model,
		"playground.opencode_acp_info",
		"inspecting OpenCode ACP agent",
		jsonOutput,
		stdout,
		stderr,
	)
}

func RunOpenCodeACPWeb(
	ctx context.Context,
	workingDir string,
	opts OpenCodeOptions,
	launcherArgs []string,
	stderr io.Writer,
) error {
	return runACPWeb(ctx, workingDir, BuildOpenCodeACPCommand(opts), opts.Model, runtimeSpec{
		component:   "playground.opencode_acp_web",
		name:        "OpenCodeACPWeb",
		description: "OpenCode CLI playground agent via ACP (web launcher)",
		startMsg:    "starting OpenCode ACP web launcher",
	}, launcherArgs, stderr)
}
