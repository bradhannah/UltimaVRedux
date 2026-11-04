// Package main provides the semantic conversation extraction CLI.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/bradhannah/UltimaVRedux/internal/conversation/extractor"
	conversationworkspace "github.com/bradhannah/UltimaVRedux/internal/conversation/workspace"
	"github.com/spf13/cobra"
)

const (
	defaultDataDirectory  = "./OLD"
	defaultPatchDirectory = "data/conversation-patches"
)

func newRootCommand() *cobra.Command {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}

	return newRootCommandWithWorkspace(filepath.Join(home, ".ultima_v_redux", "conversations"))
}

func newRootCommandWithWorkspace(defaultWorkspace string) *cobra.Command {
	root := &cobra.Command{ //nolint:exhaustruct_v5 // Cobra command options are intentionally sparse.
		Use:           "conversation-tool",
		Short:         "Conversation data maintenance tools",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return command.Help()
		},
	}
	root.AddCommand(newExtractCommand(), newWorkspaceCommand(defaultWorkspace))

	return root
}

func newWorkspaceCommand(defaultWorkspace string) *cobra.Command {
	options := conversationworkspace.Options{DataDir: "", Workspace: "", PatchDir: ""}
	command := &cobra.Command{ //nolint:exhaustruct_v5 // Cobra command options are intentionally sparse.
		Use:   "workspace",
		Short: "Manage the private original-conversation editing workspace",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return command.Help()
		},
	}
	command.PersistentFlags().StringVar(
		&options.DataDir,
		"data-dir",
		defaultDataDirectory,
		"Directory containing DATA.OVL and the four TLK files",
	)
	command.PersistentFlags().StringVar(
		&options.Workspace,
		"workspace",
		defaultWorkspace,
		"Private base/edited conversation workspace",
	)
	command.PersistentFlags().StringVar(
		&options.PatchDir,
		"patch-dir",
		defaultPatchDirectory,
		"Tracked cumulative conversation patch root",
	)
	command.AddCommand(
		newWorkspaceInitCommand(&options),
		newWorkspaceDiffCommand(&options),
		newWorkspaceCheckCommand(&options),
		newWorkspacePatchesCommand(&options),
		newWorkspaceVerifyCommand(&options),
	)

	return command
}

func newWorkspaceInitCommand(options *conversationworkspace.Options) *cobra.Command {
	return &cobra.Command{ //nolint:exhaustruct_v5 // Cobra command options are intentionally sparse.
		Use:   "init",
		Short: "Create and validate a new private conversation workspace",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			result, err := conversationworkspace.Init(*options)
			if err != nil {
				return fmt.Errorf("initializing conversation workspace: %w", err)
			}
			_, err = fmt.Fprintf(
				command.OutOrStdout(),
				"Initialized %d conversations at %s with %d project patches applied\n",
				result.ConversationCount,
				result.Workspace,
				result.AppliedPatches,
			)
			if err != nil {
				return fmt.Errorf("writing workspace initialization summary: %w", err)
			}

			return nil
		},
	}
}

func newWorkspaceDiffCommand(options *conversationworkspace.Options) *cobra.Command {
	return &cobra.Command{ //nolint:exhaustruct_v5 // Cobra command options are intentionally sparse.
		Use:   "diff",
		Short: "Print validated base-to-edited conversation diffs",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			result, err := conversationworkspace.Diff(*options, command.OutOrStdout())
			if err != nil {
				return fmt.Errorf("diffing conversation workspace: %w", err)
			}
			_, err = fmt.Fprintf(
				command.ErrOrStderr(),
				"%d changed conversations; %d dialogue lines touched\n",
				result.ChangedConversations,
				result.DialogueLinesTouched,
			)
			if err != nil {
				return fmt.Errorf("writing workspace diff summary: %w", err)
			}

			return nil
		},
	}
}

func newWorkspaceCheckCommand(options *conversationworkspace.Options) *cobra.Command {
	return &cobra.Command{ //nolint:exhaustruct_v5 // Cobra command options are intentionally sparse.
		Use:   "check",
		Short: "Validate the protected base and complete edited catalog",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if err := conversationworkspace.Check(*options); err != nil {
				return fmt.Errorf("checking conversation workspace: %w", err)
			}
			if _, err := fmt.Fprintln(command.OutOrStdout(), "Conversation workspace is valid"); err != nil {
				return fmt.Errorf("writing workspace check summary: %w", err)
			}

			return nil
		},
	}
}

