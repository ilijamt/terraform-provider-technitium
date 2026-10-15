package testserver

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/ilijamt/terraform-provider-technitium/internal/client"
	"github.com/ilijamt/terraform-provider-technitium/internal/technitium"
)

const (
	Username = "admin"
	Password = "admin"
	// Label marks the containers Start runs, so ones leaked by a killed test
	// binary can be found and removed (make testacc-clean).
	Label = "terraform-provider-technitium.testacc"
)

// Start uses the server at TECHNITIUM_HOST, which must accept Username and
// Password, or else runs one in Docker.
func Start(ctx context.Context) (host string, stop func(), err error) {
	if host = os.Getenv("TECHNITIUM_HOST"); host != "" {
		return host, func() {}, nil
	}
	host, _, stop, err = Run(ctx)
	return host, stop, err
}

// Run also returns the container's IP on the Docker bridge network, where
// other containers reach it.
func Run(ctx context.Context) (host, ip string, stop func(), err error) {
	out, err := exec.CommandContext(ctx, "docker", "run", "--detach", "--rm", "--label", Label,
		"--publish", "127.0.0.1::5380",
		"--env", "DNS_SERVER_ADMIN_PASSWORD="+Password,
		"technitium/dns-server:"+technitium.Version).Output()
	if err != nil {
		return "", "", nil, fmt.Errorf("docker run: %w", exitErr(err))
	}
	id := strings.TrimSpace(string(out))
	stop = func() { _ = exec.Command("docker", "rm", "--force", id).Run() }

	out, err = exec.CommandContext(ctx, "docker", "port", id, "5380/tcp").Output()
	if err != nil {
		stop()
		return "", "", nil, fmt.Errorf("docker port: %w", exitErr(err))
	}
	addr, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
	host = "http://" + addr

	out, err = exec.CommandContext(ctx, "docker", "inspect", "--format", "{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}", id).Output()
	if err != nil {
		stop()
		return "", "", nil, fmt.Errorf("docker inspect: %w", exitErr(err))
	}
	ip = strings.TrimSpace(string(out))

	if err = wait(ctx, host); err != nil {
		stop()
		return "", "", nil, err
	}
	return host, ip, stop, nil
}

func wait(ctx context.Context, host string) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	c := client.New(host, "", "testserver", false, nil)
	for {
		err := c.Login(ctx, Username, Password)
		if err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("technitium at %s did not start: %w", host, err)
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func exitErr(err error) error {
	if e, ok := err.(*exec.ExitError); ok && len(e.Stderr) > 0 {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(e.Stderr)))
	}
	return err
}
