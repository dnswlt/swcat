package plugins

import (
	"slices"
	"strings"

	"golang.org/x/mod/semver"
)

// Some well-known annotation keys used by multiple plugins.
const (
	// (Inherited) annotation in which to find the JFrog Docker repository name.
	JFrogDockerRepositoryAnnotation = "jfrog.com/docker-repository"
	// (Inherited) annotation in which to find the JFrog Artifactory repository name.
	JFrogRepositoryAnnotation = "jfrog.com/repository"
)

// semverNormalize returns tag with a "v" prefix for semver comparison,
// leaving tags that already have one unchanged.
func semverNormalize(tag string) string {
	if strings.HasPrefix(tag, "v") {
		return tag
	}
	return "v" + tag
}

// latestSemverVersions filters tags to those with valid semver, sorts them in
// descending order, and returns the top n original tags (preserving their
// original "v"-prefix or lack thereof).
func latestSemverVersions(tags []string, n int) []string {
	var valid []string
	for _, tag := range tags {
		if semver.IsValid(semverNormalize(tag)) {
			valid = append(valid, tag)
		}
	}
	slices.SortFunc(valid, func(v1, v2 string) int {
		return semver.Compare(semverNormalize(v2), semverNormalize(v1)) // descending
	})
	if len(valid) > n {
		valid = valid[:n]
	}
	return valid
}
