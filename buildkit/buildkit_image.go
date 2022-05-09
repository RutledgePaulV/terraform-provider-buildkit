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
	},
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
				Description: "The hash of the context, except files which match a pattern contained in a .dockerignore file (if present).",
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
			"context_digest": {
				Type:        schema.TypeString,
				ForceNew:    true,
				Computed:    true,
				Description: "The hash of the context, except files which match a pattern contained in a .dockerignore file (if present).",
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
