package buildkit

import (
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func buildkitImageResource() *schema.Resource {
	return &schema.Resource{
		CreateContext: createImage,
		ReadContext:   readImage,
		UpdateContext: updateImage,
		DeleteContext: deleteImage,
		Description:   "A docker image built with buildkit and published to target registries.",
		Schema: map[string]*schema.Schema{
			"publish_targets": {
				Type:     schema.TypeList,
				MinItems: 1,
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
				Required: false,
				Default:  "./Dockerfile",
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
			"ssh_sockets": {
				Type:     schema.TypeMap,
				ForceNew: true,
				Default: map[string]string{
					"default": "$SSH_AUTH_SOCK",
				},
			},
			"sha256_digest": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "The hash of the image.",
			},
		},
	}
}
