package buildkit

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"github.com/denisbrodbeck/machineid"
	"github.com/docker/cli/cli/command/image/build"
	"github.com/docker/docker/pkg/archive"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/session"
	"github.com/moby/buildkit/session/secrets/secretsprovider"
	"github.com/moby/buildkit/session/sshforward/sshprovider"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func getCompiledOutputs(data *schema.ResourceData) []client.ExportEntry {
	outputs := make([]client.ExportEntry, 0)
	publish_targets := data.Get("publish_target").([]interface{})
	for _, x := range publish_targets {
		casted := x.(map[string]interface{})
		withoutProtocol := strings.ReplaceAll(casted["registry"].(string), "https://", "")
		outputs = append(outputs, client.ExportEntry{
			Type: "image",
			Attrs: map[string]string{
				"name": fmt.Sprintf("%s/%s:%s", withoutProtocol, casted["name"].(string), casted["tag"].(string)),
				"push": "true",
			},
		})
	}
	return outputs
}

func getSecretsProvider(secrets map[string][]byte) session.Attachable {
	return secretsprovider.FromMap(secrets)
}

func getPlatforms(data *schema.ResourceData) []string {
	platforms := data.Get("platforms").([]interface{})
	result := make([]string, len(platforms))
	for i, x := range platforms {
		result[i] = x.(string)
	}
	return result
}

func getSecrets(data *schema.ResourceData) (map[string][]byte, diag.Diagnostics) {
	diagnostics := diag.Diagnostics{}
	result := map[string][]byte{}
	secrets := data.Get("secrets").(map[string]interface{})
	secrets_base64 := data.Get("secrets_base64").(map[string]interface{})
	for k, v := range secrets {
		result[k] = []byte(v.(string))
	}
	for k, v := range secrets_base64 {
		decoded, err := base64.StdEncoding.DecodeString(v.(string))
		if err == nil {
			result[k] = decoded
		} else {
			diagnostics = append(diagnostics, diag.Diagnostic{
				Severity: diag.Error,
				Summary:  "Failed to base64 decode a secret.",
			})
		}
	}
	return result, diagnostics
}

func getSSHAgents(data *schema.ResourceData) map[string]string {
	result := map[string]string{}
	if data.Get("forward_ssh_agent_socket").(bool) {
		result["default"] = os.Getenv("SSH_AUTH_SOCK")
		return result
	} else {
		return result
	}
}

func getSSHProvider(ssh map[string]string) (session.Attachable, diag.Diagnostics) {
	configs := make([]sshprovider.AgentConfig, 0)
	for k, v := range ssh {
		configs = append(configs, sshprovider.AgentConfig{
			ID:    k,
			Paths: strings.Split(v, ","),
		})
	}
	sshProvider, err := sshprovider.NewSSHAgentProvider(configs)
	if err != nil {
		return nil, diag.Diagnostics{diag.Diagnostic{
			Severity: diag.Error,
			Summary:  err.Error(),
		}}
	}
	return sshProvider, diag.Diagnostics{}
}

func merge(maps ...map[string]string) map[string]string {
	result := map[string]string{}
	for _, m := range maps {
		for k, v := range m {
			result[k] = v
		}
	}
	return result
}

func getLabels(data *schema.ResourceData) map[string]string {
	result := map[string]string{}
	secrets := data.Get("labels").(map[string]interface{})
	for k, v := range secrets {
		result["label:"+k] = v.(string)
	}
	return result
}

func getBuildArgs(data *schema.ResourceData) map[string]string {
	result := map[string]string{}
	secrets := data.Get("args").(map[string]interface{})
	for k, v := range secrets {
		result["build-arg:"+k] = v.(string)
	}
	return result
}

func getDirectoryHash(directory string) (string, diag.Diagnostics) {
	directory, _ = filepath.Abs(directory)
	excludePatterns, err := build.ReadDockerignore(directory)
	if err != nil {
		return "", diag.Diagnostics{
			diag.Diagnostic{
				Severity: diag.Error,
				Summary:  fmt.Sprintf("Could not open .dockerignore file in directory '%s'.", directory),
				Detail:   err.Error(),
			},
		}
	}
	tarHandle, err := archive.TarWithOptions(directory, &archive.TarOptions{
		ExcludePatterns: excludePatterns,
	})
	hash := sha256.New()
	_, err = io.Copy(hash, tarHandle)
	if err != nil {
		return "", diag.Diagnostics{
			diag.Diagnostic{
				Severity: diag.Error,
				Summary:  err.Error(),
			},
		}
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), diag.Diagnostics{}
}

