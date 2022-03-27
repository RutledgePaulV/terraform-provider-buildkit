package buildkit

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"github.com/denisbrodbeck/machineid"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/session"
	"github.com/moby/buildkit/session/filesync"
	"github.com/moby/buildkit/session/secrets/secretsprovider"
	"github.com/moby/buildkit/session/sshforward/sshprovider"
	fsutiltypes "github.com/tonistiigi/fsutil/types"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	validPrefixes = map[string][]string{
		"url":       {"http://", "https://"},
		"git":       {"git://", "github.com/", "git@"},
		"transport": {"tcp://", "tcp+tls://", "udp://", "unix://", "unixgram://"},
	}
	urlPathWithFragmentSuffix = regexp.MustCompile(".git(?:#.+)?$")
)

// IsURL returns true if the provided str is an HTTP(S) URL.
func IsURL(str string) bool {
	return checkURL(str, "url")
}

// IsGitURL returns true if the provided str is a git repository URL.
func IsGitURL(str string) bool {
	if IsURL(str) && urlPathWithFragmentSuffix.MatchString(str) {
		return true
	}
	return checkURL(str, "git")
}

// IsTransportURL returns true if the provided str is a transport (tcp, tcp+tls, udp, unix) URL.
func IsTransportURL(str string) bool {
	return checkURL(str, "transport")
}

func checkURL(str, kind string) bool {
	for _, prefix := range validPrefixes[kind] {
		if strings.HasPrefix(str, prefix) {
			return true
		}
	}
	return false
}

func getCompiledTags(data *schema.ResourceData) []string {
	tags := make([]string, 0)
	publish_targets := data.Get("publish_target").([]interface{})
	for _, x := range publish_targets {
		casted := x.(map[string]interface{})
		withoutProtocol := strings.ReplaceAll(casted["registry"].(string), "https://", "")
		tags = append(tags, fmt.Sprintf("%s/%s:%s", withoutProtocol, casted["name"].(string), casted["tag"].(string)))
	}
	return tags
}

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

func getCompiledAuthConfigs(meta TerraformProviderBuildkit) map[string]map[string]string {
	result := map[string]map[string]string{}
	for k, v := range meta.registry_auth {
		result[k] = map[string]string{
			"Username":      v.username,
			"Password":      v.password,
			"ServerAddress": v.hostname,
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
				Severity: diag.Error,
				Summary:  "Failed to base64 decode a secret.",
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

func createImage2(context context.Context, data *schema.ResourceData, meta interface{}) diag.Diagnostics {

	buildContext := data.Get("context").(string)
	dockerfile := data.Get("dockerfile").(string)
	provider := meta.(TerraformProviderBuildkit)
	//platform := data.Get("platform").(string)
	secrets, diags := getSecrets(data)
	sshAgents := getSSHAgents(data)
	outputs := getCompiledOutputs(data)

	sessionProviders := make([]session.Attachable, 0)

	if len(diags) > 0 {
		return diags
	}

	dockerAuthProvider := NewDockerAuthProvider(provider.registry_auth)
	secretsProvider := getSecretsProvider(secrets)
	sshProvider, diags := getSSHProvider(sshAgents)

	if len(diags) > 0 {
		return diags
	}

	sessionProviders = append(sessionProviders, dockerAuthProvider, secretsProvider, sshProvider)

	cli, err := client.New(context, provider.host, client.WithFailFast())

	if err != nil {
		panic(err)
	}

	result := make(chan *client.SolveStatus)

	go func() {
		print(<-result)
	}()

	resp, err := cli.Solve(context, nil, client.SolveOpt{
		Exports:       outputs,
		Frontend:      "dockerfile.v0",
		FrontendAttrs: map[string]string{},
		LocalDirs: map[string]string{
			"context":    buildContext,
			"dockerfile": filepath.Dir(dockerfile),
		},
		Session: sessionProviders,
	}, result)

	print(resp)

	if IsGitURL(buildContext) || IsURL(buildContext) {
		//options.RemoteContext = buildContext
	}

	return diag.Diagnostics{}
}

func readImage(context context.Context, data *schema.ResourceData, meta interface{}) diag.Diagnostics {
	diagnostics := make(diag.Diagnostics, 0)

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
