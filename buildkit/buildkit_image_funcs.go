package buildkit

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/docker/cli/cli/command/image/build"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/archive"
	"github.com/docker/go-units"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func getCompiledTags(data *schema.ResourceData) []string {
	tags := make([]string, 0)
	publish_targets := data.Get("publish_targets").([]interface{})
	for _, x := range publish_targets {
		casted := x.(map[string]string)
		tags = append(tags, fmt.Sprintf("%s/%s:%s", casted["registry"], casted["name"], casted["tag"]))
	}
	return tags
}

func getCompiledOutputs(data *schema.ResourceData) []types.ImageBuildOutput {
	outputs := make([]types.ImageBuildOutput, 0)
	publish_targets := data.Get("publish_targets").([]interface{})
	for _, x := range publish_targets {
		casted := x.(map[string]string)
		outputs = append(outputs, types.ImageBuildOutput{
			Type: "image",
			Attrs: map[string]string{
				"name": fmt.Sprintf("%s/%s:%s", casted["registry"], casted["name"], casted["tag"]),
				"push": "true",
			},
		})
	}
	return outputs
}

func getCompiledBuildArgs(data *schema.ResourceData) map[string]*string {
	result := map[string]*string{}
	x := "test"
	result["test"] = &x
	return result
}

func getCompiledAuthConfigs(meta interface{}) map[string]types.AuthConfig {
	result := map[string]types.AuthConfig{}

	return result
}

func getCompiledLabels(data *schema.ResourceData) map[string]string {
	result := map[string]string{}
	for key, x := range data.Get("labels").(map[string]interface{}) {
		casted := x.(string)
		result[key] = casted
	}
	return result
}

func parseLineDelimitedJson(body io.ReadCloser) ([]interface{}, error) {
	result := make([]interface{}, 0)
	decoder := json.NewDecoder(body)
	defer body.Close()
	for decoder.More() {
		var record interface{}
		err := decoder.Decode(&record)
		if err != nil {
			return nil, err
		} else {
			result = append(result, record)
		}
	}
	return result, nil
}

func createImage(context context.Context, data *schema.ResourceData, meta interface{}) diag.Diagnostics {
	diagnostics := make(diag.Diagnostics, 0)

	cli, err := client.NewClientWithOpts()

	if err != nil {
		return append(diagnostics, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  "Encountered error initializing docker client.",
		})
	}

	currentDir, err := os.Getwd()

	if err != nil {
		return append(diagnostics, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  "Encountered error getting current directory.",
		})
	}

	defer cli.Close()

	contextDir := data.Get("context").(string)

	if !strings.HasPrefix(contextDir, "/") {
		contextDir = filepath.Join(currentDir, "../", contextDir)
	}

	excludePatterns, err := build.ReadDockerignore(contextDir)

	tarHandle, err := archive.TarWithOptions(contextDir, &archive.TarOptions{
		ExcludePatterns: excludePatterns,
	})

	if err != nil {
		return append(diagnostics, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  "Encountered error opening temporary tarball file to send to docker daemon for building.",
		})
	}

	defer tarHandle.Close()

	dockerFilePath := data.Get("dockerfile").(string)

	if !strings.HasPrefix(dockerFilePath, "/") {
		dockerFilePath = filepath.Join(contextDir, dockerFilePath)
	}

	dockerFileHandle, err := os.Open(dockerFilePath)

	if err != nil {
		return append(diagnostics, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  "Encountered error opening dockerfile.",
		})
	}

	defer dockerFileHandle.Close()

	dockerFileText, err := io.ReadAll(dockerFileHandle)

	if err != nil {
		return append(diagnostics, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  "Encountered error reading dockerfile.",
		})
	}

	outputs := getCompiledOutputs(data)

	options := types.ImageBuildOptions{
		// options influenced by terraform settings
		Tags:        getCompiledTags(data),
		Dockerfile:  string(dockerFileText),
		BuildArgs:   getCompiledBuildArgs(data),
		AuthConfigs: getCompiledAuthConfigs(meta),
		Context:     tarHandle,
		Labels:      getCompiledLabels(data),
		Version:     types.BuilderBuildKit,
		Outputs:     outputs,

		// unexposed options
		Squash:         false,
		CacheFrom:      make([]string, 0),
		SecurityOpt:    make([]string, 0),
		ExtraHosts:     make([]string, 0),
		Ulimits:        make([]*units.Ulimit, 0),
		SuppressOutput: false,
		RemoteContext:  "",
		NoCache:        false,
		Remove:         false,
		ForceRemove:    false,
		PullParent:     false,
		Isolation:      container.IsolationDefault,
		CPUSetCPUs:     "",
		CPUSetMems:     "",
		CPUShares:      0,
		CPUQuota:       0,
		CPUPeriod:      0,
		Memory:         0,
		MemorySwap:     0,
		CgroupParent:   "",
		NetworkMode:    "",
		ShmSize:        0,
		Target:         "",
		SessionID:      "",
		Platform:       "",
		BuildID:        "",
	}

	response, err := cli.ImageBuild(context, tarHandle, options)

	if response.Body != nil {
		defer response.Body.Close()
	}

	if err != nil {
		return append(diagnostics, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  "Encountered error while sending files to build docker image.",
		})
	}

	if response.Body != nil {
		asText, err := parseLineDelimitedJson(response.Body)

		errorCount := len(diagnostics)

		for _, x := range asText {
			casted := x.(map[string]interface{})
			if val, ok := casted["error"]; ok {
				asJson, _ := json.MarshalIndent(casted, "", "    ")

				diagnostics = append(diagnostics, diag.Diagnostic{
					Severity: diag.Error,
					Summary:  val.(string),
					Detail:   string(asJson),
				})
			}
		}

		if err != nil {
			diagnostics = append(diagnostics, diag.Diagnostic{
				Severity: diag.Error,
				Summary:  "Encountered error parsing response from docker build process.",
			})
		}

		if errorCount < len(diagnostics) {
			return diagnostics
		}

	}

	return diagnostics
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
