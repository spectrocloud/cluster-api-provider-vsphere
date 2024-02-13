//go:build boringcrypto

package flags

import _ "crypto/tls/fipsonly"

func InsecureSkipVerify(insecureSkipVerify bool) bool {
	return false
}

func GetTlsMaxVersion() uint16 {
	return tls.VersionTLS12
}
