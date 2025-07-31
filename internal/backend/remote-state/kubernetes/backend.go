// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package kubernetes

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/mitchellh/go-homedir"
	"github.com/opentofu/opentofu/internal/backend"
	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/httpclient"
	"github.com/opentofu/opentofu/internal/legacy/helper/schema"
	"github.com/opentofu/opentofu/version"
	k8sSchema "k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	coordinationv1 "k8s.io/client-go/kubernetes/typed/coordination/v1"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

var (
	secretResource = k8sSchema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "secrets",
	}
)

// New creates a new backend for kubernetes remote state.
func New(enc encryption.StateEncryption) backend.Backend {
	s := &schema.Backend{
		Schema: map[string]*schema.Schema{
			"secret_suffix": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "Suffix used when creating the secret. The secret will be named in the format: `tfstate-{workspace}-{secret_suffix}`.",
			},
			"labels": {
				Type:        schema.TypeMap,
				Optional:    true,
				Description: "Map of additional labels to be applied to the secret.",
				Elem:        &schema.Schema{Type: schema.TypeString},
			},
			"namespace": {
				Type:        schema.TypeString,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("KUBE_NAMESPACE", "default"),
				Description: "Namespace to store the secret in.",
			},
			"in_cluster_config": {
				Type:        schema.TypeBool,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("KUBE_IN_CLUSTER_CONFIG", false),
				Description: "Used to authenticate to the cluster from inside a pod.",
			},
			"load_config_file": {
				Type:        schema.TypeBool,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("KUBE_LOAD_CONFIG_FILE", true),
				Description: "Load local kubeconfig.",
			},
			"host": {
				Type:        schema.TypeString,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("KUBE_HOST", ""),
				Description: "The hostname (in form of URI) of Kubernetes master.",
			},
			"username": {
				Type:        schema.TypeString,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("KUBE_USER", ""),
				Description: "The username to use for HTTP basic authentication when accessing the Kubernetes master endpoint.",
			},
			"password": {
				Type:        schema.TypeString,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("KUBE_PASSWORD", ""),
				Description: "The password to use for HTTP basic authentication when accessing the Kubernetes master endpoint.",
			},
			"insecure": {
				Type:        schema.TypeBool,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("KUBE_INSECURE", false),
				Description: "Whether server should be accessed without verifying the TLS certificate.",
			},
			"client_certificate": {
				Type:        schema.TypeString,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("KUBE_CLIENT_CERT_DATA", ""),
				Description: "PEM-encoded client certificate for TLS authentication.",
			},
			"client_key": {
				Type:        schema.TypeString,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("KUBE_CLIENT_KEY_DATA", ""),
				Description: "PEM-encoded client certificate key for TLS authentication.",
			},
			"cluster_ca_certificate": {
				Type:        schema.TypeString,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("KUBE_CLUSTER_CA_CERT_DATA", ""),
				Description: "PEM-encoded root certificates bundle for TLS authentication.",
			},
			"config_paths": {
				Type:        schema.TypeList,
				Elem:        &schema.Schema{Type: schema.TypeString},
				Optional:    true,
				Description: "A list of paths to kube config files. Can be set with KUBE_CONFIG_PATHS environment variable.",
			},
			"config_path": {
				Type:        schema.TypeString,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("KUBE_CONFIG_PATH", ""),
				Description: "Path to the kube config file. Can be set with KUBE_CONFIG_PATH environment variable.",
			},
			"config_context": {
				Type:        schema.TypeString,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("KUBE_CTX", ""),
			},
			"config_context_auth_info": {
				Type:        schema.TypeString,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("KUBE_CTX_AUTH_INFO", ""),
				Description: "",
			},
			"config_context_cluster": {
				Type:        schema.TypeString,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("KUBE_CTX_CLUSTER", ""),
				Description: "",
			},
			"token": {
				Type:        schema.TypeString,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("KUBE_TOKEN", ""),
				Description: "Token to authentifcate a service account.",
			},
			"exec": {
				Type:     schema.TypeList,
				Optional: true,
				MaxItems: 1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"api_version": {
							Type:     schema.TypeString,
							Required: true,
						},
						"command": {
							Type:     schema.TypeString,
							Required: true,
						},
						"env": {
							Type:     schema.TypeMap,
							Optional: true,
							Elem:     &schema.Schema{Type: schema.TypeString},
						},
						"args": {
							Type:     schema.TypeList,
							Optional: true,
							Elem:     &schema.Schema{Type: schema.TypeString},
						},
					},
				},
				Description: "Use a credential plugin to authenticate.",
			},
		},
	}

	result := &Backend{Backend: s, encryption: enc}
	result.Backend.ConfigureFunc = result.configure
	return result
}

