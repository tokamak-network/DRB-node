package utils

type RevealOrderData struct {
	Round        string   `json:"round"`
	TrialNum     string   `json:"trial_num"`
	OrderedNodes []string `json:"ordered_nodes"`
	RevealOrder  []int    `json:"reveal_order"`
	RV           string   `json:"rv"`
}
