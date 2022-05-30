package buildkit

import (
	"context"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

type RegistryAuth struct {
	registry_url string
	username     string
	password     string
}

type TerraformProviderBuildkit struct {
	buildkit_url  string
	registry_auth map[string]RegistryAuth
}

func Provider() *schema.Provider {
	return &schema.Provider{
		Schema: map[string]*schema.Schema{
			"buildkit_url": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "URL for a running buildkit daemon.",
			},
			"registry_auth": {
				Type:     schema.TypeSet,
				Optional: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"registry_url": {
							Type:        schema.TypeString,
							Required:    true,
							Description: "The base url of the registry you want to support communicating with.",
						},
						"username": {
							Type:        schema.TypeString,
							Required:    true,
							Description: "The username you want to use to authenticate to the registry.",
						},
						"password": {
							Type:        schema.TypeString,
							Sensitive:   true,
							Required:    true,
							Description: "The password for authenticating to the registry as `username`.",
						},
					},
				},
			},
		},
		ResourcesMap: map[string]*schema.Resource{
			"buildkit_image": buildkitImageResource(),
		},
		DataSourcesMap: map[string]*schema.Resource{
			"buildkit_images": buildkitImagesDataSource(),
		},
		ConfigureContextFunc: providerConfigure,
	}
}

func providerConfigure(context context.Context, data *schema.ResourceData) (interface{}, diag.Diagnostics) {
	registry_auth := data.Get("registry_auth").(*schema.Set).List()

	by_host := make(map[string]RegistryAuth)

	for _, x := range registry_auth {
		casted := x.(map[string]interface{})
		by_host[casted["registry_url"].(string)] = RegistryAuth{
			registry_url: casted["registry_url"].(string),
			username:     casted["username"].(string),
			password:     casted["password"].(string),
		}
	}

	return TerraformProviderBuildkit{
			registry_auth: by_host,
			buildkit_url:  data.Get("buildkit_url").(string),
		},
		make(diag.Diagnostics, 0)
}