func newWorkspacePatchesCommand(options *conversationworkspace.Options) *cobra.Command {
	return &cobra.Command{ //nolint:exhaustruct_v5 // Cobra command options are intentionally sparse.
		Use:   "patches",
		Short: "Atomically regenerate cumulative project conversation patches",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			result, err := conversationworkspace.Patches(*options)
			if err != nil {
				return fmt.Errorf("regenerating conversation patches: %w", err)
			}
			_, err = fmt.Fprintf(
				command.OutOrStdout(),
				"Conversation patches: %d added, %d updated, %d removed, %d unchanged\n",
				result.Added,
				result.Updated,
				result.Removed,
				result.Unchanged,
			)
			if err != nil {
				return fmt.Errorf("writing conversation patch summary: %w", err)
			}
			targets := make([]string, 0, len(result.DialogueWarnings))
			for target := range result.DialogueWarnings {
				targets = append(targets, target)
			}
			sort.Strings(targets)
			for _, target := range targets {
				if _, err := fmt.Fprintf(
					command.ErrOrStderr(),
					"Warning: %s patch changes %d dialogue lines\n",
					target,
					result.DialogueWarnings[target],
				); err != nil {
					return fmt.Errorf("writing dialogue-content warning: %w", err)
				}
			}
			if result.DialogueLinesTouched > 0 {
				if _, err := fmt.Fprintf(
					command.ErrOrStderr(),
					"Warning total: %d dialogue lines across %d conversations\n",
					result.DialogueLinesTouched,
					result.DialogueConversations,
				); err != nil {
					return fmt.Errorf("writing dialogue-content warning summary: %w", err)
				}
			}

			return nil
		},
	}
}

func newWorkspaceVerifyCommand(options *conversationworkspace.Options) *cobra.Command {
	return &cobra.Command{ //nolint:exhaustruct_v5 // Cobra command options are intentionally sparse.
		Use:   "verify",
		Short: "Prove project patches reproduce every edited conversation",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			result, err := conversationworkspace.Verify(*options)
			if err != nil {
				return fmt.Errorf("verifying conversation patches: %w", err)
			}
			_, err = fmt.Fprintf(
				command.OutOrStdout(),
				"Verified %d conversations with %d project patches\n",
				result.ConversationCount,
				result.AppliedPatches,
			)
			if err != nil {
				return fmt.Errorf("writing conversation verification summary: %w", err)
			}

			return nil
		},
	}
}

func newExtractCommand() *cobra.Command {
	var options extractor.Options
	command := &cobra.Command{ //nolint:exhaustruct_v5 // Cobra command options are intentionally sparse.
		Use:   "extract",
		Short: "Extract original TLK files into canonical semantic YAML",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			result, err := extractor.Extract(options)
			if err != nil {
				return fmt.Errorf("extracting original conversations: %w", err)
			}
			_, err = fmt.Fprintf(
				command.OutOrStdout(),
				"Extracted %d conversations with %d audited omissions to %s (content SHA-256 %s)\n",
				result.ConversationCount,
				result.AuditEntryCount,
				result.Output,
				result.ContentSHA256,
			)
			if err != nil {
				return fmt.Errorf("writing extraction summary: %w", err)
			}

			return nil
		},
	}
	command.Flags().StringVar(&options.DataDir, "data-dir", "", "Directory containing DATA.OVL and the four TLK files")
	command.Flags().StringVar(&options.Output, "output", "", "Private output tree to create")
	command.Flags().BoolVar(&options.Force, "force", false, "Replace an existing tree owned by this extractor")
	if err := command.MarkFlagRequired("data-dir"); err != nil {
		panic(fmt.Sprintf("configuring extraction data-dir flag: %v", err))
	}
	if err := command.MarkFlagRequired("output"); err != nil {
		panic(fmt.Sprintf("configuring extraction output flag: %v", err))
	}

	return command
}

func main() {
	if err := newRootCommand().Execute(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
