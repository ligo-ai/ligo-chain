package consensus

import (
	. "github.com/ligo-libs/common-go"
)

var Spec = "1"
var Major = "6"
var Minor = "5"
var Revision = "1"

var Version = Fmt("v%s/%s.%s.%s", Spec, Major, Minor, Revision)
