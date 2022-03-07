package buildkit

import (
	"context"
	"github.com/docker/cli/cli/command/image/build"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/archive"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"io"
)

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

	contextDir := ""

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

	options := types.ImageBuildOptions{
		Labels: data.Get("labels").(map[string]string),
	}

	response, err := cli.ImageBuild(context, tarHandle, options)
	defer response.Body.Close()

	if err != nil {
		return append(diagnostics, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  "Encountered error while sending files to build docker image.",
		})
	}

	asText, err := io.ReadAll(response.Body)

	if err != nil {
		return append(diagnostics, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  "Encountered error parsing response from docker build process.",
		})
	}

	print(asText)

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
