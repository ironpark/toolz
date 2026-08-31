// Package container runs a trial's commands inside a Docker or Podman
// container instead of on the host.
//
// A trial is only reproducible as far as its environment is. Copying the
// source tree fixes what the agent starts from, but the toolchain that builds
// it and the commands that grade it still come from whatever the machine
// happens to have installed, and an agent working without a sandbox can write
// anywhere. A container closes both gaps with one mechanism: the image pins
// the toolchain, and the container bounds what a trial can touch.
//
// The package deliberately shells out to the runtime's CLI rather than
// speaking to its socket. Docker and Podman disagree on almost nothing at the
// command line, both are already installed wherever either is used, and the
// alternative is a large dependency that would have to be kept current with
// two daemons instead of none.
package container
