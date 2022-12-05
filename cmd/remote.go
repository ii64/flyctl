package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/docstrings"
	"github.com/superfly/flyctl/proxy"
)

func runRemote(cmdCtx *cmdctx.CmdContext) error {
	ctx := client.NewContext(cmdCtx.Command.Context(), cmdCtx.Client)

	apiClient := client.FromContext(ctx).API()

	agentClient, err := agent.Establish(ctx, apiClient)
	if err != nil {
		return err
	}

	machine, app, err := remoteBuilderMachine(ctx, apiClient, cmdCtx.Config.GetString("app"))
	if err != nil {
		return err
	}

	var remoteHost string
	for _, ip := range machine.IPs.Nodes {
		if ip.Kind == "privatenet" {
			remoteHost = ip.IP
			break
		}
	}

	if remoteHost == "" {
		return fmt.Errorf("could not find machine IP")
	}

	dialer, err := agentClient.ConnectToTunnel(ctx, app.Organization.Slug)
	if err != nil {
		return err
	}

	sockPath := cmdCtx.Config.GetString("bind")
	if sockPath == "" {
		tmpdir, err := os.MkdirTemp("", "")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmpdir)
		sockPath = filepath.Join(tmpdir, "docker.sock")
	} else {
		_, err := os.Stat(sockPath)
		if !os.IsNotExist(err) {
			err := os.Remove(sockPath)
			if err != nil {
				return err
			}
		}
	}

	params := &proxy.ConnectParams{
		Ports:            []string{sockPath, "2375"},
		AppName:          app.Name,
		OrganizationSlug: app.Organization.Slug,
		Dialer:           dialer,
		PromptInstance:   false,
		RemoteHost:       remoteHost,
	}
	dockerHost := fmt.Sprintf("unix://%s", sockPath)

	fmt.Printf("Docker Engine @ %s\n", dockerHost)

	server, err := proxy.NewServer(ctx, params)
	if err != nil {
		return nil
	}

	return server.ProxyServer(ctx)
}

func newRemoteCommand(client *client.Client) *Command {
	remoteDocStrings := docstrings.Get("remote")
	cmd := BuildCommandKS(nil, runRemote, remoteDocStrings, client, requireSession)
	cmd.Args = cobra.ExactArgs(0)

	cmd.AddStringFlag(StringFlagOpts{
		Name:        "app",
		Description: "App name",
	})
	cmd.AddStringFlag(StringFlagOpts{
		Name:        "bind",
		Description: "Bind path for docker.sock",
		Default:     "",
	})
	return cmd
}

func remoteBuilderMachine(ctx context.Context, apiClient *api.Client, appName string) (*api.GqlMachine, *api.App, error) {
	if v := os.Getenv("FLY_REMOTE_BUILDER_HOST"); v != "" {
		return nil, nil, nil
	}

	return apiClient.EnsureRemoteBuilder(ctx, "", appName)
}
