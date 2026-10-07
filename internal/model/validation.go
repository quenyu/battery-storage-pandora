package model

import "regexp"

var locationPattern = regexp.MustCompile(`^1\.[1-9][0-9]*\.[1-9][0-9]*$`)

func ValidLocation(value string) bool { return locationPattern.MatchString(value) }
