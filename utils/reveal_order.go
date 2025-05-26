package utils

type RevealOrderData struct {
	OrderedNodes []string `json:"ordered_nodes"`
	RevealOrder  []int    `json:"reveal_order"`
	RV           string   `json:"rv"`
}
