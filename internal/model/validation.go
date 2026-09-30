package model

import "regexp"

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
var locationPattern = regexp.MustCompile(`^[1-9][0-9]*\.[1-9][0-9]*\.[1-9][0-9]*$`)

func ValidUUID(value string) bool     { return uuidPattern.MatchString(value) }
func ValidLocation(value string) bool { return locationPattern.MatchString(value) }
