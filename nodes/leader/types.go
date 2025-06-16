package leader

// committedNodes and activatedOperators are authoritative in-memory states.
// var committedNodes = make(map[string]map[common.Address]utils.LeaderCommitData)
// var activatedOperators = make(map[string]map[common.Address]bool)
type SigRS struct {
	R [32]byte
	S [32]byte
}
type CvAndSigRS struct {
	Cv [32]byte
	Rs SigRS
}
