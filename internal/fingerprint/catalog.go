// Package fingerprint defines the public names and revision of Straw's
// built-in outbound fingerprint catalogue.
package fingerprint

import "slices"

// ContractRevision pins the upstream catalogue whose wire data Straw adapts.
const ContractRevision = "tls-client-v1.15.1-http1-http2"

var names = []string{
	"brave_146",
	"brave_146_PSK",
	"chrome_103",
	"chrome_104",
	"chrome_105",
	"chrome_106",
	"chrome_107",
	"chrome_108",
	"chrome_109",
	"chrome_110",
	"chrome_111",
	"chrome_112",
	"chrome_116_PSK",
	"chrome_116_PSK_PQ",
	"chrome_117",
	"chrome_120",
	"chrome_124",
	"chrome_130_PSK",
	"chrome_131",
	"chrome_131_PSK",
	"chrome_133",
	"chrome_133_PSK",
	"chrome_144",
	"chrome_144_PSK",
	"chrome_146",
	"chrome_146_PSK",
	"cloudscraper",
	"confirmed_android",
	"confirmed_ios",
	"firefox_102",
	"firefox_104",
	"firefox_105",
	"firefox_106",
	"firefox_108",
	"firefox_110",
	"firefox_117",
	"firefox_120",
	"firefox_123",
	"firefox_132",
	"firefox_133",
	"firefox_135",
	"firefox_146_PSK",
	"firefox_147",
	"firefox_147_PSK",
	"firefox_148",
	"mesh_android",
	"mesh_android_1",
	"mesh_android_2",
	"mesh_ios",
	"mesh_ios_1",
	"mesh_ios_2",
	"mms_ios",
	"mms_ios_1",
	"mms_ios_2",
	"mms_ios_3",
	"nike_android_mobile",
	"nike_ios_mobile",
	"okhttp4_android_10",
	"okhttp4_android_11",
	"okhttp4_android_12",
	"okhttp4_android_13",
	"okhttp4_android_7",
	"okhttp4_android_8",
	"okhttp4_android_9",
	"opera_89",
	"opera_90",
	"opera_91",
	"safari_15_6_1",
	"safari_16_0",
	"safari_ios_15_5",
	"safari_ios_15_6",
	"safari_ios_16_0",
	"safari_ios_17_0",
	"safari_ios_18_0",
	"safari_ios_18_5",
	"safari_ios_26_0",
	"safari_ipad_15_6",
	"zalando_android_mobile",
	"zalando_ios_mobile",
}

// Names returns the exact, sorted public catalogue without exposing mutable
// package state.
func Names() []string {
	return slices.Clone(names)
}

// Contains reports whether name is an exact public catalogue entry.
func Contains(name string) bool {
	_, ok := slices.BinarySearch(names, name)

	return ok
}
