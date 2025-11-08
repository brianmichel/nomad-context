package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/brianmichel/nomad-context/internal/contexts"
)

const nomadBinaryEnv = "NOMAD_CONTEXT_NOMAD_PATH"

func NewRootCmd() *cobra.Command {
	mgr := contexts.NewManager()

	root := &cobra.Command{
		Use:           "nomad-context",
		Short:         "Manage Nomad CLI contexts or proxy commands to nomad",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			return runNomad(args, mgr)
		},
	}

	root.AddCommand(newCtxCommand(mgr))
	return root
}

func runNomad(args []string, mgr *contexts.Manager) error {
	ctx, err := mgr.Current()
	if err != nil {
		return err
	}

	token, err := mgr.Token(ctx.Name)
	if err != nil {
		if errors.Is(err, contexts.ErrTokenNotFound) {
			token = ""
		} else {
			return err
		}
	}

	binary := os.Getenv(nomadBinaryEnv)
	if binary == "" {
		binary = "nomad"
	}

	env := removeEnvVar(os.Environ(), "NOMAD_TOKEN")
	overrides := map[string]string{
		"NOMAD_ADDR": ctx.Address,
	}
	if token != "" {
		overrides["NOMAD_TOKEN"] = token
	}

	command := exec.Command(binary, args...) // #nosec G204 -- arguments are provided intentionally by the user.
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	command.Stdin = os.Stdin
	command.Env = overrideEnv(env, overrides)

	return command.Run()
}

func overrideEnv(base []string, overrides map[string]string) []string {
	result := make([]string, 0, len(base)+len(overrides))
	used := make(map[string]struct{})

	for _, kv := range base {
		key, _ := splitEnvPair(kv)
		if value, ok := overrides[key]; ok {
			result = append(result, fmt.Sprintf("%s=%s", key, value))
			used[key] = struct{}{}
			continue
		}
		result = append(result, kv)
	}

	for key, value := range overrides {
		if _, ok := used[key]; ok {
			continue
		}
		result = append(result, fmt.Sprintf("%s=%s", key, value))
	}
	return result
}

func removeEnvVar(env []string, key string) []string {
	filtered := make([]string, 0, len(env))
	for _, kv := range env {
		name, _ := splitEnvPair(kv)
		if name == key {
			continue
		}
		filtered = append(filtered, kv)
	}
	return filtered
}

func splitEnvPair(kv string) (string, string) {
	if i := strings.IndexByte(kv, '='); i >= 0 {
		return kv[:i], kv[i+1:]
	}
	return kv, ""
}
