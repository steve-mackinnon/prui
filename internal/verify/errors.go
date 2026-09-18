package verify

import "errors"

var errUnsafeArtifactPath = errors.New("artifact path must be a relative path within the artifact directory")
var errUnsafeScreenName = errors.New("screen name must contain only letters, digits, underscores, or hyphens")
var errArtifactLimit = errors.New("artifact exceeds the configured size limit")
