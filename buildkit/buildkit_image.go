package buildkit

import (
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func buildkitImageResource() *schema.Resource {
	return &schema.Resource{
		CreateContext: createImage2,
		ReadContext:   readImage,
		UpdateContext: updateImage,
		DeleteContext: deleteImage,
		Description:   "A docker image built with buildkit and published to target registries.",
		Schema: map[string]*schema.Schema{
			"publish_target": {
				Type:     schema.TypeList,
				Optional: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"registry": {
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
				},
			},
			"context": {
				Type:     schema.TypeString,
				Required: true,
			},
			"dockerfile": {
				Type:     schema.TypeString,
				Optional: true,
				Default:  "./Dockerfile",
			},
			"platform": {
				Type:     schema.TypeString,
				Optional: true,
				Default:  "local",
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
			"ssh_sockets": {
				Type:     schema.TypeMap,
				ForceNew: true,
				Optional: true,
				Default: map[string]string{
					"default": "$SSH_AUTH_SOCK",
				},
			},
			"context_digest": {
				Type:        schema.TypeString,
				ForceNew:    true,
				Computed:    true,
				Description: "The hash of the context, except files which match a pattern contained in a .dockerignore file (if present).",
			},
			"image_digest": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "The sha256 digest of the docker image. This is the canonical content addressable hash for a docker image.",
			},
		},
	}
}
