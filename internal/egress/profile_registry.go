package egress

import (
	utls "github.com/bogdanfinn/utls"

	"github.com/beremaran/straw-oss/internal/egress/profilecatalog"
	"github.com/beremaran/straw-oss/internal/fingerprint"
	strawpb "github.com/beremaran/straw-protos-go/straw/v1"
)

const baselineFingerprintProfile = "baseline"

const profileSessionCacheCapacity = 32

var executableFingerprintProfiles = buildExecutableFingerprintProfiles()

func buildExecutableFingerprintProfiles() map[string]profilecatalog.ClientProfile {
	profiles := make(map[string]profilecatalog.ClientProfile, len(profilecatalog.MappedTLSClients))
	for _, name := range fingerprint.Names() {
		profile, ok := profilecatalog.MappedTLSClients[name]
		if !ok {
			panic("fingerprint catalogue is missing executable profile " + name)
		}

		profiles[name] = profile
	}

	if len(profiles) != len(profilecatalog.MappedTLSClients) {
		panic("executable fingerprint catalogue contains an unadvertised profile")
	}

	return profiles
}

func newProfileSessionCaches() map[string]utls.ClientSessionCache {
	caches := make(map[string]utls.ClientSessionCache)

	for name, profile := range executableFingerprintProfiles {
		if supportsProfileSessionResumption(profile) {
			caches[name] = utls.NewLRUClientSessionCache(profileSessionCacheCapacity)
		}
	}

	return caches
}

func supportsProfileSessionResumption(profile profilecatalog.ClientProfile) bool {
	id := profile.GetClientHelloId()

	spec, err := id.ToSpec()
	if err != nil {
		spec, err = utls.UTLSIdToSpec(id)
		if err != nil {
			return false
		}
	}

	for _, extension := range spec.Extensions {
		if _, ok := extension.(*utls.UtlsPreSharedKeyExtension); ok {
			return true
		}
	}

	return false
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
