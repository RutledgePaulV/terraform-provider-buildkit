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
				Config: step1(),
				Check:  resource.ComposeTestCheckFunc(),
			},
		},
	})
}

func step1() string {
	return fmt.Sprintf(`
		provider buildkit {
			host = "tcp://127.0.0.1:1234"
			registry_auth {
				hostname = "https://index.docker.io/v1/"
				username = "%s"
				password = "%s"
			}
		}

		resource buildkit_image this {
			context = "../examples/basic"
			dockerfile = "../examples/basic/Dockerfile"
			publish_target {
				registry = "https://docker.io"
			    name = "rutledgepaulv/paul-test"
				tag = "latest"
			}
			labels = {
				"paul" = "love"
			}
		}
	`, os.Getenv("DOCKER_USERNAME"), os.Getenv("DOCKER_TOKEN"))
}
