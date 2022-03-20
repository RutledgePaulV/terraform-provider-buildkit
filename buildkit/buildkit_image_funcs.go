package buildkit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/docker/cli/cli/command/image/build"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/archive"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/uuid"
	"io"
	"path/filepath"
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

func getTarHandle(contextDir string) (io.ReadCloser, diag.Diagnostics) {

	feedback := diag.Diagnostics{}

	contextDir, _ = filepath.Abs(contextDir)

	excludePatterns, err := build.ReadDockerignore(contextDir)

	if err != nil {
		return nil, append(feedback, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  fmt.Sprintf("Could not open .dockerignore file in directory '%s'.", contextDir),
			Detail:   err.Error(),
		})
	}

	tarHandle, err := archive.TarWithOptions(contextDir, &archive.TarOptions{
		Compression:     archive.Gzip,
		ExcludePatterns: excludePatterns,
	})

	if err != nil {
		return nil, append(feedback, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  fmt.Sprintf("Encountered error opening archive of context directory '%s'.", contextDir),
			Detail:   err.Error(),
		})
	}

	return tarHandle, feedback
}

func parseResponse(source io.ReadCloser) ([]interface{}, diag.Diagnostics) {
	feedback := diag.Diagnostics{}
	results := make([]interface{}, 0)

	defer source.Close()
	asText, err := parseLineDelimitedJson(source)

	if err != nil {
		return results, append(feedback, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  "Error parsing line delimited json response from external process.",
			Detail:   err.Error(),
		})
	} else {
		for _, x := range asText {
			casted := x.(map[string]interface{})
			if val, ok := casted["error"]; ok {
				asJson, _ := json.MarshalIndent(casted, "", "    ")
				feedback = append(feedback, diag.Diagnostic{
					Severity: diag.Error,
					Summary:  val.(string),
					Detail:   string(asJson),
				})
			} else {
				results = append(results, casted)
			}
		}
	}

	return results, feedback
}

func getImageBySha(context context.Context, cli *client.Client, digest string) (*types.ImageSummary, diag.Diagnostics) {
	feedback := diag.Diagnostics{}
	imageDetails, _, err := cli.ImageInspectWithRaw(context, digest)

	if err != nil {
		return nil, append(feedback, diag.Diagnostic{})
	}

	return &types.ImageSummary{
		Containers:  0,
		Created:     0,
		ID:          imageDetails.ID,
		Labels:      imageDetails.Config.Labels,
		ParentID:    imageDetails.Parent,
		RepoDigests: imageDetails.RepoDigests,
		RepoTags:    imageDetails.RepoTags,
		SharedSize:  imageDetails.Size,
		Size:        imageDetails.Size,
		VirtualSize: imageDetails.VirtualSize,
	}, feedback
}

func getImageByTag(context context.Context, cli *client.Client, tag string) (*types.ImageSummary, diag.Diagnostics) {
	feedback := diag.Diagnostics{}

	images, err := cli.ImageList(context, types.ImageListOptions{
		Filters: filters.NewArgs(filters.KeyValuePair{
			Key:   "reference",
			Value: tag,
		}),
	})

	if err != nil {
		return nil, append(feedback, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  "Experienced an error when trying to find an image by tag.",
			Detail:   err.Error(),
		})
	}

	if len(images) > 1 {
		return nil, append(feedback, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  "After building docker image more than one image was found with an internal tag that should've been unique.",
		})
	} else if len(images) < 1 {
		return nil, append(feedback, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  "After building docker image, no images could be found with the expected internal tag.",
		})
	} else {
		return &images[0], feedback
	}
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

	defer cli.Close()

	tarHandle, diags := getTarHandle(data.Get("context").(string))

	if len(diags) > 0 {
		return append(diagnostics, diags...)
	}

	defer tarHandle.Close()
	hash := sha256.New()
	teedTarReader := io.TeeReader(tarHandle, hash)

	dockerFilePath := data.Get("dockerfile").(string)
	internalTag := "terraform-provider-buildkit:" + uuid.GenerateUUID()
	tags := append(getCompiledTags(data), internalTag)
	outputs := getCompiledOutputs(data)

	options := types.ImageBuildOptions{
		Tags:        tags,
		Dockerfile:  dockerFilePath,
		BuildArgs:   getCompiledBuildArgs(data),
		AuthConfigs: getCompiledAuthConfigs(meta),
		Context:     teedTarReader,
		Labels:      getCompiledLabels(data),
		Version:     types.BuilderBuildKit,
		Outputs:     outputs,
	}

	response, err := cli.ImageBuild(context, tarHandle, options)

	if err != nil {
		return append(diagnostics, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  "Encountered error from daemon when attempting to build docker image.",
			Detail:   err.Error(),
		})
	}

	if response.Body != nil {
		_, diags := parseResponse(response.Body)

		if len(diags) > 0 {
			return append(diagnostics, diags...)
		}
	}

	image, diags := getImageByTag(context, cli, internalTag)

	if len(diags) > 0 {
		return append(diagnostics, diags...)
	}

	data.SetId(image.ID)
	_ = data.Set("image_digest", image.ID)
	contextDigest := hex.EncodeToString(hash.Sum(nil))
	_ = data.Set("context_digest", contextDigest)

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
