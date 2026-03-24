// Package ssh provides the optional SSH worker extension per spec Appendix A.
//
// This package is a stub. The SSH extension allows Symphony to dispatch
// worker runs to remote hosts over SSH while keeping the orchestrator
// as the single source of truth for polling, claims, and reconciliation.
//
// When fully implemented:
//   - worker.ssh_hosts provides candidate SSH destinations
//   - Each worker run is assigned to one host at a time
//   - workspace.root is interpreted on the remote host
//   - The coding-agent adapter is launched over SSH stdio
//   - Continuation turns stay on the same host and workspace
package ssh

// HostConfig describes an SSH host for remote worker dispatch.
type HostConfig struct {
	Host       string
	Port       int
	User       string
	PrivateKey string
	MaxWorkers int
}

// Pool manages a set of SSH hosts for worker dispatch.
type Pool struct {
	hosts []HostConfig
}

// NewPool creates an SSH host pool.
func NewPool(hosts []HostConfig) *Pool {
	return &Pool{hosts: hosts}
}

// Available returns true if any host has capacity.
func (p *Pool) Available() bool {
	return len(p.hosts) > 0
}
