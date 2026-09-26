package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/brianmichel/nomad-context/internal/config"
	"github.com/brianmichel/nomad-context/internal/contexts"
)

type nomadLoginResult struct {
	SecretID       string     `json:"SecretID"`
	ExpirationTime *time.Time `json:"ExpirationTime"`
}

func performNomadLogin(cmd *cobra.Command, ctx *config.Context, callbackAddr string, mgr *contexts.Manager) (time.Time, error) {
	binary := os.Getenv(nomadBinaryEnv)
	if binary == "" {
		binary = "nomad"
	}
	args := []string{"login", "-json", "-method=" + ctx.AuthMethod}
	if callbackAddr != "" {
		args = append(args, "-oidc-callback-addr="+callbackAddr)
	}
	commandContext := cmd.Context()
	if commandContext == nil {
		commandContext = context.Background()
	}
	child := exec.CommandContext(commandContext, binary, args...) // #nosec G204 -- configured binary and flags are intentional.
	child.Stdin = cmd.InOrStdin()
	child.Stderr = cmd.ErrOrStderr()
	env := removeEnvVar(os.Environ(), "NOMAD_TOKEN")
	child.Env = overrideEnv(env, map[string]string{"NOMAD_ADDR": ctx.Address})

	var stdout bytes.Buffer
	child.Stdout = &stdout
	if err := child.Run(); err != nil {
		return time.Time{}, fmt.Errorf("Nomad login failed for context %q: %w", ctx.Name, err)
	}

	var result nomadLoginResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		return time.Time{}, errors.New("Nomad login succeeded but returned invalid JSON; token was not saved")
	}
	if strings.TrimSpace(result.SecretID) == "" {
		return time.Time{}, errors.New("Nomad login succeeded but returned no token; token was not saved")
	}

	expiresAt := time.Time{}
	if result.ExpirationTime != nil {
		expiresAt = *result.ExpirationTime
	}
	if err := mgr.SaveLoginToken(ctx.Name, result.SecretID, expiresAt); err != nil {
		return time.Time{}, fmt.Errorf("Nomad login succeeded but the token could not be saved securely: %w", err)
	}
	return expiresAt, nil
}