type Backend struct {
	*schema.Backend
	encryption encryption.StateEncryption

	// The fields below are set from configure
	kubernetesSecretClient dynamic.ResourceInterface
	kubernetesLeaseClient  coordinationv1.LeaseInterface
	config                 *restclient.Config
	namespace              string
	labels                 map[string]string
	nameSuffix             string
}

func (b Backend) getKubernetesSecretClient() (dynamic.ResourceInterface, error) {
	if b.kubernetesSecretClient != nil {
		return b.kubernetesSecretClient, nil
	}

	client, err := dynamic.NewForConfig(b.config)
	if err != nil {
		return nil, fmt.Errorf("Failed to configure: %w", err)
	}

	b.kubernetesSecretClient = client.Resource(secretResource).Namespace(b.namespace)
	return b.kubernetesSecretClient, nil
}

func (b Backend) getKubernetesLeaseClient() (coordinationv1.LeaseInterface, error) {
	if b.kubernetesLeaseClient != nil {
		return b.kubernetesLeaseClient, nil
	}

	client, err := kubernetes.NewForConfig(b.config)
	if err != nil {
		return nil, err
	}

	b.kubernetesLeaseClient = client.CoordinationV1().Leases(b.namespace)
	return b.kubernetesLeaseClient, nil
}

func (b *Backend) configure(ctx context.Context) error {
	if b.config != nil {
		return nil
	}

	// Grab the resource data
	data := schema.FromContextBackendConfig(ctx)

	cfg, err := getInitialConfig(data)
	if err != nil {
		return err
	}

	// Overriding with static configuration
	cfg.UserAgent = httpclient.OpenTofuUserAgent(version.Version)

	if v, ok := data.GetOk("host"); ok {
		cfg.Host = v.(string)
	}
	if v, ok := data.GetOk("username"); ok {
		cfg.Username = v.(string)
	}
	if v, ok := data.GetOk("password"); ok {
		cfg.Password = v.(string)
	}
	if v, ok := data.GetOk("insecure"); ok {
		cfg.Insecure = v.(bool)
	}
	if v, ok := data.GetOk("cluster_ca_certificate"); ok {
		cfg.CAData = bytes.NewBufferString(v.(string)).Bytes()
	}
	if v, ok := data.GetOk("client_certificate"); ok {
		cfg.CertData = bytes.NewBufferString(v.(string)).Bytes()
	}
	if v, ok := data.GetOk("client_key"); ok {
		cfg.KeyData = bytes.NewBufferString(v.(string)).Bytes()
	}
	if v, ok := data.GetOk("token"); ok {
		cfg.BearerToken = v.(string)
	}

	if v, ok := data.GetOk("labels"); ok {
		labels := map[string]string{}
		for k, vv := range v.(map[string]interface{}) {
			labels[k] = vv.(string)
		}
		b.labels = labels
	}

	ns := data.Get("namespace").(string)
	b.namespace = ns
	b.nameSuffix = data.Get("secret_suffix").(string)
	b.config = cfg

	return nil
}

