package buildkit

import "time"

type Labels map[string]string

type ImageResult struct {
	Name           string
	Registry       string
	Tag            string
	Labels         Labels
	TagUrl         string
	DigestUrl      string
	ImageDigest    string
	Platform       string
	BuildTimestamp time.Time
}

type ImageQuery struct {
	Name       string
	TagPattern string
	Labels     Labels
	Platforms  []string
}

type RegistrationAuthentication struct {
	BaseUrl  string
	Username string
	Password string
}

type Platform struct {
	OperatingSystem string
	Architecture    string
}

type ImageConfigManifest struct {
	Architecture string `json:"architecture"`
	Config       struct {
		Env     []string          `json:"Env"`
		Cmd     []string          `json:"Cmd"`
		Labels  map[string]string `json:"Labels"`
		OnBuild interface{}       `json:"OnBuild"`
	} `json:"config"`
	Created time.Time `json:"created"`
	History []struct {
		Created    time.Time `json:"created"`
		CreatedBy  string    `json:"created_by"`
		EmptyLayer bool      `json:"empty_layer,omitempty"`
		Comment    string    `json:"comment,omitempty"`
	} `json:"history"`
	MobyBuildkitBuildinfoV1 string `json:"moby.buildkit.buildinfo.v1"`
	Os                      string `json:"os"`
	Rootfs                  struct {
		Type    string   `json:"type"`
		DiffIds []string `json:"diff_ids"`
	} `json:"rootfs"`
	Variant string `json:"variant"`
}

type SchemaV1History struct {
	ID              string    `json:"id"`
	Parent          string    `json:"parent"`
	Created         time.Time `json:"created"`
	Container       string    `json:"container"`
	ContainerConfig struct {
		Hostname     string        `json:"Hostname"`
		Domainname   string        `json:"Domainname"`
		User         string        `json:"User"`
		AttachStdin  bool          `json:"AttachStdin"`
		AttachStdout bool          `json:"AttachStdout"`
		AttachStderr bool          `json:"AttachStderr"`
		Tty          bool          `json:"Tty"`
		OpenStdin    bool          `json:"OpenStdin"`
		StdinOnce    bool          `json:"StdinOnce"`
		Env          []string      `json:"Env"`
		Cmd          []string      `json:"Cmd"`
		Image        string        `json:"Image"`
		Volumes      interface{}   `json:"Volumes"`
		WorkingDir   string        `json:"WorkingDir"`
		Entrypoint   interface{}   `json:"Entrypoint"`
		OnBuild      []interface{} `json:"OnBuild"`
		Labels       interface{}   `json:"Labels"`
	} `json:"container_config"`
	DockerVersion string `json:"docker_version"`
	Author        string `json:"author"`
	Config        struct {
		Hostname     string            `json:"Hostname"`
		Domainname   string            `json:"Domainname"`
		User         string            `json:"User"`
		AttachStdin  bool              `json:"AttachStdin"`
		AttachStdout bool              `json:"AttachStdout"`
		AttachStderr bool              `json:"AttachStderr"`
		Tty          bool              `json:"Tty"`
		OpenStdin    bool              `json:"OpenStdin"`
		StdinOnce    bool              `json:"StdinOnce"`
		Env          []string          `json:"Env"`
		Cmd          []string          `json:"Cmd"`
		Image        string            `json:"Image"`
		Volumes      interface{}       `json:"Volumes"`
		WorkingDir   string            `json:"WorkingDir"`
		Entrypoint   interface{}       `json:"Entrypoint"`
		OnBuild      []interface{}     `json:"OnBuild"`
		Labels       map[string]string `json:"Labels"`
	} `json:"config"`
	Architecture string `json:"architecture"`
	Os           string `json:"os"`
}

type SchemaV1 struct {
	SchemaVersion int    `json:"schemaVersion"`
	Name          string `json:"name"`
	Tag           string `json:"tag"`
	Architecture  string `json:"architecture"`
	FsLayers      []struct {
		BlobSum string `json:"blobSum"`
	} `json:"fsLayers"`
	History []struct {
		V1Compatibility string `json:"v1Compatibility"`
	} `json:"history"`
	Signatures []struct {
		Header struct {
			Jwk struct {
				Crv string `json:"crv"`
				Kid string `json:"kid"`
				Kty string `json:"kty"`
				X   string `json:"x"`
				Y   string `json:"y"`
			} `json:"jwk"`
			Alg string `json:"alg"`
		} `json:"header"`
		Signature string `json:"signature"`
		Protected string `json:"protected"`
	} `json:"signatures"`
}
