package utils

type RevealOrderData struct {
	Round        string   `json:"round"`
	OrderedNodes []string `json:"ordered_nodes"`
	RevealOrder  []int    `json:"reveal_order"`
	RV           string   `json:"rv"`
}