func getInitialConfig(data *schema.ResourceData) (*restclient.Config, error) {
	var cfg *restclient.Config
	var err error

	inCluster := data.Get("in_cluster_config").(bool)
	if inCluster {
		cfg, err = restclient.InClusterConfig()
		if err != nil {
			return nil, err
		}
	} else {
		cfg, err = tryLoadingConfigFile(data)
		if err != nil {
			return nil, err
		}
	}

	if cfg == nil {
		cfg = &restclient.Config{}
	}
	return cfg, err
}

func tryLoadingConfigFile(d *schema.ResourceData) (*restclient.Config, error) {
	loader := &clientcmd.ClientConfigLoadingRules{}

	configPaths := []string{}
	if v, ok := d.Get("config_path").(string); ok && v != "" {
		configPaths = []string{v}
	} else if v, ok := d.Get("config_paths").([]interface{}); ok && len(v) > 0 {
		for _, p := range v {
			configPaths = append(configPaths, p.(string))
		}
	} else if v := os.Getenv("KUBE_CONFIG_PATHS"); v != "" {
		configPaths = filepath.SplitList(v)
	}

	expandedPaths := []string{}
	for _, p := range configPaths {
		path, err := homedir.Expand(p)
		if err != nil {
			log.Printf("[DEBUG] Could not expand path: %s", err)
			return nil, err
		}
		log.Printf("[DEBUG] Using kubeconfig: %s", path)
		expandedPaths = append(expandedPaths, path)
	}

	if len(expandedPaths) == 1 {
		loader.ExplicitPath = expandedPaths[0]
	} else {
		loader.Precedence = expandedPaths
	}

	overrides := &clientcmd.ConfigOverrides{}

	ctx, ctxOk := d.GetOk("config_context")
	authInfo, authInfoOk := d.GetOk("config_context_auth_info")
	cluster, clusterOk := d.GetOk("config_context_cluster")
	if ctxOk || authInfoOk || clusterOk {
		if ctxOk {
			overrides.CurrentContext = ctx.(string)
			log.Printf("[DEBUG] Using custom current context: %q", overrides.CurrentContext)
		}

		overrides.Context = clientcmdapi.Context{}
		if authInfoOk {
			overrides.Context.AuthInfo = authInfo.(string)
		}
		if clusterOk {
			overrides.Context.Cluster = cluster.(string)
		}
		log.Printf("[DEBUG] Using overridden context: %#v", overrides.Context)
	}

	if v, ok := d.GetOk("exec"); ok {
		exec := &clientcmdapi.ExecConfig{}
		if spec, ok := v.([]interface{})[0].(map[string]interface{}); ok {
			exec.APIVersion = spec["api_version"].(string)
			exec.Command = spec["command"].(string)
			exec.Args = expandStringSlice(spec["args"].([]interface{}))
			for kk, vv := range spec["env"].(map[string]interface{}) {
				exec.Env = append(exec.Env, clientcmdapi.ExecEnvVar{Name: kk, Value: vv.(string)})
			}
		} else {
			return nil, fmt.Errorf("Failed to parse exec")
		}
		overrides.AuthInfo.Exec = exec
	}

	cc := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loader, overrides)
	cfg, err := cc.ClientConfig()
	if err != nil {
		if pathErr, ok := err.(*os.PathError); ok && os.IsNotExist(pathErr.Err) {
			log.Printf("[INFO] Unable to load config file as it doesn't exist at %q", pathErr.Path)
			return nil, nil
		}
		return nil, fmt.Errorf("Failed to initialize kubernetes configuration: %w", err)
	}

	log.Printf("[INFO] Successfully initialized config")
	return cfg, nil
}

func expandStringSlice(s []interface{}) []string {
	result := make([]string, len(s))
	for k, v := range s {
		// Handle the OpenTofu parser bug which turns empty strings in lists to nil.
		if v == nil {
			result[k] = ""
		} else {
			result[k] = v.(string)
		}
	}
	return result
}
