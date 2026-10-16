package testserver

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/ilijamt/terraform-provider-technitium/internal/client"
	"github.com/ilijamt/terraform-provider-technitium/internal/technitium"
)

const (
	Username = "admin"
	Password = "admin"
	// Label marks the containers Launch runs, so ones leaked by a killed test
	// binary can be found and removed (make testacc-clean).
	Label = "terraform-provider-technitium.testacc"
)

// Container is a server that Launch started.
type Container struct {
	// ID names the container for docker exec.
	ID string
	// Host is the web service's address on 127.0.0.1.
	Host string
	// IP is the container's address on the Docker bridge network.
	IP string
	// Ports are the extra ports, each published on the same port of 127.0.0.1.
	Ports []int
	// Stop removes the container.
	Stop func()
}

// Launch runs a server in Docker. Each of ports is also published on the same
// port of 127.0.0.1, for a test that moves a service there.
func Launch(ctx context.Context, ports ...int) (*Container, error) {
	args := []string{"run", "--detach", "--rm", "--label", Label, "--publish", "127.0.0.1::5380"}
	for _, p := range ports {
		args = append(args, "--publish", fmt.Sprintf("127.0.0.1:%d:%d", p, p))
	}
	args = append(args, "--env", "DNS_SERVER_ADMIN_PASSWORD="+Password, "technitium/dns-server:"+technitium.Version)
	out, err := exec.CommandContext(ctx, "docker", args...).Output()
	if err != nil {
		return nil, fmt.Errorf("docker run: %w", exitErr(err))
	}
	id := strings.TrimSpace(string(out))
	c := &Container{ID: id, Ports: ports, Stop: func() { _ = exec.Command("docker", "rm", "--force", id).Run() }}

	out, err = exec.CommandContext(ctx, "docker", "port", id, "5380/tcp").Output()
	if err != nil {
		c.Stop()
		return nil, fmt.Errorf("docker port: %w", exitErr(err))
	}
	addr, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
	c.Host = "http://" + addr

	out, err = exec.CommandContext(ctx, "docker", "inspect", "--format", "{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}", id).Output()
	if err != nil {
		c.Stop()
		return nil, fmt.Errorf("docker inspect: %w", exitErr(err))
	}
	c.IP = strings.TrimSpace(string(out))

	if err = wait(ctx, c.Host); err != nil {
		c.Stop()
		return nil, err
	}
	return c, nil
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
