package buildkit

import (
	"context"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

type RegistryAuth struct {
	hostname string
	username string
	password string
}

type TerraformProviderBuildkit struct {
	host          string
	registry_auth map[string]RegistryAuth
}

func Provider() *schema.Provider {
	return &schema.Provider{
		Schema: map[string]*schema.Schema{
			"host": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "URL for a running buildkit daemon.",
			},
			"registry_auth": {
				Type:     schema.TypeList,
				Optional: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"hostname": {
							Type:     schema.TypeString,
							Required: true,
						},
						"username": {
							Type:     schema.TypeString,
							Required: true,
						},
						"password": {
							Type:      schema.TypeString,
							Sensitive: true,
							Required:  true,
						},
					},
				},
			},
		},
		ResourcesMap: map[string]*schema.Resource{
			"buildkit_image": buildkitImageResource(),
		},
		DataSourcesMap:       map[string]*schema.Resource{},
		ConfigureContextFunc: providerConfigure,
	}
}

func providerConfigure(context context.Context, schema *schema.ResourceData) (interface{}, diag.Diagnostics) {
	registry_auth := schema.Get("registry_auth").([]interface{})

	by_host := make(map[string]RegistryAuth)

	for _, x := range registry_auth {
		casted := x.(map[string]interface{})
		by_host[casted["hostname"].(string)] = RegistryAuth{
			hostname: casted["hostname"].(string),
			username: casted["username"].(string),
			password: casted["password"].(string),
		}
	}

	return TerraformProviderBuildkit{
			registry_auth: by_host,
			host:          schema.Get("host").(string),
		},
		make(diag.Diagnostics, 0)
}
