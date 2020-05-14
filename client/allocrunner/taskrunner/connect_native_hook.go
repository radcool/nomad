package taskrunner

import (
	"context"
	"io"
	"os"
	"path/filepath"

	hclog "github.com/hashicorp/go-hclog"
	ifs "github.com/hashicorp/nomad/client/allocrunner/interfaces"
	"github.com/hashicorp/nomad/nomad/structs"
	"github.com/hashicorp/nomad/nomad/structs/config"
	"github.com/pkg/errors"
)

const (
	connectNativeHookName = "connect_native"
)

type connectNativeHookConfig struct {
	consul consulTransportConfig
	alloc  *structs.Allocation
	logger hclog.Logger
}

func newConnectNativeHookConfig(alloc *structs.Allocation, consul *config.ConsulConfig, logger hclog.Logger) *connectNativeHookConfig {
	return &connectNativeHookConfig{
		alloc:  alloc,
		logger: logger,
		consul: newConsulTransportConfig(consul),
	}
}

// connectNativeHook manages additional automagic configuration for a connect
// native task.
//
// If nomad client is configured to talk to Consul using TLS (or other special
// auth), the native task will inherit that configuration EXCEPT for the consul
// token.
//
// If consul is configured with ACLs enabled, a Service Identity token will be
// generated on behalf of the native service and supplied to the task.
type connectNativeHook struct {
	// alloc is the allocation with the connect native task being run
	alloc *structs.Allocation

	// consulConfig is used to enable the connect native enabled task to
	// communicate with consul directly, as is necessary for the task to request
	// its connect mTLS certificates.
	consulConfig consulTransportConfig

	// logger is used to log things
	logger hclog.Logger
}

func newConnectNativeHook(c *connectNativeHookConfig) *connectNativeHook {
	return &connectNativeHook{
		alloc:        c.alloc,
		consulConfig: c.consul,
		logger:       c.logger.Named(connectNativeHookName),
	}
}

func (connectNativeHook) Name() string {
	return connectNativeHookName
}

func (h *connectNativeHook) Prestart(
	ctx context.Context,
	request *ifs.TaskPrestartRequest,
	response *ifs.TaskPrestartResponse) error {

	if !request.Task.Kind.IsConnectNative() {
		response.Done = true
		return nil
	}

	// copy TLS certificates
	if err := h.copyCertificates(h.consulConfig, request.TaskDir.SecretsDir); err != nil {
		h.logger.Error("failed to copy Consul TLS certificates", "error", err)
		return err
	}

	// request SI token

	// set environment variables

	return nil
}

const (
	secretCAFilename       = "_consul_ca.pem"
	secretCertfileFilename = "_consul_cert.pem"
	secretKeyfileFilename  = "_consul_key.pem"
)

func (h *connectNativeHook) copyCertificates(consulConfig consulTransportConfig, dir string) error {
	if err := h.copyCertificate(consulConfig.CAFile, dir, secretCAFilename); err != nil {
		return err
	}
	if err := h.copyCertificate(consulConfig.CertFile, dir, secretCertfileFilename); err != nil {
		return err
	}
	if err := h.copyCertificate(consulConfig.KeyFile, dir, secretKeyfileFilename); err != nil {
		return err
	}
	return nil
}

func (connectNativeHook) copyCertificate(source, dir, name string) error {
	if source == "" {
		return nil
	}

	original, err := os.Open(source)
	if err != nil {
		return errors.Wrap(err, "failed to open consul TLS certificate")
	}
	defer original.Close()

	destination := filepath.Join(dir, name)
	fd, err := os.Create(destination)
	if err != nil {
		return errors.Wrapf(err, "failed to create secrets/%s", name)
	}
	defer fd.Close()

	if _, err := io.Copy(fd, original); err != nil {
		return errors.Wrapf(err, "failed to copy certificate secrets/%s", name)
	}

	if err := fd.Sync(); err != nil {
		return errors.Wrapf(err, "failed to write secrets/%s", name)
	}

	return nil
}
