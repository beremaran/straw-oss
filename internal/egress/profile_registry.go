package egress

import (
	"github.com/bogdanfinn/tls-client/profiles"

	strawpb "github.com/beremaran/straw-oss/api/proto/straw/v1"
)

const baselineFingerprintProfile = "baseline"

var executableFingerprintProfiles = map[string]profiles.ClientProfile{
	chrome120FingerprintProfile: profiles.Chrome_120,
}

func resolveFingerprintInstruction(instruction string) (string, *executionError) {
	if instruction == "" {
		return baselineFingerprintProfile, nil
	}

	if _, ok := executableFingerprintProfiles[instruction]; !ok {
		return "", executorFailure(strawpb.ErrorCode_ERROR_CODE_UNSUPPORTED_FINGERPRINT, unsupportedFingerprint)
	}

	return instruction, nil
}
