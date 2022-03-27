package buildkit

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"github.com/containerd/containerd/platforms"
	"github.com/denisbrodbeck/machineid"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/pkg/urlutil"
	"github.com/google/uuid"
	"github.com/hashicorp/go-cty/cty"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/moby/buildkit/session"
	"github.com/moby/buildkit/session/auth/authprovider"
	"github.com/moby/buildkit/session/filesync"
	"github.com/moby/buildkit/session/secrets/secretsprovider"
	"github.com/moby/buildkit/session/sshforward/sshprovider"
	fsutiltypes "github.com/tonistiigi/fsutil/types"
	"github.com/tonistiigi/go-rosetta"
	"os"
	"path/filepath"
	"strings"
)

func getCompiledTags(data *schema.ResourceData) []string {
	tags := make([]string, 0)
	publish_targets := data.Get("publish_target").([]interface{})
	for _, x := range publish_targets {
		casted := x.(map[string]interface{})
		tags = append(tags, fmt.Sprintf("%s/%s:%s", casted["registry"].(string), casted["name"].(string), casted["tag"].(string)))
	}
	return tags
}

func getCompiledOutputs(data *schema.ResourceData) []types.ImageBuildOutput {
	outputs := make([]types.ImageBuildOutput, 0)
	publish_targets := data.Get("publish_target").([]interface{})
	for _, x := range publish_targets {
		casted := x.(map[string]interface{})
		outputs = append(outputs, types.ImageBuildOutput{
			Type: "image",
			Attrs: map[string]string{
				"name": fmt.Sprintf("%s/%s:%s", casted["registry"].(string), casted["name"].(string), casted["tag"].(string)),
				"push": "true",
			},
		})
	}
	return outputs
}

func getCompiledAuthConfigs(meta TerraformProviderBuildkit) map[string]types.AuthConfig {
	result := map[string]types.AuthConfig{}
	for k, v := range meta.registry_auth {
		result[k] = types.AuthConfig{
			Username:      v.username,
			Password:      v.password,
			ServerAddress: v.hostname,
		}
	}
	return result
}

func computeSessionHash(buildContext string) string {
	machineId, err := machineid.ProtectedID("terraform-provider-buildkit")
	if err != nil {
		s := sha256.Sum256([]byte(fmt.Sprintf("%s:%s", uuid.NewString(), buildContext)))
		return hex.EncodeToString(s[:])
	} else {
		s := sha256.Sum256([]byte(fmt.Sprintf("%s:%s", machineId, buildContext)))
		return hex.EncodeToString(s[:])
	}
}

func createBuildKitSession(buildContext string) (*session.Session, diag.Diagnostics) {
	diagnostics := diag.Diagnostics{}
	sessionId := computeSessionHash(buildContext)
	buildkitSession, err := session.NewSession(context.Background(), filepath.Base(buildContext), sessionId)
	if err != nil {
		return nil, append(diagnostics, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  "Failed to create buildkit session.",
		})
	} else {
		return buildkitSession, diagnostics
	}
}

func resetUIDAndGID(_ string, s *fsutiltypes.Stat) bool {
	s.Uid = 0
	s.Gid = 0
	return true
}

func isLocalDir(c string) bool {
	_, err := os.Stat(c)
	return err == nil
}

func getSecretsProvider(secrets map[string][]byte) session.Attachable {
	return secretsprovider.FromMap(secrets)
}

func getLocalContextProvider(buildContext string, dockerfile string) session.Attachable {
	return filesync.NewFSSyncProvider([]filesync.SyncedDir{
		{
			Name: "context",
			Dir:  buildContext,
			Map:  resetUIDAndGID,
		},
		{
			Name: "dockerfile",
			Dir:  dockerfile,
		},
	})
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
				Severity:      diag.Error,
				Summary:       "Failed to base64 decode a secret.",
				AttributePath: cty.IndexStringPath("secrets_base64").IndexString(k),
			})
		}
	}
	return result, diagnostics
}

func getSSHAgents(data *schema.ResourceData) map[string]string {
	result := map[string]string{}
	secrets := data.Get("ssh_sockets").(map[string]interface{})
	for k, v := range secrets {
		value := v.(string)
		if strings.HasPrefix(value, "$") {
			result[k] = os.Getenv(value)
		} else {
			result[k] = value
		}
	}
	return result
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

func getLocalPlatform() string {
	p := platforms.DefaultSpec()
	p.Architecture = rosetta.NativeArch()
	return platforms.Format(p)
}

func createImage2(context context.Context, data *schema.ResourceData, meta interface{}) diag.Diagnostics {

	buildContext := data.Get("context").(string)
	dockerfile := data.Get("dockerfile").(string)
	platform := data.Get("platform").(string)
	secrets, diags := getSecrets(data)
	sshAgents := getSSHAgents(data)
	outputs := getCompiledOutputs(data)

	if len(diags) > 0 {
		return diags
	}

	buildkitSession, diags := createBuildKitSession(buildContext)

	if len(diags) > 0 {
		return diags
	}

	defer buildkitSession.Close()

	// grant access to local directories for sending the build files to the server

	if isLocalDir(buildContext) {
		dockerFileDirectory := filepath.Dir(dockerfile)
		localContextProvider := getLocalContextProvider(buildContext, dockerFileDirectory)
		buildkitSession.Allow(localContextProvider)
	}

	// grant access to local directories for outputting build results

	// configure daemon authentication

	dockerAuthProvider := authprovider.NewDockerAuthProvider(os.Stderr)
	buildkitSession.Allow(dockerAuthProvider)

	// configure secret access
	secretsProvider := getSecretsProvider(secrets)
	buildkitSession.Allow(secretsProvider)

	sshProvider, diags := getSSHProvider(sshAgents)
	if len(diags) > 0 {
		return diags
	}
	buildkitSession.Allow(sshProvider)

	if strings.EqualFold(platform, "local") {
		platform = getLocalPlatform()
	}

	options := types.ImageBuildOptions{
		Tags:        getCompiledTags(data),
		Dockerfile:  filepath.Base(dockerfile),
		BuildArgs:   getCompiledBuildArgs(data),
		AuthConfigs: getCompiledAuthConfigs(meta.(TerraformProviderBuildkit)),
		Labels:      getCompiledLabels(data),
		Version:     types.BuilderBuildKit,
		SessionID:   buildkitSession.ID(),
		Outputs:     outputs,
	}

	if urlutil.IsGitURL(buildContext) || urlutil.IsURL(buildContext) {
		options.RemoteContext = buildContext
	}

	return diag.Diagnostics{}
}
