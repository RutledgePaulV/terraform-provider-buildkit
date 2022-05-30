package buildkit

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"io/ioutil"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

func mergeChannels[K interface{}](channels []chan K) chan K {
	out := make(chan K)
	var wg sync.WaitGroup
	wg.Add(len(channels))
	for _, c := range channels {
		go func(c <-chan K) {
			for v := range c {
				out <- v
			}
			wg.Done()
		}(c)
	}
	go func() {
		wg.Wait()
		close(out)
	}()
	return out
}

func sinkChannels[K interface{}](resultsIn chan K, errorsIn chan error) ([]K, error) {
	results := make([]K, 0)
	for {
		select {
		case result, ok := <-resultsIn:
			if ok {
				results = append(results, result)
			} else {
				return results, nil
			}
		case e, ok := <-errorsIn:
			if ok {
				close(resultsIn)
				close(errorsIn)
				return results, e
			} else {
				for {
					select {
					case result, ok := <-resultsIn:
						if ok {
							results = append(results, result)
						} else {
							return results, nil
						}
					}
				}
			}
		}
	}
}

func copyChannels[K interface{}](resultsOut chan K, resultsIn chan K, errorsOut chan error, errorsIn chan error) {
	for {
		select {
		case result, ok := <-resultsIn:
			if ok {
				resultsOut <- result
			} else {
				close(errorsOut)
				close(resultsOut)
				return
			}
		case e, ok := <-errorsIn:
			if ok {
				errorsOut <- e
				close(errorsIn)
				close(errorsOut)
				close(resultsIn)
				close(resultsOut)
				return
			} else {
				close(errorsOut)
				for {
					select {
					case result, ok := <-resultsIn:
						if ok {
							resultsOut <- result
						} else {
							close(resultsOut)
							return
						}
					}
				}
			}
		}
	}
}

func query(ctx context.Context, auth RegistryAuth, query ImageQuery) ([]ImageResult, error) {

	tags, err := crane.ListTags(query.Name, crane.WithAuth(&authn.Basic{
		Username: auth.username,
		Password: auth.password,
	}))

	if err != nil {
		return []ImageResult{}, err
	}

	matchingTags := filterTags(tags, query.TagPattern)

	if len(matchingTags) == 0 {
		return []ImageResult{}, nil
	}

	errorChannels := make([]chan error, 0)
	resultChannels := make([]chan ImageResult, 0)

	for _, tag := range matchingTags {
		childResults, childErrors := queryOne(ctx, auth, query, tag)
		errorChannels = append(errorChannels, childErrors)
		resultChannels = append(resultChannels, childResults)
	}

	resultsChannel := mergeChannels(resultChannels)
	errorsChannel := mergeChannels(errorChannels)

	results, err := sinkChannels(resultsChannel, errorsChannel)

	if err == nil {
		results = filterLabels(results, query.Labels)
	}

	sort.SliceStable(results, func(i, j int) bool {
		if results[i].BuildTimestamp.Before(results[j].BuildTimestamp) {
			return false
		}
		if results[i].BuildTimestamp.After(results[j].BuildTimestamp) {
			return true
		}
		return results[i].ImageDigest > results[j].ImageDigest
	})

	return results, err
}

func queryOne(ctx context.Context, auth RegistryAuth, query ImageQuery, tag string) (chan ImageResult, chan error) {
	results := make(chan ImageResult)
	errors := make(chan error)

	go func() {

		tagReference, err := name.ParseReference(query.Name + ":" + tag)

		if err != nil {
			errors <- err
			close(results)
			close(errors)
			return
		}

		tagDescriptor, err := remote.Get(tagReference, makeOptions(crane.WithAuth(&authn.Basic{
			Username: auth.username,
			Password: auth.password,
		})).Remote...)

		if err != nil {
			errors <- err
			close(results)
			close(errors)
			return
		}

		if isV2IndexManifest(tagDescriptor.MediaType) {

			indexManifestReader := bytes.NewReader(tagDescriptor.Manifest)
			parsedIndexManifest, err := v1.ParseIndexManifest(indexManifestReader)

			if err != nil {
				errors <- err
				close(results)
				close(errors)
				return
			}

			childResults := make([]chan ImageResult, 0)
			childErrors := make([]chan error, 0)

			for _, indexManifest := range parsedIndexManifest.Manifests {
				if isSupportedPlatform(query.Platforms, indexManifest.Platform) {
					childResult := make(chan ImageResult)
					childError := make(chan error)
					childResults = append(childResults, childResult)
					childErrors = append(childErrors, childError)
					go func(indexManifest v1.Descriptor) {
						imageManifestReference := tagReference.Context().Digest(indexManifest.Digest.String())

						imageManifestDescriptor, err := remote.Get(imageManifestReference, makeOptions(crane.WithAuth(&authn.Basic{
							Username: auth.username,
							Password: auth.password,
						})).Remote...)

						if err != nil {
							childError <- err
							close(childResult)
							close(childError)
							return
						}

						result, err := processManifest(tagReference, imageManifestDescriptor.Manifest, auth)

						if err != nil {
							childError <- err
							close(childResult)
							close(childError)
							return
						}

						childResult <- *result
						close(childResult)
						close(childError)

					}(indexManifest)
				}
			}

			childResult := mergeChannels(childResults)
			childError := mergeChannels(childErrors)

			copyChannels(results, childResult, errors, childError)

		} else if isV2ImageManifest(tagDescriptor.MediaType) {

			result, err := processManifest(tagReference, tagDescriptor.Manifest, auth)

			if err != nil {
				errors <- err
				close(results)
				close(errors)
				return
			}

			results <- *result
			close(results)
			close(errors)

		} else if isV1ImageManifest(tagDescriptor.MediaType) {
			imageManifest := SchemaV1{}
			err = json.Unmarshal(tagDescriptor.Manifest, &imageManifest)
			lastLayer := imageManifest.History[0].V1Compatibility
			layerManifest := SchemaV1History{}
			err = json.Unmarshal([]byte(lastLayer), &layerManifest)

			if err != nil {
				errors <- err
				close(results)
				close(errors)
				return
			}

			digest, err := crane.Digest(tagReference.String(), crane.WithAuth(&authn.Basic{
				Username: auth.username,
				Password: auth.password,
			}))

			if err != nil {
				errors <- err
				close(results)
				close(errors)
				return
			}

			results <- ImageResult{
				Name:           tagReference.Context().RepositoryStr(),
				Registry:       tagReference.Context().RegistryStr(),
				Tag:            tagReference.Identifier(),
				Labels:         normalize(layerManifest.Config.Labels),
				TagUrl:         tagReference.Name(),
				DigestUrl:      tagReference.Context().Digest(digest).String(),
				ImageDigest:    layerManifest.Config.Image,
				Platform:       layerManifest.Os + "/" + layerManifest.Architecture,
				BuildTimestamp: layerManifest.Created.UTC().Round(time.Second),
			}

			close(results)
			close(errors)
		}
	}()

	return results, errors
}

