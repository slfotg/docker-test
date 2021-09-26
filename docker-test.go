package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
)

func main() {
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		panic(err)
	}
	// uid=$(id -u `whoami`)
	uid := os.Getuid()
	// gid=$(id -g `whoami`)
	gid := os.Getgid()
	// container_id=$(docker run -u $uid:$gid -v `pwd`:`pwd` -w `pwd` -d --rm rust:1.55 tail -f /dev/null)
	containerId, err := startContainer(ctx, cli, uid, gid, "rust:1.55", "/home/sam/repos/rad1")
	if err != nil {
		panic(err)
	}
	// echo "$ rustup component add clippy"
	if err := execCommands(ctx, cli, containerId,
		"rustup component add clippy rustfmt",
		"cargo build --release --verbose",
		"cargo test --verbose",
		"cargo fmt --all -- --check",
		"cargo clippy -- -D warnings"); err != nil {
		fmt.Printf("err: %v\n", err)
	}

	if err := cli.ContainerStop(ctx, containerId, nil); err != nil {
		panic(err)
	}
}

func startContainer(ctx context.Context, cli *client.Client, uid int, gid int, image string, dir string) (string, error) {
	resp, err := cli.ContainerCreate(ctx, &container.Config{
		Image:      image,
		User:       fmt.Sprintf("%v:%v", uid, gid),
		Cmd:        []string{"tail", "-f", "/dev/null"},
		WorkingDir: dir,
		Env:        []string{"CARGO_TERM_COLOR=always"},
		Tty:        true,
	}, &container.HostConfig{
		AutoRemove: true,
		Mounts: []mount.Mount{
			{
				Type:   mount.TypeBind,
				Source: dir,
				Target: dir,
			},
		},
	}, nil, nil, "")
	if err != nil {
		return "", err
	}
	return resp.ID, cli.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{})
}

func execCommands(ctx context.Context, cli *client.Client, containerId string, commands ...string) error {
	for _, command := range commands {
		if err := execCommand(ctx, cli, containerId, command); err != nil {
			return err
		}
	}
	return nil
}

func execCommand(ctx context.Context, cli *client.Client, containerId string, command string) error {
	fmt.Println("$ ", command)
	config := types.ExecConfig{AttachStdout: true, AttachStderr: true,
		Cmd: strings.Split(command, " "),
		Tty: true}
	execID, _ := cli.ContainerExecCreate(ctx, containerId, config)
	resp, err := cli.ContainerExecAttach(ctx, execID.ID, types.ExecStartCheck{})
	if err != nil {
		return err
	}
	defer resp.Close()

	stdcopy.StdCopy(os.Stdout, os.Stderr, resp.Reader)

	inspect, err := cli.ContainerExecInspect(ctx, execID.ID)
	if err != nil {
		return err
	}
	if inspect.ExitCode != 0 {
		return errors.New(fmt.Sprintf("Command %v failed with exit code %v", config.Cmd, inspect.ExitCode))
	}
	return nil
}
