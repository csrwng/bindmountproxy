package client

import (
	"fmt"
	"io"
	"net/http/httputil"
	"os"
	"strings"

	"golang.org/x/net/context"

	"github.com/Sirupsen/logrus"
	Cli "github.com/docker/docker/cli"
	flag "github.com/docker/docker/pkg/mflag"
	"github.com/docker/docker/pkg/promise"
	"github.com/docker/docker/pkg/signal"
	"github.com/docker/engine-api/types"
)

func (cli *DockerCli) forwardAllSignals(cid string) chan os.Signal {
	sigc := make(chan os.Signal, 128)
	signal.CatchAll(sigc)
	go func() {
		for s := range sigc {
			if s == signal.SIGCHLD || s == signal.SIGPIPE {
				continue
			}
			var sig string
			for sigStr, sigN := range signal.SignalMap {
				if sigN == s {
					sig = sigStr
					break
				}
			}
			if sig == "" {
				fmt.Fprintf(cli.err, "Unsupported signal: %v. Discarding.\n", s)
				continue
			}

			if err := cli.client.ContainerKill(context.Background(), cid, sig); err != nil {
				logrus.Debugf("Error sending signal: %s", err)
			}
		}
	}()
	return sigc
}

// CmdStart starts one or more containers.
//
// Usage: docker start [OPTIONS] CONTAINER [CONTAINER...]
func (cli *DockerCli) CmdStart(args ...string) error {
	cmd := Cli.Subcmd("start", []string{"CONTAINER [CONTAINER...]"}, Cli.DockerCommands["start"].Description, true)
	attach := cmd.Bool([]string{"a", "-attach"}, false, "Attach STDOUT/STDERR and forward signals")
	openStdin := cmd.Bool([]string{"i", "-interactive"}, false, "Attach container's STDIN")
	detachKeys := cmd.String([]string{"-detach-keys"}, "", "Override the key sequence for detaching a container")
	cmd.Require(flag.Min, 1)

	cmd.ParseFlags(args, true)

	if *attach || *openStdin {
		// We're going to attach to a container.
		// 1. Ensure we only have one container.
		if cmd.NArg() > 1 {
			return fmt.Errorf("You cannot start and attach multiple containers at once.")
		}

		// 2. Attach to the container.
		container := cmd.Arg(0)
		c, err := cli.client.ContainerInspect(context.Background(), container)
		if err != nil {
			return err
		}

		if !c.Config.Tty {
			sigc := cli.forwardAllSignals(container)
			defer signal.StopCatch(sigc)
		}

		if *detachKeys != "" {
			cli.configFile.DetachKeys = *detachKeys
		}

		options := types.ContainerAttachOptions{
			Stream:     true,
			Stdin:      *openStdin && c.Config.OpenStdin,
			Stdout:     true,
			Stderr:     true,
			DetachKeys: cli.configFile.DetachKeys,
		}

		var in io.ReadCloser

		if options.Stdin {
			in = cli.in
		}

		resp, errAttach := cli.client.ContainerAttach(context.Background(), container, options)
		if errAttach != nil && errAttach != httputil.ErrPersistEOF {
			// ContainerAttach return an ErrPersistEOF (connection closed)
			// means server met an error and put it in Hijacked connection
			// keep the error and read detailed error message from hijacked connection
			return errAttach
		}
		defer resp.Close()
		ctx, cancelFun := context.WithCancel(context.Background())
		cErr := promise.Go(func() error {
			errHijack := cli.holdHijackedConnection(ctx, c.Config.Tty, in, cli.out, cli.err, resp)
			if errHijack == nil {
				return errAttach
			}
			return errHijack
		})

		// 3. Start the container.
		if err := cli.client.ContainerStart(context.Background(), container); err != nil {
			cancelFun()
			<-cErr
			return err
		}

		// 4. Wait for attachment to break.
		if c.Config.Tty && cli.isTerminalOut {
			if err := cli.monitorTtySize(container, false); err != nil {
				fmt.Fprintf(cli.err, "Error monitoring TTY size: %s\n", err)
			}
		}
		if attchErr := <-cErr; attchErr != nil {
			return attchErr
		}
		_, status, err := getExitCode(cli, container)
		if err != nil {
			return err
		}
		if status != 0 {
			return Cli.StatusError{StatusCode: status}
		}
	} else {
		// We're not going to attach to anything.
		// Start as many containers as we want.
		return cli.startContainersWithoutAttachments(cmd.Args())
	}

	return nil
}

func (cli *DockerCli) startContainersWithoutAttachments(containers []string) error {
	var failedContainers []string
	for _, container := range containers {
		if err := cli.client.ContainerStart(context.Background(), container); err != nil {
			fmt.Fprintf(cli.err, "%s\n", err)
			failedContainers = append(failedContainers, container)
		} else {
			fmt.Fprintf(cli.out, "%s\n", container)
		}
	}

	if len(failedContainers) > 0 {
		return fmt.Errorf("Error: failed to start containers: %v", strings.Join(failedContainers, ", "))
	}
	return nil
}
