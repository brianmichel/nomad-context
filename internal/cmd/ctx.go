package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/jedib0t/go-pretty/v6/list"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/brianmichel/nomad-context/internal/config"
	"github.com/brianmichel/nomad-context/internal/contexts"
)

const activeIndicator = "*"

func newCtxCommand(mgr *contexts.Manager) *cobra.Command {
	ctxCmd := &cobra.Command{
		Use:   "ctx",
		Short: "Manage saved Nomad contexts",
	}

	ctxCmd.AddCommand(
		newCtxListCommand(mgr),
		newCtxSetCommand(mgr),
		newCtxUseCommand(mgr),
		newCtxDeleteCommand(mgr),
		newCtxShowCommand(mgr),
	)

	return ctxCmd
}

func newCtxListCommand(mgr *contexts.Manager) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all stored contexts",
		RunE: func(cmd *cobra.Command, _ []string) error {
			contextsList, current, err := mgr.List()
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			if len(contextsList) == 0 {
				fmt.Fprintln(out, "No contexts configured.")
				return nil
			}

			renderContextTable(out, contextsList, current)
			return nil
		},
	}
}

func renderContextTable(out io.Writer, contexts []*config.Context, current string) {
	tw := table.NewWriter()
	tw.SetOutputMirror(out)
	tw.SetStyle(table.StyleRounded)
	tw.AppendHeader(table.Row{"CURRENT", "NAME", "ADDRESS"})

	useColor := shouldUseColor(out)
	if useColor {
		tw.SetRowPainter(func(row table.Row) text.Colors {
			if len(row) == 0 {
				return nil
			}
			if flag, ok := row[0].(string); ok && flag == activeIndicator {
				return text.Colors{text.FgHiGreen}
			}
			return nil
		})
	}

	for _, ctx := range contexts {
		currentIndicator := ""
		if ctx.Name == current {
			currentIndicator = activeIndicator
		}

		tw.AppendRow(table.Row{currentIndicator, ctx.Name, ctx.Address})
	}

	tw.Render()
}

func renderContextDetails(out io.Writer, ctx *config.Context, hasToken bool) {
	listWriter := list.NewWriter()
	listWriter.SetOutputMirror(out)
	listWriter.SetStyle(list.StyleConnectedRounded)

	listWriter.AppendItem(fmt.Sprintf("Context %q", ctx.Name))
	listWriter.Indent()
	listWriter.AppendItem(fmt.Sprintf("Address: %s", ctx.Address))
	listWriter.AppendItem(fmt.Sprintf("Token stored: %s", formatTokenPresence(hasToken, shouldUseColor(out))))
	listWriter.UnIndentAll()

	listWriter.Render()
}

func formatTokenPresence(hasToken bool, useColor bool) string {
	tokenStatus := "no"
	if hasToken {
		tokenStatus = "yes"
	}

	if !useColor {
		return tokenStatus
	}

	colors := text.Colors{text.FgHiRed}
	if hasToken {
		colors = text.Colors{text.FgHiGreen}
	}

	return colors.Sprint(tokenStatus)
}

func shouldUseColor(out io.Writer) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}

	if f, ok := out.(*os.File); ok {
		return term.IsTerminal(int(f.Fd()))
	}

	return false
}

func newCtxSetCommand(mgr *contexts.Manager) *cobra.Command {
	var addr string
	var token string
	var promptToken bool

	cmd := &cobra.Command{
		Use:   "set <name>",
		Short: "Create or update a context",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			existing, err := mgr.Resolve(name)
			if err != nil && !errors.Is(err, contexts.ErrContextNotFound) {
				return err
			}
			if errors.Is(err, contexts.ErrContextNotFound) {
				existing = nil
			}

			targetAddr := addr
			if targetAddr == "" && existing != nil {
				targetAddr = existing.Address
			}
			if targetAddr == "" {
				return errors.New("address is required")
			}

			saveToken := false
			tokenValue := strings.TrimSpace(token)

			if tokenValue != "" {
				saveToken = true
			} else if promptToken {
				tokenInput, err := promptForSecret(fmt.Sprintf("Enter token for %s: ", name))
				if err != nil {
					return err
				}
				tokenValue = strings.TrimSpace(tokenInput)
				if tokenValue == "" {
					return errors.New("token cannot be empty")
				}
				saveToken = true
			}

			tokenArg := ""
			if saveToken {
				tokenArg = tokenValue
			}

			if err := mgr.Upsert(name, targetAddr, tokenArg); err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Saved context %q (%s).\n", name, targetAddr)
			return nil
		},
	}

	cmd.Flags().StringVar(&addr, "addr", "", "Nomad server address, e.g. https://nomad.service:4646")
	cmd.Flags().StringVar(&token, "token", "", "Nomad ACL token to store securely")
	cmd.Flags().BoolVar(&promptToken, "prompt-token", false, "Interactively prompt for the token (useful for rotation)")
	return cmd
}

func newCtxUseCommand(mgr *contexts.Manager) *cobra.Command {
	return &cobra.Command{
		Use:   "use <name>",
		Short: "Switch the active context",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if err := mgr.Use(name); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Now using context %q.\n", name)
			return nil
		},
	}
}

func newCtxDeleteCommand(mgr *contexts.Manager) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <name>",
		Short: "Remove a stored context",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if err := mgr.Delete(name); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Deleted context %q.\n", name)
			return nil
		},
	}
}

func newCtxShowCommand(mgr *contexts.Manager) *cobra.Command {
	return &cobra.Command{
		Use:   "show [name]",
		Short: "Display details for a context (defaults to current)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := ""
			if len(args) > 0 {
				target = args[0]
			}

			ctx, err := mgr.Resolve(target)
			if err != nil {
				return err
			}

			hasToken := true
			if _, err := mgr.Token(ctx.Name); err != nil {
				if errors.Is(err, contexts.ErrTokenNotFound) {
					hasToken = false
				} else {
					return err
				}
			}

			out := cmd.OutOrStdout()
			renderContextDetails(out, ctx, hasToken)
			return nil
		},
	}
}

func promptForSecret(prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt)
	if term.IsTerminal(int(os.Stdin.Fd())) {
		data, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return "", err
		}
		return string(data), nil
	}

	reader := bufio.NewReader(os.Stdin)
	text, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(text), nil
}
