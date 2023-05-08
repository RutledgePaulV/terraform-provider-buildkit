package buildkit

import (
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

var PublishTargetResource = &schema.Resource{
	Schema: map[string]*schema.Schema{
		"registry_url": {
			Type:        schema.TypeString,
			Required:    true,
			Description: "The base url of the registry you want to publish to.",
		},
		"name": {
			Type:        schema.TypeString,
			Required:    true,
			Description: "The name of the repository within the registry you want to publish to.",
		},
		"tag": {
			Type:        schema.TypeString,
			Required:    true,
			Description: "The tag you want to publish this particular build as.",
		},
		"tag_url": {
			Type:        schema.TypeString,
			Computed:    true,
			ForceNew:    true,
			Description: "The tag you want to publish this particular build as.",
		},
		"digest_url": {
			Type:        schema.TypeString,
			Computed:    true,
			ForceNew:    true,
			Description: "The tag you want to publish this particular build as.",
		},
	},
}

var ImageResource = &schema.Resource{
	Schema: map[string]*schema.Schema{
		"name": {
			Type:        schema.TypeString,
			Computed:    true,
			Description: "The repository name of the image.",
		},
		"tag": {
			Type:        schema.TypeString,
			Computed:    true,
			Description: "The tag of the image.",
		},
		"tag_url": {
			Type:        schema.TypeString,
			Computed:    true,
			Description: "The tag-based url for the image.",
		},
		"digest_url": {
			Type:        schema.TypeString,
			Computed:    true,
			Description: "The hash-based url for the image. You should prefer this when you need to point to the exact image.",
		},
		"labels": {
			Type:        schema.TypeMap,
			Elem:        schema.TypeString,
			Computed:    true,
			Description: "Labels that are set in the image metadata.",
		},
		"platform": {
			Type:        schema.TypeString,
			Computed:    true,
			Description: "Platform that is supported by this image.",
		},
	},
}

func buildkitDirectoryHashDataSource() *schema.Resource {
	return &schema.Resource{
		ReadContext: readDirectoryHashDataSource,
		Schema: map[string]*schema.Schema{
			"context": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "Path to the directory that should be used as the docker context.",
			},
			"hash": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "The hash of the directory, excluding any .dockerignore files.",
			},
		},
	}
}

func buildkitImagesDataSource() *schema.Resource {
	return &schema.Resource{
		ReadContext: readImagesDataSource,
		Schema: map[string]*schema.Schema{
			"most_recent_only": {
				Type:        schema.TypeBool,
				Default:     true,
				Optional:    true,
				Description: "Should all images be returned that match the criteria or only the most recent which matches?",
			},
			"images": {
				Type:        schema.TypeList,
				Computed:    true,
				Elem:        ImageResource,
				Description: "The image results of your query.",
			},
			"registry_url": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "The registry url you want to search.",
			},
			"repository_name": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "The repository name you want to search.",
			},
			"tag_pattern": {
				Type:        schema.TypeString,
				Default:     "/.*/",
				Optional:    true,
				Description: "A regex pattern you want to filter tags by.",
			},
			"labels": {
				Type:        schema.TypeMap,
				Default:     map[string]string{},
				Optional:    true,
				Description: "Required label keys / values to filter the returned images by.",
			},
			"supported_platforms": {
				Type:     schema.TypeSet,
				Required: true,
				MinItems: 1,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
				Description: "Required platforms that must be supported by the returned images.",
			},
		},
	}
}

func buildkitImageResource() *schema.Resource {
	return &schema.Resource{
		CreateContext: createImage,
		ReadContext:   readImage,
		UpdateContext: updateImage,
		DeleteContext: deleteImage,
		Description:   "A docker image built with buildkit and published to target registries.",
		Schema: map[string]*schema.Schema{
			"id": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "A unique identifier for the image.",
			},
			"triggers": {
				Type:        schema.TypeMap,
				Optional:    true,
				ForceNew:    true,
				Default:     map[string]string{},
				Description: "A map of strings that will cause a change to the counter when any of the values change.",
			},
			"publish_target": {
				Type:        schema.TypeSet,
				Optional:    true,
				Elem:        PublishTargetResource,
				Description: "Describes a coordinate where you want to publish the image after building.",
			},
			"context": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "Path to the directory that should be used as the docker context.",
			},
			"dockerfile": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "Path to the Dockerfile. For now this is expected to live somewhere within the context dir already.",
			},
			"platforms": {
				Type:     schema.TypeSet,
				Required: true,
				ForceNew: true,
				MinItems: 1,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
				Description: "Target platforms / architectures that should be supported by the image being built by Buildkit.",
			},
			"labels": {
				Type:        schema.TypeMap,
				Default:     map[string]string{},
				ForceNew:    true,
				Optional:    true,
				Description: "Labels that should be added to the metadata f the image being built by Buildkit. Equivalent to LABEL commands in the Dockerfile.",
			},
			"args": {
				Type:        schema.TypeMap,
				Default:     map[string]string{},
				ForceNew:    true,
				Optional:    true,
				Description: "Arguments that should be made available to the image being built by Buildkit. Used to set values for ARG commands in the Dockerfile.",
			},
			"secrets": {
				Type:        schema.TypeMap,
				Default:     map[string]string{},
				ForceNew:    true,
				Optional:    true,
				Sensitive:   true,
				Description: "A map of secrets in key => value form that will be made accessible to the image being built by Buildkit.",
			},
			"secrets_base64": {
				Type:        schema.TypeMap,
				Default:     map[string]string{},
				ForceNew:    true,
				Optional:    true,
				Sensitive:   true,
				Description: "A map of secrets in key => base64_encoded_value form that will be made accessible to the image being built by Buildkit.",
			},
			"forward_ssh_agent_socket": {
				Type:        schema.TypeBool,
				ForceNew:    false,
				Optional:    true,
				Default:     false,
				Description: "Should the host running Terraform make their ssh agent socket available to the image being built by Buildkit?",
			},
			"image_digest": {
				Type:        schema.TypeString,
				ForceNew:    true,
				Computed:    true,
				Description: "The sha256 digest of the docker image. This is the canonical content addressable hash for a docker image.",
			},
		},
	}
}
