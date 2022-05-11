package buildkit

import (
	"bytes"
	"encoding/json"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"io/ioutil"
	"regexp"
	"strings"
)

type ImageDescriptor struct {
	tag       string
	digest    string
	platforms []string
	labels    map[string]string
}

type ImageFilters struct {
	tag_pattern         string
	most_recent_only    bool
	supported_platforms []string
	labels              map[string]string
}

func listImages(repository string, auth RegistryAuth, filters ImageFilters) ([]ImageDescriptor, diag.Diagnostics) {

	result := make([]ImageDescriptor, 0)

	diagnostics := diag.Diagnostics{}

	tags, err := crane.ListTags(repository, crane.WithAuth(&authn.Basic{
		Username: auth.username,
		Password: auth.password,
	}))

	if err != nil {
		return result, append(diagnostics, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  err.Error(),
		})
	}

	pattern := regexp.QuoteMeta(filters.tag_pattern)

	if strings.HasPrefix(filters.tag_pattern, "/") && strings.HasSuffix(filters.tag_pattern, "/") {
		pattern = strings.Trim(filters.tag_pattern, "/")
	}

	regex := regexp.MustCompile(pattern)

	options := makeOptions(crane.WithAuth(&authn.Basic{
		Username: auth.username,
		Password: auth.password,
	}))

	for _, tag := range tags {
		if regex.MatchString(tag) {

			tagReference, err := name.ParseReference(repository + ":" + tag)
			tagDescriptor, err := remote.Get(tagReference, options.Remote...)

			if err != nil {
				diagnostics = append(diagnostics, diag.Diagnostic{
					Severity: diag.Error,
					Summary:  err.Error(),
				})
				continue
			} else {
				if tagDescriptor.MediaType.IsIndex() {

					indexManifestReader := bytes.NewReader(tagDescriptor.Manifest)
					parsedIndexManifest, err := v1.ParseIndexManifest(indexManifestReader)

					if err != nil {
						diagnostics = append(diagnostics, diag.Diagnostic{
							Severity: diag.Error,
							Summary:  err.Error(),
						})
						continue
					}

					for _, manifest := range parsedIndexManifest.Manifests {

						platformAsString := platformToString(manifest.Platform)

						if contains(filters.supported_platforms, platformAsString) {
							imageManifestReference := tagReference.Context().Digest(manifest.Digest.String())

							if err != nil {
								diagnostics = append(diagnostics, diag.Diagnostic{
									Severity: diag.Error,
									Summary:  err.Error(),
								})
								continue
							}

							imageManifestDescriptor, err := remote.Get(imageManifestReference, options.Remote...)
							imageManifestReader := bytes.NewReader(imageManifestDescriptor.Manifest)
							parsedImageManifest, err := v1.ParseManifest(imageManifestReader)
							if err != nil {
								diagnostics = append(diagnostics, diag.Diagnostic{
									Severity: diag.Error,
									Summary:  err.Error(),
								})
								continue
							}

							imageConfigManifestReference := imageManifestReference.Context().Digest(parsedImageManifest.Config.Digest.String())

							if err != nil {
								diagnostics = append(diagnostics, diag.Diagnostic{
									Severity: diag.Error,
									Summary:  err.Error(),
								})
								continue
							}

							imageConfigLayer, err := remote.Layer(imageConfigManifestReference, options.Remote...)

							if err != nil {
								diagnostics = append(diagnostics, diag.Diagnostic{
									Severity: diag.Error,
									Summary:  err.Error(),
								})
								continue
							}

							imageConfigLayerReader, err := imageConfigLayer.Uncompressed()
							if err != nil {
								diagnostics = append(diagnostics, diag.Diagnostic{
									Severity: diag.Error,
									Summary:  err.Error(),
								})
								continue
							}

							imageConfig := map[string]interface{}{}

							bites, err := ioutil.ReadAll(imageConfigLayerReader)

							if err != nil {
								diagnostics = append(diagnostics, diag.Diagnostic{
									Severity: diag.Error,
									Summary:  err.Error(),
								})
								continue
							}

							err = json.Unmarshal(bites, &imageConfig)

							if err != nil {
								diagnostics = append(diagnostics, diag.Diagnostic{
									Severity: diag.Error,
									Summary:  err.Error(),
								})
								continue
							}

							labels_as_interface := imageConfig["config"].(map[string]interface{})["Labels"].(map[string]interface{})

							labels := map[string]string{}
							for k, v := range labels_as_interface {
								labels[k] = v.(string)
							}

							matches := true

							for k, v := range filters.labels {
								if v != labels[k] {
									matches = false
									break
								}
							}

							if matches {
								result = append(result, ImageDescriptor{
									tag:       tag,
									labels:    labels,
									digest:    tagDescriptor.Digest.String(),
									platforms: []string{platformAsString},
								})
							}
						}
					}

				} else if tagDescriptor.MediaType.IsImage() {

				} else {
					diagnostics = append(diagnostics, diag.Diagnostic{
						Severity: diag.Error,
						Summary:  "Encountered unexpected layer manifest.",
					})
					continue
				}
			}
		}
	}

	return result, diagnostics
}

func platformToString(platform *v1.Platform) string {
	return platform.OS + "/" + platform.Architecture
}
