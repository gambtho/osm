package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	helm "helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/strvals"
)

const installDesc = `
This command installs the osm control plane on the Kubernetes cluster.

The osm control plane components get installed into the osm-system namespace
by default. This can be overriden with the --namespace flag. If the give
namespace does not exist, it will be created.

Usage:
  $ osm install --namespace hello-world

Each instance of the osm control plane installation is given a unqiue ID
referred to as the OSM ID. An OSM ID can be passed in via the --osm-id flag.
The osm ID is used in various different ways by the control plane including
the resource name for the MutatingWebhookConfiguration created by the control
plane for sidecar injection of envoy proxies.

By default, OSM ID will be configured to "osm-cp-<namespace>" where namespace is the namespace where the control plane is being installed

Usage:
  $ osm install --osm-id "hello-osm"

`
const (
	defaultOSMChartPath  = "charts/osm"
	defaultCertManager   = "tresor"
	defaultVaultProtocol = "http"
)

type installCmd struct {
	out                        io.Writer
	containerRegistry          string
	containerRegistrySecret    string
	chartPath                  string
	osmImageTag                string
	certManager                string
	vaultHost                  string
	vaultProtocol              string
	vaultToken                 string
	vaultRole                  string
	serviceCertValidityMinutes int
}

func newInstallCmd(config *helm.Configuration, out io.Writer) *cobra.Command {

	inst := &installCmd{
		out: out,
	}

	cmd := &cobra.Command{
		Use:   "install",
		Short: "install osm control plane",
		Long:  installDesc,
		RunE: func(_ *cobra.Command, args []string) error {
			return inst.run(helm.NewInstall(config))
		},
	}

	f := cmd.Flags()
	f.StringVar(&inst.containerRegistry, "container-registry", "smctest.azurecr.io", "container registry that hosts control plane component images")
	f.StringVar(&inst.osmImageTag, "osm-image-tag", "latest", "osm image tag")
	f.StringVar(&inst.containerRegistrySecret, "container-registry-secret", "acr-creds", "name of Kubernetes secret for container registry credentials to be created if it doesn't already exist")
	f.StringVar(&inst.chartPath, "osm-chart-path", "", "path to osm chart to override default chart")
	f.StringVar(&inst.certManager, "cert-manager", defaultCertManager, "certificate manager to use (tresor or vault)")
	f.StringVar(&inst.vaultHost, "vault-host", "", "Hashicorp Vault host/service - where Vault is installed")
	f.StringVar(&inst.vaultProtocol, "vault-protocol", defaultVaultProtocol, "protocol to use to connect to Vault")
	f.StringVar(&inst.vaultToken, "vault-token", "", "token that should be used to connect to Vault")
	f.StringVar(&inst.vaultRole, "vault-role", "open-service-mesh", "Vault role to be used by Open Service Mesh")
	f.IntVar(&inst.serviceCertValidityMinutes, "service-cert-validity-minutes", int(1), "Certificate TTL in minutes")

	return cmd
}

func (i *installCmd) run(installClient *helm.Install) error {
	installClient.ReleaseName = settings.Namespace()
	installClient.Namespace = settings.Namespace()
	installClient.CreateNamespace = true

	chart := ""
	if i.chartPath != "" {
		chart = i.chartPath
	} else {
		chart = defaultOSMChartPath
	}
	chartRequested, err := loader.Load(chart)
	if err != nil {
		return err
	}

	if strings.EqualFold(i.certManager, "vault") {
		var missingFields []string
		if i.vaultHost == "" {
			missingFields = append(missingFields, "vault-host")
		}
		if i.vaultToken == "" {
			missingFields = append(missingFields, "vault-token")
		}
		if len(missingFields) != 0 {
			return fmt.Errorf("Missing arguments for cert-manager vault: %v", missingFields)
		}
	}
	values, err := i.resolveValues()
	if err != nil {
		return err
	}

	if _, err = installClient.Run(chartRequested, values); err != nil {
		return err
	}

	fmt.Fprintf(i.out, "OSM installed successfully in %s namespace\n", settings.Namespace())
	return nil
}

func (i *installCmd) resolveValues() (map[string]interface{}, error) {
	finalValues := map[string]interface{}{}
	valuesConfig := []string{
		fmt.Sprintf("image.registry=%s", i.containerRegistry),
		fmt.Sprintf("image.tag=%s", i.osmImageTag),
		fmt.Sprintf("imagePullSecrets[0].name=%s", i.containerRegistrySecret),
		fmt.Sprintf("namespace=%s", settings.Namespace()),
		fmt.Sprintf("certManager=%s", i.certManager),
		fmt.Sprintf("vault.host=%s", i.vaultHost),
		fmt.Sprintf("vault.protocol=%s", i.vaultProtocol),
		fmt.Sprintf("vault.token=%s", i.vaultToken),
		fmt.Sprintf("serviceCertValidityMinutes=%d", i.serviceCertValidityMinutes),
	}

	for _, val := range valuesConfig {
		if err := strvals.ParseInto(val, finalValues); err != nil {
			return finalValues, err
		}
	}
	return finalValues, nil
}
