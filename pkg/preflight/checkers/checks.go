package checkers

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"

	api "github.com/weaveworks/ignite/pkg/apis/ignite"
	"github.com/weaveworks/ignite/pkg/constants"
	"github.com/weaveworks/ignite/pkg/network"
	"github.com/weaveworks/ignite/pkg/preflight"
	"github.com/weaveworks/ignite/pkg/providers"
	"k8s.io/apimachinery/pkg/util/sets"
)

const (
	oldPathString  = "/"
	newPathString  = "-"
	noReplaceLimit = -1
)

type PortOpenChecker struct {
	port uint64
}

func (poc PortOpenChecker) Check() error {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", poc.port))
	if err != nil {
		return fmt.Errorf("Port %d is in use", poc.port)
	}
	if err := listener.Close(); err != nil {
		return fmt.Errorf("Port %d is in used, failed to close it", poc.port)
	}
	return nil
}

func (poc PortOpenChecker) Name() string {
	return fmt.Sprintf("Port-%d", poc.port)
}

func (poc PortOpenChecker) Type() string {
	return "Port"
}

type ExistingFileChecker struct {
	filePath string
}

func NewExistingFileChecker(filePath string) ExistingFileChecker {
	return ExistingFileChecker{
		filePath: filePath,
	}
}

func (efc ExistingFileChecker) Check() error {
	if _, err := os.Stat(efc.filePath); os.IsNotExist(err) {
		return fmt.Errorf("File %s, does not exist", efc.filePath)
	}
	return nil
}

func (efc ExistingFileChecker) Name() string {
	return fmt.Sprintf("ExistingFile-%s", strings.Replace(efc.filePath, oldPathString, newPathString, noReplaceLimit))
}

func (efc ExistingFileChecker) Type() string {
	return "ExistingFile"
}

type BinInPathChecker struct {
	bin string
}

func (bipc BinInPathChecker) Check() error {
	_, err := exec.LookPath(bipc.bin)
	if err != nil {
		return fmt.Errorf("Bin %s is not in your PATH", bipc.bin)
	}
	return nil
}

func (bipc BinInPathChecker) Name() string {
	return ""
}

func (bipc BinInPathChecker) Type() string {
	return ""
}

type AvailablePathChecker struct {
	path string
}

func StartCmdChecks(vm *api.VM, ignoredPreflightErrors sets.String) error {
	checks := []preflight.Checker{}
	for _, dependency := range constants.PathDependencies {
		checks = append(checks, ExistingFileChecker{filePath: dependency})
	}
	if providers.NetworkPluginName == network.PluginCNI {
		for _, dependency := range constants.CNIDependencies {
			checks = append(checks, ExistingFileChecker{filePath: dependency})
		}
	}

	checks = append(checks, providers.Runtime.PreflightChecker())
	for _, port := range vm.Spec.Network.Ports {
		checks = append(checks, PortOpenChecker{port: port.HostPort})
	}

	// Binary name of the runtime is just the runtime name in string, "docker" or "containerd"
	// So it is OK for us to check only one of them, as people may installing only containerd but not docker
	runtimeBinaryName := providers.RuntimeName.String()
	checks = append(checks, BinInPathChecker{bin: runtimeBinaryName})

	// Check common binaries
	for _, dependency := range constants.BinaryDependencies {
		checks = append(checks, BinInPathChecker{bin: dependency})
	}
	return runChecks(checks, ignoredPreflightErrors)
}

func runChecks(checks []preflight.Checker, ignoredPreflightErrors sets.String) error {
	var errBuffer bytes.Buffer

	for _, check := range checks {
		name := check.Name()
		checkType := check.Type()

		err := check.Check()
		if isIgnoredPreflightError(ignoredPreflightErrors, checkType) {
			err = nil
		}

		if err != nil {
			errBuffer.WriteString(fmt.Sprintf("[ERROR %s]: %v\n", name, err))
		}
	}
	if errBuffer.Len() > 0 {
		return fmt.Errorf(errBuffer.String())
	}
	return nil
}

func isIgnoredPreflightError(ignoredPreflightError sets.String, checkType string) bool {
	return ignoredPreflightError.Has("all") || ignoredPreflightError.Has(strings.ToLower(checkType))
}
