package update

// ReleaseKeys verify release signatures (see signature.go). The private
// key never touches GitHub: it's on the maintainer's PC, with an offline
// backup (tools/release). To rotate, add the new key here in a release
// signed with the old one, and remove the old one a release later.
var ReleaseKeys = mustKeys(
	"B59dCTxm8VjribRDfgl4nfwp1O4jQZjpfY6SVR+AprQ=", // made 2026-09-27 (Seaglass; WaterLauncher's key was lost)
)
