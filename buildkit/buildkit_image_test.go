package buildkit

import (
	"fmt"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"os"
	"testing"
)

func TestProvider(t *testing.T) {
	if err := Provider().InternalValidate(); err != nil {
		t.Fatalf("err: %s", err)
	}
}

func TestAccImage_Basic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProviderFactories: map[string]func() (*schema.Provider, error){
			"buildkit": func() (*schema.Provider, error) {
				return Provider(), nil
			},
		},
		Steps: []resource.TestStep{
			{
				Config: step1("basic"),
				Check:  resource.ComposeTestCheckFunc(),
			},
		},
	})
}

func TestAccImage_BasicAddAPublishTarget(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProviderFactories: map[string]func() (*schema.Provider, error){
			"buildkit": func() (*schema.Provider, error) {
				return Provider(), nil
			},
		},
		Steps: []resource.TestStep{
			{
				Config: step1("basic"),
				Check:  resource.ComposeTestCheckFunc(),
			},
			{
				Config: addAPublishTarget("basic"),
				Check:  resource.ComposeTestCheckFunc(),
			},
		},
	})
}

func TestAccImage_Ignore(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProviderFactories: map[string]func() (*schema.Provider, error){
			"buildkit": func() (*schema.Provider, error) {
				return Provider(), nil
			},
		},
		Steps: []resource.TestStep{
			{
				Config: step1("ignore"),
				Check:  resource.ComposeTestCheckFunc(),
			},
		},
	})
}

func TestAccImage_Secrets(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProviderFactories: map[string]func() (*schema.Provider, error){
			"buildkit": func() (*schema.Provider, error) {
				return Provider(), nil
			},
		},
		Steps: []resource.TestStep{
			{
				Config: step1("secrets"),
				Check:  resource.ComposeTestCheckFunc(),
			},
		},
	})
}

func TestAccImage_Ssh(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProviderFactories: map[string]func() (*schema.Provider, error){
			"buildkit": func() (*schema.Provider, error) {
				return Provider(), nil
			},
		},
		Steps: []resource.TestStep{
			{
				Config: step1("ssh"),
				Check:  resource.ComposeTestCheckFunc(),
			},
		},
	})
}

func step1(folder string) string {
	return fmt.Sprintf(`
		provider buildkit {
			buildkit_url = "tcp://127.0.0.1:1234"
			registry_auth {
				registry_url = "https://docker.io"
				username = "%s"
				password = "%s"
			}
		}

		resource buildkit_image this {
			context = "../examples/%s"
			dockerfile = "../examples/%s/Dockerfile"
			platforms = ["linux/amd64", "linux/arm"]
			publish_target {
				registry_url = "https://docker.io"
			    name = "rutledgepaulv/paul-test"
				tag = "%s"
			}
			labels = {
				"paul" = "love"
			}
			secrets = {
				"mysecret" = "sdfasdfasdf"
			}
		}
	`,
		os.Getenv("DOCKER_USERNAME"),
		os.Getenv("DOCKER_TOKEN"),
		folder,
		folder,
		folder)
}

func addAPublishTarget(folder string) string {
	return fmt.Sprintf(`
		provider buildkit {
			buildkit_url = "tcp://127.0.0.1:1234"
			registry_auth {
				registry_url = "https://docker.io"
				username = "%s"
				password = "%s"
			}
		}

		resource buildkit_image this {
			context = "../examples/%s"
			dockerfile = "../examples/%s/Dockerfile"
			platforms = ["linux/amd64", "linux/arm"]
			publish_target {
				registry_url = "https://docker.io"
			    name = "rutledgepaulv/paul-test"
				tag = "%s"
			}
			publish_target {
				registry_url = "https://docker.io"
			    name = "rutledgepaulv/paul-test"
				tag = "%s-diggity"
			}
			labels = {
				"paul" = "love"
			}
			secrets = {
				"mysecret" = "sdfasdfasdf"
			}
		}
	`,
		os.Getenv("DOCKER_USERNAME"),
		os.Getenv("DOCKER_TOKEN"),
		folder,
		folder,
		folder,
		folder)
}