func processManifest(reference name.Reference, manifest []byte, auth RegistryAuth) (*ImageResult, error) {

	imageManifestReader := bytes.NewReader(manifest)
	parsedImageManifest, err := v1.ParseManifest(imageManifestReader)
	if err != nil {
		return nil, err
	}

	imageConfigManifestReference := reference.Context().Digest(parsedImageManifest.Config.Digest.String())
	imageConfigLayer, err := remote.Layer(imageConfigManifestReference, makeOptions(crane.WithAuth(&authn.Basic{
		Username: auth.username,
		Password: auth.password,
	})).Remote...)
	if err != nil {
		return nil, err
	}

	imageConfigLayerReader, err := imageConfigLayer.Uncompressed()
	if err != nil {
		return nil, err
	}

	imageConfig := ImageConfigManifest{}
	bites, err := ioutil.ReadAll(imageConfigLayerReader)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(bites, &imageConfig)
	if err != nil {
		return nil, err
	}

	digest, err := crane.Digest(reference.String(), crane.WithAuth(&authn.Basic{
		Username: auth.username,
		Password: auth.password,
	}))

	if err != nil {
		return nil, err
	}

	return &ImageResult{
		Name:           reference.Context().RepositoryStr(),
		Registry:       reference.Context().RegistryStr(),
		Tag:            reference.Identifier(),
		Labels:         normalize(imageConfig.Config.Labels),
		TagUrl:         reference.Name(),
		DigestUrl:      reference.Context().Digest(digest).String(),
		ImageDigest:    parsedImageManifest.Config.Digest.String(),
		Platform:       imageConfig.Os + "/" + imageConfig.Architecture,
		BuildTimestamp: imageConfig.Created.UTC().Round(time.Second),
	}, nil

}

func normalize[K comparable, V interface{}](x map[K]V) map[K]V {
	if x == nil {
		return map[K]V{}
	} else {
		return x
	}
}

func parseGroups(re *regexp.Regexp, s string) map[string]string {
	match := re.FindStringSubmatch(s)
	result := map[string]string{}
	for i, n := range re.SubexpNames() {
		if i != 0 && n != "" {
			result[n] = match[i]
		}
	}
	return result
}

func parsePlatform(platform string) Platform {
	re := regexp.MustCompile(`(?P<os>[^/]+)/(?P<architecture>[^/]+)`)
	groups := parseGroups(re, platform)
	return Platform{
		OperatingSystem: groups["os"],
		Architecture:    groups["architecture"],
	}
}

func isSupportedPlatform(requiredPlatforms []string, platform *v1.Platform) bool {
	if len(requiredPlatforms) == 0 {
		return true
	}
	for _, x := range requiredPlatforms {
		parsed := parsePlatform(x)
		if strings.EqualFold(parsed.OperatingSystem, platform.OS) &&
			strings.EqualFold(parsed.Architecture, platform.Architecture) {
			return true
		}
	}
	return false
}

func isV2IndexManifest(kind types.MediaType) bool {
	return kind.IsIndex()
}

func isV2ImageManifest(kind types.MediaType) bool {
	return kind.IsImage()
}

func isV1ImageManifest(kind types.MediaType) bool {
	return kind == types.DockerManifestSchema1Signed || kind == types.DockerManifestSchema1
}

func filterLabels(images []ImageResult, labels Labels) []ImageResult {
	results := make([]ImageResult, 0)
	for _, image := range images {
		matches := true
		for k, v := range labels {
			if image.Labels[k] != v {
				matches = false
				break
			}
		}
		if matches {
			results = append(results, image)
		}
	}
	return results
}

func filterTags(tags []string, tagPattern string) []string {

	var regex *regexp.Regexp
	result := []string{}

	if strings.HasPrefix(tagPattern, "/") && strings.HasSuffix(tagPattern, "/") {
		regex = regexp.MustCompile(strings.Trim(tagPattern, "/"))
	} else {
		regex = regexp.MustCompile("^" + regexp.QuoteMeta(tagPattern) + "$")
	}

	for _, x := range tags {
		if regex.MatchString(x) {
			result = append(result, x)
		}
	}

	return result
}

func makeOptions(opts ...crane.Option) crane.Options {
	opt := crane.Options{
		Remote: []remote.Option{
			remote.WithAuthFromKeychain(authn.DefaultKeychain),
			//remote.WithContext(ctx),
		},
	}
	for _, o := range opts {
		o(&opt)
	}
	return opt
}