func createImage(ctx context.Context, data *schema.ResourceData, meta interface{}) diag.Diagnostics {

	buildContext := data.Get("context").(string)
	dockerfile := data.Get("dockerfile").(string)
	provider := meta.(TerraformProviderBuildkit)
	platforms := getPlatforms(data)
	labels := getLabels(data)
	args := getBuildArgs(data)
	secrets, diags := getSecrets(data)

	if len(diags) > 0 {
		return diags
	}

	sshAgents := getSSHAgents(data)
	outputs := getCompiledOutputs(data)
	contextHash, diags := getDirectoryHash(buildContext)

	if len(diags) > 0 {
		return diags
	}

	data.SetId(contextHash)

	_ = data.Set("context_digest", contextHash)

	sessionProviders := make([]session.Attachable, 0)
	dockerAuthProvider := NewDockerAuthProvider(provider.registry_auth)
	secretsProvider := getSecretsProvider(secrets)
	sshProvider, diags := getSSHProvider(sshAgents)

	if len(diags) > 0 {
		return diags
	}

	sessionProviders = append(sessionProviders, dockerAuthProvider, secretsProvider, sshProvider)

	cli, err := client.New(context.Background(), provider.host, client.WithFailFast())

	if err != nil {
		panic(err)
	}

	defer cli.Close()

	sharedKey, err := machineid.ProtectedID("terraform-provider-buildkit")

	if err != nil {
		return diag.Diagnostics{
			diag.Diagnostic{
				Severity: diag.Error,
				Summary:  err.Error(),
			},
		}
	}

	resp, err := cli.Solve(ctx, nil, client.SolveOpt{
		Exports:  outputs,
		Frontend: "dockerfile.v0",
		FrontendAttrs: merge(labels, args, map[string]string{
			"platform": strings.Join(platforms, ","),
		}),
		LocalDirs: map[string]string{
			"context":    buildContext,
			"dockerfile": filepath.Dir(dockerfile),
		},
		Session:   sessionProviders,
		SharedKey: sharedKey,
	}, nil)

	if err != nil {
		return diag.Diagnostics{diag.Diagnostic{
			Severity: diag.Error,
			Summary:  err.Error(),
		}}
	} else {
		_ = data.Set("image_digest", resp.ExporterResponse["containerimage.digest"])
	}

	return diag.Diagnostics{}
}

func readImage(context context.Context, data *schema.ResourceData, meta interface{}) diag.Diagnostics {
	diagnostics := make(diag.Diagnostics, 0)
	digest := data.Get("image_digest")
	buildContext := data.Get("context").(string)
	contextHash, diags := getDirectoryHash(buildContext)

	if len(diags) > 0 {
		return diags
	} else {
		data.SetId(contextHash)
	}

	provider := meta.(TerraformProviderBuildkit)
	expected_targets := data.Get("publish_target").([]interface{})
	actual_targets := make([]interface{}, 0)

	diagnostics = make(diag.Diagnostics, 0)

	for _, target := range expected_targets {
		casted := target.(map[string]interface{})
		hostname := casted["registry"].(string)
		auth := provider.registry_auth[hostname]

		hash, err := crane.Digest(casted["name"].(string)+":"+casted["tag"].(string), crane.WithAuth(&authn.Basic{
			Username: auth.username,
			Password: auth.password,
		}))

		if err != nil {
			// an error is expected if it just doesn't exist on this registry yet at the expected tag
			if te, ok := err.(*transport.Error); ok {
				if te.StatusCode == 404 {
					continue
				}
			}

			diagnostics = append(diagnostics, diag.Diagnostic{
				Severity: diag.Error,
				Summary:  err.Error(),
			})
		}

		if hash == digest {
			actual_targets = append(actual_targets, target)
		}
	}

	if len(diagnostics) > 0 {
		return diagnostics
	} else {
		data.Set("publish_targets", actual_targets)
		data.Set("context_hash", contextHash)
	}

	return diagnostics
}

func updateImage(context context.Context, data *schema.ResourceData, meta interface{}) diag.Diagnostics {
	diagnostics := make(diag.Diagnostics, 0)

	return diagnostics
}

func deleteImage(context context.Context, data *schema.ResourceData, meta interface{}) diag.Diagnostics {
	diagnostics := make(diag.Diagnostics, 0)

	return diagnostics
}
