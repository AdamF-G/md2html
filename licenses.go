package md2html

import _ "embed"

//go:embed THIRD_PARTY_NOTICES
var thirdPartyNotices string

// ThirdPartyNotices returns the licence notices of every third-party
// component in this binary.
func ThirdPartyNotices() string { return thirdPartyNotices }
