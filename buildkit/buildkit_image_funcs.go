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
	"github.com/hashicorp/go-uuid"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/session"
	"github.com/moby/buildkit/session/secrets/secretsprovider"
	"github.com/moby/buildkit/session/sshforward/sshprovider"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

func getCompiledOutputs(data *schema.ResourceData) []client.ExportEntry {
	publish_targets := data.Get("publish_target").(*schema.Set).List()
	if len(publish_targets) > 0 {
		names := make([]string, 0)
		for _, x := range publish_targets {
			casted := x.(map[string]interface{})
			registry := casted["registry_url"].(string)
			completeRef := fullImage(registry, casted["name"].(string)+":"+casted["tag"].(string))
			names = append(names, completeRef)
		}
		return append(make([]client.ExportEntry, 0), client.ExportEntry{
			Type: "image",
			Attrs: map[string]string{
				"name": strings.Join(names, ","),
				"push": "true",
			},
		})
	} else {
		return make([]client.ExportEntry, 0)
	}
}

func getSecretsProvider(secrets map[string][]byte) session.Attachable {
	return secretsprovider.FromMap(secrets)
}

func getPlatforms(data *schema.ResourceData) []string {
	platforms := data.Get("platforms").(*schema.Set).List()
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

func merge[K comparable, V interface{}](maps ...map[K]V) map[K]V {
	result := map[K]V{}
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

	if len(diags) > 0 {
		return diags
	}

	id, _ := uuid.GenerateUUID()

	data.SetId(id)

	sessionProviders := make([]session.Attachable, 0)
	dockerAuthProvider := NewDockerAuthProvider(provider.registry_auth)
	secretsProvider := getSecretsProvider(secrets)
	sshProvider, diags := getSSHProvider(sshAgents)

	if len(diags) > 0 {
		return diags
	}

	sessionProviders = append(sessionProviders, dockerAuthProvider, secretsProvider, sshProvider)

	cli, err := client.New(context.Background(), provider.buildkit_url, client.WithFailFast())

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

	cacheExports := []client.CacheOptionsEntry{
		client.CacheOptionsEntry{
			Type:  "inline",
			Attrs: make(map[string]string, 0),
		},
	}
	cacheImports := make([]client.CacheOptionsEntry, 0)

	cacheRef := data.Get("cache_from").(string)
	if cacheRef != "" {
		cacheImports = append(cacheImports, client.CacheOptionsEntry{
			Type: "registry",
			Attrs: map[string]string{
				"ref": cacheRef,
			},
		})
	}

	resp, err := cli.Solve(ctx, nil, client.SolveOpt{
		CacheExports: cacheExports,
		CacheImports: cacheImports,
		Exports:      outputs,
		Frontend:     "dockerfile.v0",
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
		publish_targets := data.Get("publish_target").(*schema.Set).List()
		new_targets := []interface{}{}

		diags := diag.Diagnostics{}
		for _, x := range publish_targets {
			casted := x.(map[string]interface{})
			new_target := merge(map[string]interface{}{}, casted)
			registry := casted["registry_url"].(string)
			completeRef := fullImage(registry, casted["name"].(string)+":"+casted["tag"].(string))
			hash, err := getRemoteImageHash(completeRef, provider.registry_auth[registry])
			if err != nil {
				diags = append(diags, diag.Diagnostic{
					Severity: diag.Error,
					Summary:  err.Error(),
				})
			}
			new_target["tag_url"] = completeRef
			new_target["digest_url"] = fullImage(registry, casted["name"].(string)+"@"+hash)

			new_targets = append(new_targets, new_target)
		}

		if len(diags) > 0 {
			return diags
		}

		fun := schema.HashResource(PublishTargetResource)
		asSet := schema.NewSet(fun, new_targets)
		data.Set("publish_target", asSet)
	}

	return diag.Diagnostics{}
}

func readImage(context context.Context, data *schema.ResourceData, meta interface{}) diag.Diagnostics {
	diagnostics := make(diag.Diagnostics, 0)

	provider := meta.(TerraformProviderBuildkit)
	expected_targets := data.Get("publish_target").(*schema.Set).List()
	actual_targets := make([]interface{}, 0)

	diagnostics = make(diag.Diagnostics, 0)

	for _, target := range expected_targets {
		casted := target.(map[string]interface{})
		hostname := casted["registry_url"].(string)
		auth := provider.registry_auth[hostname]

		qualified := fullImage(hostname, casted["name"].(string)+":"+casted["tag"].(string))
		hash, err := getRemoteImageHash(qualified, auth)

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

		casted["digest_url"] = hash
		actual_targets = append(actual_targets, target)
	}

	if len(diagnostics) > 0 {
		return diagnostics
	} else {
		if !reflect.DeepEqual(expected_targets, actual_targets) {
			fun := schema.HashResource(PublishTargetResource)
			asSet := schema.NewSet(fun, actual_targets)
			data.Set("publish_target", asSet)
		}
	}

	return diagnostics
}

func getRemoteImageHash(qualified string, auth RegistryAuth) (string, error) {
	return crane.Digest(qualified, crane.WithAuth(&authn.Basic{
		Username: auth.username,
		Password: auth.password,
	}))
}

func updateImage(context context.Context, data *schema.ResourceData, meta interface{}) diag.Diagnostics {

	changeKeys := []string{
		"secrets",
		"labels",
		"args",
		"platforms",
		"publish_target",
		"triggers",
		"secrets_base64",
	}

	for _, k := range changeKeys {
		if data.HasChange(k) {
			return createImage(context, data, meta)
		}
	}

	return diag.Diagnostics{}
}

func deleteImage(context context.Context, data *schema.ResourceData, meta interface{}) diag.Diagnostics {
	diagnostics := make(diag.Diagnostics, 0)

	return diagnostics
}

func fullImage(registry string, repository string) string {
	return strings.TrimPrefix(strings.TrimPrefix(registry, "https://"), "http://") + "/" + repository
}

func readDirectoryHashDataSource(context context.Context, data *schema.ResourceData, meta interface{}) diag.Diagnostics {
	diagnostics := make(diag.Diagnostics, 0)

	dir := data.Get("context").(string)
	hash, err := getDirectoryHash(dir)

	if hash == "" {
		return err
	}

	id, _ := uuid.GenerateUUID()
	data.SetId(id)
	data.Set("hash", hash)

	return diagnostics
}

func readImagesDataSource(context context.Context, data *schema.ResourceData, meta interface{}) diag.Diagnostics {

	labels_as_interface := data.Get("labels").(map[string]interface{})
	supported_platforms_as_interface := data.Get("supported_platforms").(*schema.Set).List()

	labels := map[string]string{}
	for k, v := range labels_as_interface {
		labels[k] = v.(string)
	}

	supported_platforms := []string{}
	for _, x := range supported_platforms_as_interface {
		supported_platforms = append(supported_platforms, x.(string))
	}

	most_recent_only := data.Get("most_recent_only").(bool)

	registry_url := data.Get("registry_url").(string)
	repository_name := data.Get("repository_name").(string)
	tag_pattern := data.Get("tag_pattern").(string)
	provider := meta.(TerraformProviderBuildkit)
	auth := provider.registry_auth[registry_url]

	repo := fullImage(registry_url, repository_name)

	results, err := query(context, auth, ImageQuery{
		Name:       repo,
		TagPattern: tag_pattern,
		Labels:     labels,
		Platforms:  supported_platforms,
	})

	if err != nil {
		return diag.Diagnostics{diag.Diagnostic{
			Severity: diag.Error,
			Summary:  err.Error(),
		}}
	}

	if most_recent_only {
		if len(results) > 1 {
			results = results[:1]
		}
	}

	id, _ := uuid.GenerateUUID()

	data.SetId(id)
	asMaps := descriptorsToMaps(results)
	data.Set("images", asMaps)

	return diag.Diagnostics{}
}

func descriptorsToMaps(data []ImageResult) []map[string]interface{} {
	results := make([]map[string]interface{}, 0)
	for _, x := range data {
		labels := map[string]interface{}{}
		for k, v := range x.Labels {
			labels[k] = v
		}

		result := map[string]interface{}{
			"name":       x.Name,
			"tag":        x.Tag,
			"tag_url":    x.TagUrl,
			"digest_url": x.DigestUrl,
			"labels":     labels,
			"platform":   x.Platform,
		}
		results = append(results, result)
	}
	return results
}
