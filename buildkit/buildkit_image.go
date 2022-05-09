package buildkit

import (
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

var PublishTargetResource = &schema.Resource{
	Schema: map[string]*schema.Schema{
		"registry_url": {
			Type:     schema.TypeString,
			Required: true,
		},
		"name": {
			Type:     schema.TypeString,
			Required: true,
		},
		"tag": {
			Type:     schema.TypeString,
			Required: true,
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
			"publish_target": {
				Type:     schema.TypeSet,
				Optional: true,
				Elem:     PublishTargetResource,
			},
			"context": {
				Type:     schema.TypeString,
				Required: true,
			},
			"dockerfile": {
				Type:     schema.TypeString,
				Required: true,
			},
			"platforms": {
				Type:     schema.TypeSet,
				Required: true,
				ForceNew: true,
				MinItems: 1,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},
			"labels": {
				Type:     schema.TypeMap,
				Default:  map[string]string{},
				ForceNew: true,
				Optional: true,
			},
			"args": {
				Type:     schema.TypeMap,
				Default:  map[string]string{},
				ForceNew: true,
				Optional: true,
			},
			"secrets": {
				Type:      schema.TypeMap,
				Default:   map[string]string{},
				ForceNew:  true,
				Optional:  true,
				Sensitive: true,
			},
			"secrets_base64": {
				Type:      schema.TypeMap,
				Default:   map[string]string{},
				ForceNew:  true,
				Optional:  true,
				Sensitive: true,
			},
			"forward_ssh_agent_socket": {
				Type:     schema.TypeBool,
				ForceNew: true,
				Optional: true,
				Default:  false,
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
